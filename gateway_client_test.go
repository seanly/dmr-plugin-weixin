package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestPostJSONGetUpdates(t *testing.T) {
	var sawPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sawPath = r.URL.Path
		if r.Header.Get("AuthorizationType") != "ilink_bot_token" {
			t.Errorf("missing AuthorizationType")
		}
		if r.Header.Get("X-WECHAT-UIN") == "" {
			t.Errorf("missing X-WECHAT-UIN")
		}
		_ = json.NewEncoder(w).Encode(getUpdatesResp{
			Ret:           0,
			Msgs:          []weixinMessage{{FromUserID: "u@im.wechat", MessageType: 1}},
			GetUpdatesBuf: "nextbuf",
		})
	}))
	defer srv.Close()

	p := NewWeixinPlugin()
	p.cfg.GatewayBaseURL = srv.URL
	p.cfg.Token = "tok"

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	out, err := p.getUpdates(ctx, "", 2*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(sawPath, "/ilink/bot/getupdates") && sawPath != "/ilink/bot/getupdates" {
		t.Errorf("path %q", sawPath)
	}
	if out.Ret != 0 || len(out.Msgs) != 1 {
		t.Fatalf("resp %+v", out)
	}
}

func TestSendMessageAPI_BizError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/ilink/bot/sendmessage" {
			t.Fatalf("path %s", r.URL.Path)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"ret":     1,
			"errcode": 0,
			"errmsg":  "downstream rejected",
		})
	}))
	defer srv.Close()

	p := NewWeixinPlugin()
	p.cfg.GatewayBaseURL = srv.URL
	p.cfg.Token = "tok"

	ctx := context.Background()
	err := p.sendMessageAPI(ctx, &weixinMessage{
		ToUserID:     "u@im.wechat",
		ClientID:     "c1",
		MessageType:  msgTypeBot,
		MessageState: 2,
		ItemList: []messageItem{
			{Type: itemTypeText, TextItem: &textItem{Text: "hi"}},
		},
		ContextToken: "tokctx",
	})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "ret=1") || !strings.Contains(err.Error(), "downstream") {
		t.Fatalf("unexpected err: %v", err)
	}
}

func TestParseIlinkBizError_baseResp(t *testing.T) {
	raw := []byte(`{"ret":0,"base_resp":{"ret":1,"errcode":0,"errmsg":"reject"}}`)
	err := parseIlinkBizError("ilink/bot/sendmessage", raw)
	if err == nil || !strings.Contains(err.Error(), "base_resp") {
		t.Fatalf("got %v", err)
	}
}

func TestParseIlinkBizError_dataStatus(t *testing.T) {
	raw := []byte(`{"ret":0,"data":{"status":400,"desc":"bad file"}}`)
	err := parseIlinkBizError("ilink/bot/sendmessage", raw)
	if err == nil || !strings.Contains(err.Error(), "data.status") {
		t.Fatalf("got %v", err)
	}
}

func TestParseIlinkBizError_okEmpty(t *testing.T) {
	if err := parseIlinkBizError("x", []byte("{}")); err != nil {
		t.Fatal(err)
	}
	if err := parseIlinkBizError("x", []byte(`{"ret":0}`)); err != nil {
		t.Fatal(err)
	}
}

func TestGetConfigAPI_OK(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/ilink/bot/getconfig" {
			t.Fatalf("path %s", r.URL.Path)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"ret":           0,
			"typing_ticket": "dGlj",
		})
	}))
	defer srv.Close()

	p := NewWeixinPlugin()
	p.cfg.GatewayBaseURL = srv.URL
	p.cfg.Token = "tok"

	out, err := p.getConfigAPI(context.Background(), "u@im.wechat", "ctx")
	if err != nil {
		t.Fatal(err)
	}
	if out.Ret != 0 || out.TypingTicket != "dGlj" {
		t.Fatalf("%+v", out)
	}
}

func TestGetConfigAPI_SessionNestedInData(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/ilink/bot/getconfig" {
			t.Fatalf("path %s", r.URL.Path)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"ret":           0,
			"typing_ticket": "dGlj",
			"data":          map[string]any{"session_id": "sess-in-data"},
		})
	}))
	defer srv.Close()

	p := NewWeixinPlugin()
	p.cfg.GatewayBaseURL = srv.URL
	p.cfg.Token = "tok"

	out, err := p.getConfigAPI(context.Background(), "u@im.wechat", "ctx")
	if err != nil {
		t.Fatal(err)
	}
	if got := out.coalesceOutboundSessionID(); got != "sess-in-data" {
		t.Fatalf("session id: got %q", got)
	}
}

func TestPrefetchOutboundSession_RemembersSessionForPeer(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"ret":           0,
			"session_id":    "persist-me",
			"typing_ticket": "dGlj",
		})
	}))
	defer srv.Close()

	p := NewWeixinPlugin()
	p.cfg.GatewayBaseURL = srv.URL
	p.cfg.Token = "tok"
	p.prefetchOutboundSession(context.Background(), "u@im.wechat", "ctx-1")
	if got := p.sessionIDForPeer("u@im.wechat"); got != "persist-me" {
		t.Fatalf("session for peer: got %q", got)
	}
}

func TestBuildBaseInfo_DefaultAndOverride(t *testing.T) {
	p := NewWeixinPlugin()
	if got := p.buildBaseInfo(); got.ChannelVersion != "1.0.2" {
		t.Fatalf("default: got %q", got.ChannelVersion)
	}
	p.cfg.ChannelVersion = "  9.9.9  "
	if got := p.buildBaseInfo(); got.ChannelVersion != "9.9.9" {
		t.Fatalf("override: got %q", got.ChannelVersion)
	}
}

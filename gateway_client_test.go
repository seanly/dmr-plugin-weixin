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

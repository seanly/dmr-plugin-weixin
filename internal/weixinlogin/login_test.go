package weixinlogin

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestFetchQRCode(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/ilink/bot/get_bot_qrcode" {
			t.Fatalf("path %s", r.URL.Path)
		}
		if r.URL.Query().Get("bot_type") != "3" {
			t.Fatalf("bot_type %q", r.URL.Query().Get("bot_type"))
		}
		_ = json.NewEncoder(w).Encode(qrResponse{Qrcode: "qr-abc", QrcodeImgContent: "https://img.example/q.png"})
	}))
	defer srv.Close()

	client := &http.Client{Timeout: 5 * time.Second}
	qr, img, err := fetchQRCode(context.Background(), client, srv.URL+"/", "3", "")
	if err != nil {
		t.Fatal(err)
	}
	if qr != "qr-abc" || img != "https://img.example/q.png" {
		t.Fatalf("got %q %q", qr, img)
	}
}

func TestPollQRStatus_Confirmed(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/ilink/bot/get_qrcode_status" {
			t.Fatalf("path %s", r.URL.Path)
		}
		if r.Header.Get("iLink-App-ClientVersion") != "1" {
			t.Fatalf("missing client version header")
		}
		if r.URL.Query().Get("qrcode") != "qr-abc" {
			t.Fatalf("qrcode param")
		}
		_ = json.NewEncoder(w).Encode(statusResponse{
			Status:      "confirmed",
			BotToken:    "tok",
			Baseurl:     "https://gw.example",
			IlinkBotID:  "bid",
			IlinkUserID: "uid",
		})
	}))
	defer srv.Close()

	client := &http.Client{Timeout: 5 * time.Second}
	st, err := pollQRStatus(context.Background(), client, srv.URL+"/", "qr-abc", "")
	if err != nil {
		t.Fatal(err)
	}
	if st.Status != "confirmed" || st.BotToken != "tok" || st.IlinkBotID != "bid" {
		t.Fatalf("%+v", st)
	}
}

func TestRun_Confirmed(t *testing.T) {
	var qrHits, statusHits int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/ilink/bot/get_bot_qrcode"):
			qrHits++
			_ = json.NewEncoder(w).Encode(qrResponse{Qrcode: "q1", QrcodeImgContent: "https://x"})
		case strings.HasSuffix(r.URL.Path, "/ilink/bot/get_qrcode_status"):
			statusHits++
			_ = json.NewEncoder(w).Encode(statusResponse{
				Status:      "confirmed",
				BotToken:    "t",
				Baseurl:     "https://api.gw",
				IlinkBotID:  "b",
				IlinkUserID: "u",
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	res, err := Run(context.Background(), Options{
		APIBaseURL: srv.URL,
		BotType:    "3",
		TotalWait:  30 * time.Second,
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.Token != "t" || res.GatewayBaseURL != "https://api.gw" || res.CDNBaseURL != DefaultCDNBaseURL {
		t.Fatalf("%+v", res)
	}
	if qrHits != 1 || statusHits < 1 {
		t.Fatalf("qrHits=%d statusHits=%d", qrHits, statusHits)
	}
}

func TestWriteTerminalQR_WritesSomething(t *testing.T) {
	var buf strings.Builder
	WriteTerminalQR(&buf, "https://liteapp.weixin.qq.com/q/x?qrcode=abc&bot_type=3")
	if buf.Len() < 50 {
		t.Fatalf("expected QR output, got %q", buf.String())
	}
}

func TestRun_NoTerminalQR(t *testing.T) {
	var qrHits, statusHits int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/ilink/bot/get_bot_qrcode"):
			qrHits++
			_ = json.NewEncoder(w).Encode(qrResponse{Qrcode: "q1", QrcodeImgContent: "https://example.com/scan-me"})
		case strings.HasSuffix(r.URL.Path, "/ilink/bot/get_qrcode_status"):
			statusHits++
			_ = json.NewEncoder(w).Encode(statusResponse{
				Status:      "confirmed",
				BotToken:    "t",
				Baseurl:     "https://api.gw",
				IlinkBotID:  "b",
				IlinkUserID: "u",
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	var out strings.Builder
	_, err := Run(context.Background(), Options{
		APIBaseURL:   srv.URL,
		BotType:      "3",
		TotalWait:    30 * time.Second,
		Stdout:       &out,
		NoTerminalQR: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(out.String(), "▄")+strings.Count(out.String(), "▀") > 10 {
		t.Fatal("expected no block QR art when NoTerminalQR")
	}
}

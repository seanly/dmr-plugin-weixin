// Package weixinlogin implements Weixin ilink QR login (reference: openclaw-weixin auth/login-qr.ts).
package weixinlogin

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Public defaults from openclaw-weixin src/auth/accounts.ts (Tencent ilink).
const (
	DefaultAPIBaseURL = "https://ilinkai.weixin.qq.com"
	DefaultCDNBaseURL = "https://novac2c.cdn.weixin.qq.com/c2c"
	DefaultBotType    = "3"
)

const (
	qrPollTimeout   = 35 * time.Second
	maxQRRefresh    = 3
	pollErrBackoff  = time.Second
)

type Options struct {
	APIBaseURL  string // ilink API root; trailing slash optional
	BotType     string
	SKRouteTag  string
	TotalWait   time.Duration // default 8m
	Stdout      io.Writer     // optional progress
	OnQRRefresh func(qrImageURL string) // optional; new QR URL after expiry
	// NoTerminalQR skips drawing a QR on Stdout (e.g. CI); liteapp URLs still print.
	NoTerminalQR bool
}

type Result struct {
	GatewayBaseURL string
	CDNBaseURL     string
	Token          string
	IlinkBotID     string
	IlinkUserID    string
}

type qrResponse struct {
	Qrcode           string `json:"qrcode"`
	QrcodeImgContent string `json:"qrcode_img_content"`
}

type statusResponse struct {
	Status       string `json:"status"`
	BotToken     string `json:"bot_token"`
	IlinkBotID   string `json:"ilink_bot_id"`
	Baseurl      string `json:"baseurl"`
	IlinkUserID  string `json:"ilink_user_id"`
}

func Run(ctx context.Context, opts Options) (*Result, error) {
	api := strings.TrimSpace(opts.APIBaseURL)
	if api == "" {
		api = DefaultAPIBaseURL
	}
	if !strings.HasSuffix(api, "/") {
		api += "/"
	}
	botType := strings.TrimSpace(opts.BotType)
	if botType == "" {
		botType = DefaultBotType
	}
	wait := opts.TotalWait
	if wait < 30*time.Second {
		wait = 8 * time.Minute
	}
	deadline := time.Now().Add(wait)

	client := &http.Client{Timeout: qrPollTimeout + 5*time.Second}

	qr, imgURL, err := fetchQRCode(ctx, client, api, botType, opts.SKRouteTag)
	if err != nil {
		return nil, fmt.Errorf("get_bot_qrcode: %w", err)
	}
	if opts.Stdout != nil && imgURL != "" {
		_, _ = fmt.Fprintf(opts.Stdout, "Scan with WeChat (扫一扫). The liteapp link often shows no QR in Chrome/Safari — use the terminal QR below.\n"+
			"说明：系统浏览器打开下列链接可能只有提示、不显示二维码，请直接扫下方终端里的二维码。\n\n%s\n", imgURL)
		if !opts.NoTerminalQR {
			WriteTerminalQR(opts.Stdout, imgURL)
		}
	}

	qrRefresh := 0
	for time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		st, err := pollQRStatus(ctx, client, api, qr, opts.SKRouteTag)
		if err != nil {
			return nil, err
		}

		switch st.Status {
		case "wait", "":
			time.Sleep(pollErrBackoff)
			continue
		case "scaned":
			if opts.Stdout != nil {
				_, _ = fmt.Fprintln(opts.Stdout, "Scanned — confirm on phone...")
			}
			time.Sleep(pollErrBackoff)
			continue
		case "expired":
			qrRefresh++
			if qrRefresh > maxQRRefresh {
				return nil, fmt.Errorf("QR expired too many times; run login again")
			}
			if opts.Stdout != nil {
				_, _ = fmt.Fprintf(opts.Stdout, "QR expired, refreshing (%d of %d max)...\n", qrRefresh, maxQRRefresh)
			}
			qr, imgURL, err = fetchQRCode(ctx, client, api, botType, opts.SKRouteTag)
			if err != nil {
				return nil, err
			}
			if opts.OnQRRefresh != nil {
				opts.OnQRRefresh(imgURL)
			} else if opts.Stdout != nil && imgURL != "" {
				_, _ = fmt.Fprintf(opts.Stdout, "%s\n", imgURL)
				if !opts.NoTerminalQR {
					WriteTerminalQR(opts.Stdout, imgURL)
				}
			}
			time.Sleep(pollErrBackoff)
			continue
		case "confirmed":
			if strings.TrimSpace(st.IlinkBotID) == "" {
				return nil, fmt.Errorf("login confirmed but ilink_bot_id missing")
			}
			gw := strings.TrimSpace(st.Baseurl)
			if gw == "" {
				gw = strings.TrimSuffix(api, "/")
			}
			tok := strings.TrimSpace(st.BotToken)
			if tok == "" {
				return nil, fmt.Errorf("login confirmed but bot_token missing")
			}
			return &Result{
				GatewayBaseURL: gw,
				CDNBaseURL:     DefaultCDNBaseURL,
				Token:          tok,
				IlinkBotID:     st.IlinkBotID,
				IlinkUserID:    st.IlinkUserID,
			}, nil
		default:
			time.Sleep(pollErrBackoff)
		}
	}
	return nil, fmt.Errorf("login timed out after %v", wait)
}

func fetchQRCode(ctx context.Context, client *http.Client, apiBase, botType, skRouteTag string) (qrcode, imgURL string, err error) {
	u, err := url.Parse(apiBase)
	if err != nil {
		return "", "", err
	}
	ref, _ := url.Parse("ilink/bot/get_bot_qrcode")
	q := ref.Query()
	q.Set("bot_type", botType)
	ref.RawQuery = q.Encode()
	full := u.ResolveReference(ref).String()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, full, nil)
	if err != nil {
		return "", "", err
	}
	if sk := strings.TrimSpace(skRouteTag); sk != "" {
		req.Header.Set("SKRouteTag", sk)
	}
	resp, err := client.Do(req)
	if err != nil {
		return "", "", err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", "", err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", "", fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(body))
	}
	var out qrResponse
	if err := json.Unmarshal(body, &out); err != nil {
		return "", "", err
	}
	if strings.TrimSpace(out.Qrcode) == "" {
		return "", "", fmt.Errorf("empty qrcode in response")
	}
	return out.Qrcode, strings.TrimSpace(out.QrcodeImgContent), nil
}

func pollQRStatus(ctx context.Context, client *http.Client, apiBase, qrcode, skRouteTag string) (*statusResponse, error) {
	u, err := url.Parse(apiBase)
	if err != nil {
		return nil, err
	}
	ref, _ := url.Parse("ilink/bot/get_qrcode_status")
	q := ref.Query()
	q.Set("qrcode", qrcode)
	ref.RawQuery = q.Encode()
	full := u.ResolveReference(ref).String()

	pollCtx, cancel := context.WithTimeout(ctx, qrPollTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(pollCtx, http.MethodGet, full, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("iLink-App-ClientVersion", "1")
	if sk := strings.TrimSpace(skRouteTag); sk != "" {
		req.Header.Set("SKRouteTag", sk)
	}
	resp, err := client.Do(req)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) || errors.Is(pollCtx.Err(), context.DeadlineExceeded) {
			return &statusResponse{Status: "wait"}, nil
		}
		return nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("get_qrcode_status HTTP %d: %s", resp.StatusCode, string(body))
	}
	var out statusResponse
	if err := json.Unmarshal(body, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

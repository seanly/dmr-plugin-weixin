package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

const (
	defaultLongPollTimeout = 35 * time.Second
	defaultAPITimeout      = 15 * time.Second
)

func ensureTrailingSlash(u string) string {
	if strings.HasSuffix(u, "/") {
		return u
	}
	return u + "/"
}

// randomWechatUINHeader matches openclaw api.ts: uint32 BE -> decimal string UTF-8 -> base64.
func randomWechatUINHeader() (string, error) {
	var b [4]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	u := binary.BigEndian.Uint32(b[:])
	s := fmt.Sprintf("%d", u)
	return base64.StdEncoding.EncodeToString([]byte(s)), nil
}

func (p *WeixinPlugin) buildBaseInfo() baseInfo {
	return baseInfo{ChannelVersion: pluginVersion}
}

func (p *WeixinPlugin) postJSON(ctx context.Context, relPath string, body any, timeout time.Duration) ([]byte, error) {
	base := ensureTrailingSlash(strings.TrimSpace(p.cfg.GatewayBaseURL))
	u, err := url.Parse(base)
	if err != nil {
		return nil, fmt.Errorf("gateway_base_url: %w", err)
	}
	ref, err := url.Parse(strings.TrimPrefix(relPath, "/"))
	if err != nil {
		return nil, err
	}
	full := u.ResolveReference(ref).String()

	payload, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}

	uin, err := randomWechatUINHeader()
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, full, bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("AuthorizationType", "ilink_bot_token")
	req.Header.Set("Content-Length", fmt.Sprintf("%d", len(payload)))
	req.Header.Set("X-WECHAT-UIN", uin)
	if tok := strings.TrimSpace(p.cfg.Token); tok != "" {
		req.Header.Set("Authorization", "Bearer "+tok)
	}
	if rt := strings.TrimSpace(p.cfg.SKRouteTag); rt != "" {
		req.Header.Set("SKRouteTag", rt)
	}

	client := &http.Client{Timeout: timeout}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("%s %d: %s", relPath, resp.StatusCode, string(raw))
	}
	return raw, nil
}

func (p *WeixinPlugin) getUpdates(ctx context.Context, buf string, timeout time.Duration) (*getUpdatesResp, error) {
	if timeout <= 0 {
		timeout = defaultLongPollTimeout
	}
	ctx2, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	body := getUpdatesReq{
		GetUpdatesBuf: buf,
		BaseInfo:      p.buildBaseInfo(),
	}
	raw, err := p.postJSON(ctx2, "ilink/bot/getupdates", body, timeout)
	if err != nil {
		if errors.Is(ctx2.Err(), context.DeadlineExceeded) || errors.Is(err, context.DeadlineExceeded) || os.IsTimeout(err) {
			return &getUpdatesResp{Ret: 0, GetUpdatesBuf: buf}, nil
		}
		return nil, err
	}
	var out getUpdatesResp
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (p *WeixinPlugin) sendMessageAPI(ctx context.Context, msg *weixinMessage) error {
	body := sendMessageReq{
		Msg:      msg,
		BaseInfo: p.buildBaseInfo(),
	}
	if ctx == nil {
		ctx = context.Background()
	}
	_, err := p.postJSON(ctx, "ilink/bot/sendmessage", body, defaultAPITimeout)
	return err
}

func (p *WeixinPlugin) getUploadURL(ctx context.Context, req getUploadURLReq) (*getUploadURLResp, error) {
	req.BaseInfo = p.buildBaseInfo()
	if ctx == nil {
		ctx = context.Background()
	}
	raw, err := p.postJSON(ctx, "ilink/bot/getuploadurl", req, defaultAPITimeout)
	if err != nil {
		return nil, err
	}
	var out getUploadURLResp
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

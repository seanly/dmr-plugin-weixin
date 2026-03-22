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
	"log"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

const (
	defaultLongPollTimeout = 35 * time.Second
	defaultAPITimeout      = 15 * time.Second
	defaultConfigTimeout   = 10 * time.Second
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
	v := strings.TrimSpace(p.cfg.ChannelVersion)
	if v == "" {
		v = "1.0.2"
	}
	return baseInfo{ChannelVersion: v}
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
	applyLooseSessionFromGetUpdatesRaw(raw, out.Msgs)
	return &out, nil
}

// getConfigAPI calls ilink/bot/getconfig. OpenClaw invokes this per inbound user (with context_token)
// before replies; some gateways require it for downstream sendmessage delivery to succeed.
func (p *WeixinPlugin) getConfigAPI(ctx context.Context, peerID, contextToken string) (*getConfigResp, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	body := getConfigReq{
		IlinkUserID:  strings.TrimSpace(peerID),
		ContextToken: strings.TrimSpace(contextToken),
		BaseInfo:     p.buildBaseInfo(),
	}
	raw, err := p.postJSON(ctx, "ilink/bot/getconfig", body, defaultConfigTimeout)
	if err != nil {
		return nil, err
	}
	if err := parseIlinkBizError("ilink/bot/getconfig", raw); err != nil {
		return nil, err
	}
	var out getConfigResp
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, fmt.Errorf("getconfig json: %w", err)
	}
	if out.coalesceOutboundSessionID() == "" {
		if s := sessionFromGetConfigRaw(raw); s != "" {
			out.SessionID = s
		}
	}
	return &out, nil
}

// prefetchOutboundSession runs getconfig for the peer; best-effort (logs on failure, does not block inbound).
func (p *WeixinPlugin) prefetchOutboundSession(ctx context.Context, peerID, contextToken string) {
	peerID = strings.TrimSpace(peerID)
	contextToken = strings.TrimSpace(contextToken)
	if peerID == "" || contextToken == "" {
		return
	}
	baseCtx := ctx
	if baseCtx == nil {
		baseCtx = context.Background()
	}
	cctx, cancel := context.WithTimeout(baseCtx, defaultConfigTimeout)
	defer cancel()
	resp, err := p.getConfigAPI(cctx, peerID, contextToken)
	if err != nil {
		log.Printf("weixin: getconfig peer=%q: %v", peerID, err)
		return
	}
	if resp.Ret != 0 || resp.Errcode != 0 {
		msg := strings.TrimSpace(resp.Errmsg)
		if msg == "" {
			msg = "(empty errmsg)"
		}
		log.Printf("weixin: getconfig peer=%q ret=%d errcode=%d errmsg=%q", peerID, resp.Ret, resp.Errcode, msg)
		return
	}
	if os.Getenv("DMR_WEIXIN_DEBUG_SEND") == "1" {
		log.Printf("weixin debug: getconfig ok peer=%q typing_ticket_present=%v session_id_present=%v",
			peerID, strings.TrimSpace(resp.TypingTicket) != "", resp.coalesceOutboundSessionID() != "")
	}
	p.rememberTypingTicket(peerID, resp.TypingTicket)
	if sid := resp.coalesceOutboundSessionID(); sid != "" {
		p.rememberSessionForPeer(peerID, sid)
	}
}

// sendTypingAPI posts ilink/bot/sendtyping (OpenClaw wraps outbound deliver with typing start/stop).
func (p *WeixinPlugin) sendTypingAPI(ctx context.Context, peerID, ticket string, status int) error {
	peerID = strings.TrimSpace(peerID)
	ticket = strings.TrimSpace(ticket)
	if peerID == "" || ticket == "" || status == 0 {
		return nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	body := sendTypingReq{
		IlinkUserID:  peerID,
		TypingTicket: ticket,
		Status:       status,
		BaseInfo:     p.buildBaseInfo(),
	}
	raw, err := p.postJSON(ctx, "ilink/bot/sendtyping", body, defaultConfigTimeout)
	if err != nil {
		return err
	}
	return parseIlinkBizError("ilink/bot/sendtyping", raw)
}

func (p *WeixinPlugin) sendMessageAPI(ctx context.Context, msg *weixinMessage) error {
	body := sendMessageReq{
		Msg:      msg,
		BaseInfo: p.buildBaseInfo(),
	}
	if os.Getenv("DMR_WEIXIN_DEBUG_SEND") == "1" {
		types := make([]int, len(msg.ItemList))
		for i, it := range msg.ItemList {
			types[i] = it.Type
		}
		payload, merr := json.Marshal(body)
		n := 0
		if merr == nil {
			n = len(payload)
		}
		log.Printf("weixin debug: sendmessage req channel_version=%q to_user_id=%q session_id=%q context_token_present=%v contextToken_present=%v item_types=%v json_len=%d marshal_err=%v",
			body.BaseInfo.ChannelVersion, msg.ToUserID, msg.SessionID.String(), strings.TrimSpace(msg.ContextToken) != "", strings.TrimSpace(msg.ContextTokCamel) != "", types, n, merr)
	}
	if ctx == nil {
		ctx = context.Background()
	}

	peerID := strings.TrimSpace(msg.ToUserID)
	ticket := p.typingTicketForPeer(peerID)
	if ticket != "" {
		if err := p.sendTypingAPI(ctx, peerID, ticket, typingStatusTyping); err != nil && os.Getenv("DMR_WEIXIN_DEBUG_SEND") == "1" {
			log.Printf("weixin debug: sendtyping start peer=%q: %v", peerID, err)
		}
	}

	raw, err := p.postJSON(ctx, "ilink/bot/sendmessage", body, defaultAPITimeout)

	if ticket != "" {
		stopCtx := ctx
		sctx, cancel := context.WithTimeout(stopCtx, defaultConfigTimeout)
		_ = p.sendTypingAPI(sctx, peerID, ticket, typingStatusCancel)
		cancel()
	}

	if err != nil {
		return err
	}
	if err := parseIlinkBizError("ilink/bot/sendmessage", raw); err != nil {
		return err
	}
	if os.Getenv("DMR_WEIXIN_DEBUG_SEND") == "1" {
		s := string(raw)
		if len(s) > 1500 {
			s = s[:1500] + "…"
		}
		log.Printf("weixin debug: sendmessage response: %s", s)
	}
	return nil
}

// parseIlinkBizError treats HTTP 200 bodies that still carry errors in nested or alternate fields
// (some ilink endpoints use base_resp or data.status while top-level ret stays 0).
func parseIlinkBizError(endpoint string, raw []byte) error {
	s := strings.TrimSpace(string(raw))
	if s == "" {
		return nil
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil {
		if len(s) > 240 {
			s = s[:240] + "…"
		}
		return fmt.Errorf("%s: response not json: %w body=%q", endpoint, err, s)
	}

	parseRetBlock := func(tag string, chunk []byte) error {
		var x struct {
			Ret      int    `json:"ret"`
			Errcode  int    `json:"errcode"`
			Errmsg   string `json:"errmsg"`
			ErrorMsg string `json:"error_msg"`
		}
		if err := json.Unmarshal(chunk, &x); err != nil {
			return nil
		}
		msg := strings.TrimSpace(x.Errmsg)
		if msg == "" {
			msg = strings.TrimSpace(x.ErrorMsg)
		}
		if x.Ret != 0 || x.Errcode != 0 {
			if msg == "" {
				msg = "(empty errmsg)"
			}
			return fmt.Errorf("%s: %s ret=%d errcode=%d errmsg=%q", endpoint, tag, x.Ret, x.Errcode, msg)
		}
		return nil
	}

	if br, ok := fields["base_resp"]; ok {
		if err := parseRetBlock("base_resp", br); err != nil {
			return err
		}
	}
	if d, ok := fields["data"]; ok {
		var ds struct {
			Status int    `json:"status"`
			Desc   string `json:"desc"`
			Code   int    `json:"code"`
		}
		if err := json.Unmarshal(d, &ds); err == nil {
			if ds.Status != 0 {
				desc := strings.TrimSpace(ds.Desc)
				if desc == "" {
					desc = "(empty desc)"
				}
				return fmt.Errorf("%s: data.status=%d desc=%q", endpoint, ds.Status, desc)
			}
			if ds.Code != 0 {
				desc := strings.TrimSpace(ds.Desc)
				if desc == "" {
					desc = "(empty desc)"
				}
				return fmt.Errorf("%s: data.code=%d desc=%q", endpoint, ds.Code, desc)
			}
		}
	}
	if err := parseRetBlock("top", raw); err != nil {
		return err
	}
	var ec struct {
		ErrorCode int    `json:"error_code"`
		ErrMsg    string `json:"err_msg"`
	}
	if err := json.Unmarshal(raw, &ec); err == nil && ec.ErrorCode != 0 {
		return fmt.Errorf("%s: error_code=%d err_msg=%q", endpoint, ec.ErrorCode, strings.TrimSpace(ec.ErrMsg))
	}
	return nil
}

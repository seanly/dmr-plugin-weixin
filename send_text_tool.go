package main

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

const weixinP2PTapePrefix = "weixin:p2p:"

func weixinP2PTapeToPeerID(tapeName string) (string, error) {
	s := strings.TrimSpace(tapeName)
	if s == "" {
		return "", fmt.Errorf("tape_name is empty")
	}
	if !strings.HasPrefix(s, weixinP2PTapePrefix) {
		return "", fmt.Errorf("tape_name must start with %q", weixinP2PTapePrefix)
	}
	id := strings.TrimSpace(s[len(weixinP2PTapePrefix):])
	if id == "" {
		return "", fmt.Errorf("empty peer id in tape_name")
	}
	return id, nil
}

func sendTextToolParamsJSON() string {
	schema := map[string]any{
		"type":     "object",
		"required": []string{"text"},
		"properties": map[string]any{
			"text": map[string]any{
				"type":        "string",
				"description": "Message body (plain text; markdown stripped).",
			},
			"markdown": map[string]any{
				"type":        "boolean",
				"description": "Ignored for Weixin (always plain); kept for schema compatibility.",
			},
			"tape_name": map[string]any{
				"type":        "string",
				"description": "For cron/non-inbound runs: weixin:p2p:<user@im.wechat>.",
			},
			"peer_id": map[string]any{
				"type":        "string",
				"description": "Alternative to tape_name: raw Weixin peer id (e.g. x@im.wechat).",
			},
		},
	}
	b, _ := json.Marshal(schema)
	return string(b)
}

func argStringTool(m map[string]any, key string) string {
	v, ok := m[key]
	if !ok || v == nil {
		return ""
	}
	switch t := v.(type) {
	case string:
		return strings.TrimSpace(t)
	default:
		return strings.TrimSpace(fmt.Sprint(t))
	}
}

func (p *WeixinPlugin) execSendText(ctx context.Context, argsJSON string) (map[string]any, error) {
	var raw map[string]any
	if strings.TrimSpace(argsJSON) == "" {
		raw = map[string]any{}
	} else if err := json.Unmarshal([]byte(argsJSON), &raw); err != nil {
		return nil, fmt.Errorf("invalid tool arguments JSON: %w", err)
	}
	text := argStringTool(raw, "text")
	if text == "" {
		return nil, fmt.Errorf("text is required")
	}
	tapeName := argStringTool(raw, "tape_name")
	peerArg := argStringTool(raw, "peer_id")

	job := p.getActiveJob()
	if job != nil {
		if tapeName != "" || peerArg != "" {
			return nil, fmt.Errorf("do not set tape_name or peer_id during a Weixin-triggered RunAgent")
		}
		tok := strings.TrimSpace(job.ContextToken)
		if tok == "" {
			tok = p.tokens.get(job.PeerID)
		}
		if err := p.sendTextToPeer(ctx, job.PeerID, tok, text, false); err != nil {
			return nil, err
		}
		return map[string]any{"ok": true, "peer_id": job.PeerID}, nil
	}

	if tapeName != "" && peerArg != "" {
		return nil, fmt.Errorf("provide at most one of tape_name or peer_id")
	}
	var peerID string
	switch {
	case tapeName != "":
		id, err := weixinP2PTapeToPeerID(tapeName)
		if err != nil {
			return nil, err
		}
		peerID = id
	case peerArg != "":
		peerID = peerArg
	default:
		return nil, fmt.Errorf("weixin.send_text requires tape_name or peer_id when not in a Weixin-triggered job")
	}
	tok := p.tokens.get(peerID)
	if tok == "" {
		return nil, fmt.Errorf("no cached context_token for peer %q; user must message the bot first", peerID)
	}
	if err := p.sendTextToPeer(ctx, peerID, tok, text, false); err != nil {
		return nil, err
	}
	return map[string]any{"ok": true, "peer_id": peerID}, nil
}

package weixin

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

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
				"description": "Cron/off-session: weixin:p2p:<peer> or weixin:<bot_id>:p2p:<peer> when several bots.",
			},
			"peer_id": map[string]any{
				"type":        "string",
				"description": "Cron/off-session: raw peer id (requires weixin_bot when multiple bots configured).",
			},
			"weixin_bot": map[string]any{
				"type":        "string",
				"description": "Bots[].id from config.toml; required with peer_id when multiple bots; optional otherwise.",
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

func (p *WeixinPlugin) execSendText(ctx context.Context, argsJSON string, toolCtx map[string]any, sessionTape string) (map[string]any, error) {
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
	weiBotArg := argStringTool(raw, "weixin_bot")

	ctxPeerID, _ := toolCtx["peer_id"].(string)
	ctxToken, _ := toolCtx["context_token"].(string)

	if ctxPeerID != "" {
		if tapeName != "" || peerArg != "" {
			return nil, fmt.Errorf("do not set tape_name or peer_id during a Weixin-triggered RunAgent")
		}
		bot, err := p.botForToolContext(toolCtx, sessionTape)
		if err != nil {
			return nil, err
		}
		tok := strings.TrimSpace(ctxToken)
		if tok == "" {
			tok = bot.tokens.get(ctxPeerID)
		}
		if err := bot.sendTextToPeer(ctx, ctxPeerID, tok, text, false); err != nil {
			return nil, err
		}
		return map[string]any{"ok": true, "peer_id": ctxPeerID}, nil
	}

	if tapeName != "" && peerArg != "" {
		return nil, fmt.Errorf("provide at most one of tape_name or peer_id")
	}
	var peerID string
	var botLabelHint string

	switch {
	case tapeName != "":
		pid, lbl, ok := parseWeixinP2PTape(tapeName)
		if !ok {
			return nil, fmt.Errorf("invalid tape_name %q (expected weixin:p2p:<peer> or weixin:<bot_id>:p2p:<peer>)", tapeName)
		}
		peerID = pid
		botLabelHint = lbl
	case peerArg != "":
		peerID = peerArg
		botLabelHint = weiBotArg
		if p.multiBot() && strings.TrimSpace(botLabelHint) == "" {
			return nil, fmt.Errorf("weixinSendText requires weixin_bot argument when peer_id is used and multiple bots are configured")
		}
	default:
		return nil, fmt.Errorf("weixinSendText requires tape_name or peer_id when not in a Weixin-triggered job")
	}

	bot, err := p.botByTapeLabel(botLabelHint)
	if err != nil {
		return nil, err
	}
	tok := bot.tokens.get(peerID)
	if tok == "" {
		return nil, fmt.Errorf("no cached context_token for peer %q; user must message this bot first", peerID)
	}
	if err := bot.sendTextToPeer(ctx, peerID, tok, text, false); err != nil {
		return nil, err
	}
	return map[string]any{"ok": true, "peer_id": peerID}, nil
}

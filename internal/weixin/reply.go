package weixin

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"
)

const maxWeixinChunkRunes = 4000 // openclaw channel textChunkLimit

func truncateRunes(s string, maxRunes int) string {
	if maxRunes <= 0 {
		return ""
	}
	if utf8.RuneCountInString(s) <= maxRunes {
		return s
	}
	runes := []rune(s)
	if len(runes) > maxRunes {
		s = string(runes[:maxRunes]) + "\n\n…(truncated)"
	}
	return s
}

// markdownToPlainText is a lightweight strip (Weixin has no rich post like Feishu).
func markdownToPlainText(text string) string {
	s := text
	for {
		start := strings.Index(s, "```")
		if start < 0 {
			break
		}
		end := strings.Index(s[start+3:], "```")
		if end < 0 {
			break
		}
		end += start + 3
		inner := s[start+3 : end]
		if nl := strings.Index(inner, "\n"); nl >= 0 {
			inner = strings.TrimSpace(inner[nl+1:])
		}
		s = s[:start] + strings.TrimSpace(inner) + s[end+3:]
	}
	s = strings.ReplaceAll(s, "**", "")
	s = strings.ReplaceAll(s, "__", "")
	return strings.TrimSpace(s)
}

func (p *WeixinPlugin) newClientID() string {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		return fmt.Sprintf("dmr-wx-%d", time.Now().UnixNano())
	}
	return "dmr-wx-" + hex.EncodeToString(b[:])
}

func (b *weixinBot) sendBotMessage(ctx context.Context, peerID, contextToken string, items []messageItem) error {
	if b == nil {
		return fmt.Errorf("nil bot")
	}
	if strings.TrimSpace(contextToken) == "" {
		return fmt.Errorf("contextToken is required")
	}
	msg := &weixinMessage{
		ToUserID:        peerID,
		ClientID:        b.wp.newClientID(),
		MessageType:     msgTypeBot,
		MessageState:    2, // FINISH
		ItemList:        items,
		ContextToken:    contextToken,
		ContextTokCamel: contextToken,
	}
	sid := b.sessionIDForPeer(peerID)
	if sid != "" {
		msg.SessionID = sessionIDJSON(sid)
	}
	return b.sendMessageAPI(ctx, msg)
}

func (b *weixinBot) sendPlainTextChunks(ctx context.Context, peerID, contextToken, text string) error {
	text = markdownToPlainText(truncateRunes(text, maxWeixinChunkRunes*20))
	if strings.TrimSpace(text) == "" {
		return nil
	}
	runes := []rune(text)
	for i := 0; i < len(runes); i += maxWeixinChunkRunes {
		end := i + maxWeixinChunkRunes
		if end > len(runes) {
			end = len(runes)
		}
		chunk := string(runes[i:end])
		if err := b.sendBotMessage(ctx, peerID, contextToken, []messageItem{
			{Type: itemTypeText, TextItem: &textItem{Text: chunk}},
		}); err != nil {
			return err
		}
	}
	return nil
}

func (p *WeixinPlugin) replyAgentOutput(ctx context.Context, job *inboundJob, output string) error {
	if job == nil {
		return fmt.Errorf("nil job")
	}
	bot := job.Bot
	if bot == nil {
		return fmt.Errorf("nil job.bot")
	}
	tok := strings.TrimSpace(job.ContextToken)
	if tok == "" {
		tok = bot.tokens.get(job.PeerID)
	}
	if tok == "" {
		return fmt.Errorf("missing context_token for reply")
	}
	bot.tokens.set(job.PeerID, tok)
	return bot.sendPlainTextChunks(ctx, job.PeerID, tok, output)
}

func (b *weixinBot) sendTextToPeer(ctx context.Context, peerID, contextToken, text string, _ bool) error {
	if b == nil {
		return fmt.Errorf("nil bot")
	}
	tok := strings.TrimSpace(contextToken)
	if tok == "" {
		tok = b.tokens.get(peerID)
	}
	if tok == "" {
		return fmt.Errorf("contextToken is required for weixin send")
	}
	return b.sendPlainTextChunks(ctx, peerID, tok, text)
}

func (b *weixinBot) sendApprovalText(ctx context.Context, peerID, contextToken, body string) error {
	if b == nil {
		return fmt.Errorf("nil bot")
	}
	body = truncateRunes(body, 12000)
	tok := strings.TrimSpace(contextToken)
	if tok == "" {
		tok = b.tokens.get(peerID)
	}
	if tok == "" {
		return fmt.Errorf("missing context_token for approval message")
	}
	return b.sendPlainTextChunks(ctx, peerID, tok, body)
}

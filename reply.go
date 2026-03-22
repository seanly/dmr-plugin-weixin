package main

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"log"
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
	// code blocks: keep inner content roughly
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

func aesKeyHexToBase64(h string) (string, error) {
	raw, err := hex.DecodeString(strings.TrimSpace(h))
	if err != nil || len(raw) != 16 {
		return "", fmt.Errorf("invalid aes key hex")
	}
	return base64.StdEncoding.EncodeToString(raw), nil
}

func (p *WeixinPlugin) sendBotMessage(ctx context.Context, peerID, contextToken string, items []messageItem) error {
	if strings.TrimSpace(contextToken) == "" {
		return fmt.Errorf("contextToken is required")
	}
	msg := &weixinMessage{
		ToUserID:     peerID,
		ClientID:     p.newClientID(),
		MessageType:  msgTypeBot,
		MessageState: 2, // FINISH
		ItemList:     items,
		ContextToken: contextToken,
	}
	return p.sendMessageAPI(ctx, msg)
}

func (p *WeixinPlugin) sendPlainTextChunks(ctx context.Context, peerID, contextToken, text string) error {
	text = markdownToPlainText(truncateRunes(text, maxWeixinChunkRunes*20)) // allow long reply split
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
		if err := p.sendBotMessage(ctx, peerID, contextToken, []messageItem{
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
	tok := strings.TrimSpace(job.ContextToken)
	if tok == "" {
		tok = p.tokens.get(job.PeerID)
	}
	if tok == "" {
		return fmt.Errorf("missing context_token for reply")
	}
	p.tokens.set(job.PeerID, tok)
	return p.sendPlainTextChunks(ctx, job.PeerID, tok, output)
}

func (p *WeixinPlugin) sendTextToPeer(ctx context.Context, peerID, contextToken, text string, _ bool) error {
	tok := strings.TrimSpace(contextToken)
	if tok == "" {
		tok = p.tokens.get(peerID)
	}
	if tok == "" {
		return fmt.Errorf("contextToken is required for weixin send")
	}
	return p.sendPlainTextChunks(ctx, peerID, tok, text)
}

func (p *WeixinPlugin) sendApprovalText(ctx context.Context, peerID, contextToken, body string) error {
	body = truncateRunes(body, 12000)
	tok := strings.TrimSpace(contextToken)
	if tok == "" {
		tok = p.tokens.get(peerID)
	}
	if tok == "" {
		return fmt.Errorf("missing context_token for approval message")
	}
	return p.sendPlainTextChunks(ctx, peerID, tok, body)
}

func (p *WeixinPlugin) sendImageMessage(ctx context.Context, peerID, contextToken, caption string, up *uploadedFileInfo) error {
	tok := strings.TrimSpace(contextToken)
	if tok == "" {
		return fmt.Errorf("context_token required")
	}
	aesB64, err := aesKeyHexToBase64(up.AESKeyHex)
	if err != nil {
		return err
	}
	if strings.TrimSpace(caption) != "" {
		if err := p.sendBotMessage(ctx, peerID, tok, []messageItem{
			{Type: itemTypeText, TextItem: &textItem{Text: truncateRunes(markdownToPlainText(caption), maxWeixinChunkRunes)}},
		}); err != nil {
			log.Printf("weixin: caption send failed: %v", err)
		}
	}
	return p.sendBotMessage(ctx, peerID, tok, []messageItem{
		{
			Type: itemTypeImage,
			ImageItem: &imageItem{
				Media: &cdnMedia{
					EncryptQueryParam: up.DownloadEncryptedQueryParam,
					AesKey:            aesB64,
					EncryptType:       1,
				},
				MidSize: up.FileSizeCiphertext,
			},
		},
	})
}

func (p *WeixinPlugin) sendFileAttachmentMessage(ctx context.Context, peerID, contextToken, caption, displayName string, up *uploadedFileInfo) error {
	tok := strings.TrimSpace(contextToken)
	if tok == "" {
		return fmt.Errorf("context_token required")
	}
	aesB64, err := aesKeyHexToBase64(up.AESKeyHex)
	if err != nil {
		return err
	}
	if strings.TrimSpace(caption) != "" {
		if err := p.sendBotMessage(ctx, peerID, tok, []messageItem{
			{Type: itemTypeText, TextItem: &textItem{Text: truncateRunes(markdownToPlainText(caption), maxWeixinChunkRunes)}},
		}); err != nil {
			log.Printf("weixin: caption send failed: %v", err)
		}
	}
	return p.sendBotMessage(ctx, peerID, tok, []messageItem{
		{
			Type: itemTypeFile,
			FileItem: &fileItem{
				Media: &cdnMedia{
					EncryptQueryParam: up.DownloadEncryptedQueryParam,
					AesKey:            aesB64,
					EncryptType:       1,
				},
				FileName: displayName,
				Len:      fmt.Sprintf("%d", up.FileSize),
			},
		},
	})
}

func (p *WeixinPlugin) sendVideoMessage(ctx context.Context, peerID, contextToken, caption string, up *uploadedFileInfo) error {
	tok := strings.TrimSpace(contextToken)
	if tok == "" {
		return fmt.Errorf("context_token required")
	}
	aesB64, err := aesKeyHexToBase64(up.AESKeyHex)
	if err != nil {
		return err
	}
	if strings.TrimSpace(caption) != "" {
		if err := p.sendBotMessage(ctx, peerID, tok, []messageItem{
			{Type: itemTypeText, TextItem: &textItem{Text: truncateRunes(markdownToPlainText(caption), maxWeixinChunkRunes)}},
		}); err != nil {
			log.Printf("weixin: caption send failed: %v", err)
		}
	}
	return p.sendBotMessage(ctx, peerID, tok, []messageItem{
		{
			Type: itemTypeVideo,
			VideoItem: &videoItem{
				Media: &cdnMedia{
					EncryptQueryParam: up.DownloadEncryptedQueryParam,
					AesKey:            aesB64,
					EncryptType:       1,
				},
				VideoSize: up.FileSizeCiphertext,
			},
		},
	})
}

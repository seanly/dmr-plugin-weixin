package weixin

import (
	"fmt"
	"strings"
)

func tapeNameForP2P(peerID string) string {
	return legacyTape(peerID)
}

// parseWeixinP2PTape parses legacy weixin:p2p:<peer> or multi-bot weixin:<id>:p2p:<peer>.
func parseWeixinP2PTape(tape string) (peerID string, botLabel string, ok bool) {
	tape = strings.TrimSpace(tape)
	const prefix = "weixin:"
	if !strings.HasPrefix(tape, prefix) {
		return "", "", false
	}
	body := tape[len(prefix):]
	const p2pPref = "p2p:"
	if strings.HasPrefix(body, p2pPref) {
		peer := strings.TrimSpace(strings.TrimPrefix(body, p2pPref))
		return peer, "", peer != ""
	}
	const sep = ":p2p:"
	idx := strings.Index(body, sep)
	if idx <= 0 {
		return "", "", false
	}
	botLabel = strings.TrimSpace(body[:idx])
	peer := strings.TrimSpace(body[idx+len(sep):])
	return peer, botLabel, botLabel != "" && peer != ""
}

func p2pPeerFromTape(tape string) (peerID string, ok bool) {
	p, _, ok := parseWeixinP2PTape(tape)
	return p, ok
}

func isMediaItemType(t int) bool {
	return t == itemTypeImage || t == itemTypeVideo || t == itemTypeFile || t == itemTypeVoice
}

func bodyFromItemList(items []messageItem) string {
	if len(items) == 0 {
		return ""
	}
	var parts []string
	for _, item := range items {
		if item.Type == itemTypeText && item.TextItem != nil && item.TextItem.Text != "" {
			text := item.TextItem.Text
			ref := item.RefMsg
			if ref == nil {
				parts = append(parts, text)
				continue
			}
			// For ref messages, just keep the user's text; ref content is
			// handled separately in handleInboundMessage via RefAttachments/RefTextContent.
			parts = append(parts, text)
			continue
		}
		if item.Type == itemTypeVoice && item.VoiceItem != nil && item.VoiceItem.Text != "" {
			parts = append(parts, item.VoiceItem.Text)
			continue
		}
	}
	return strings.Join(parts, "\n")
}

func dedupKeyForMessage(m weixinMessage) string {
	if m.MessageID != 0 {
		return fmt.Sprintf("mid:%d", m.MessageID)
	}
	if m.Seq != 0 {
		return fmt.Sprintf("seq:%d", m.Seq)
	}
	return ""
}

func isAllowedSender(allow []string, senderID string) bool {
	if len(allow) == 0 {
		return true
	}
	senderID = strings.TrimSpace(senderID)
	for _, a := range allow {
		if strings.TrimSpace(a) == senderID {
			return true
		}
	}
	return false
}

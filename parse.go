package main

import (
	"fmt"
	"strings"
)

func tapeNameForP2P(peerID string) string {
	return "weixin:p2p:" + peerID
}

func p2pPeerFromTape(tape string) (peerID string, ok bool) {
	const p = "weixin:p2p:"
	if !strings.HasPrefix(tape, p) {
		return "", false
	}
	id := strings.TrimSpace(tape[len(p):])
	if id == "" {
		return "", false
	}
	return id, true
}

func isMediaItemType(t int) bool {
	return t == itemTypeImage || t == itemTypeVideo || t == itemTypeFile || t == itemTypeVoice
}

func bodyFromItemList(items []messageItem) string {
	if len(items) == 0 {
		return ""
	}
	for _, item := range items {
		if item.Type == itemTypeText && item.TextItem != nil && item.TextItem.Text != "" {
			text := item.TextItem.Text
			ref := item.RefMsg
			if ref == nil {
				return text
			}
			if ref.MessageItem != nil && isMediaItemType(ref.MessageItem.Type) {
				return text
			}
			var parts []string
			if ref.Title != "" {
				parts = append(parts, ref.Title)
			}
			if ref.MessageItem != nil {
				if rb := bodyFromItemList([]messageItem{*ref.MessageItem}); rb != "" {
					parts = append(parts, rb)
				}
			}
			if len(parts) == 0 {
				return text
			}
			return fmt.Sprintf("[引用: %s]\n%s", strings.Join(parts, " | "), text)
		}
		if item.Type == itemTypeVoice && item.VoiceItem != nil && item.VoiceItem.Text != "" {
			return item.VoiceItem.Text
		}
	}
	return ""
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

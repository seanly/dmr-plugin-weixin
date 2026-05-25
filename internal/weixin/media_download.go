package weixin

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// InboundAttachment describes a media file saved from an inbound message.
type InboundAttachment struct {
	Type     string // "image", "voice", "file", "video"
	FilePath string // absolute path on disk
	FileName string // original file name (if known)
}

// saveInboundMedia downloads and saves a media item to workspace/weixin/{date}/.
func (b *weixinBot) saveInboundMedia(ctx context.Context, item messageItem) (*InboundAttachment, error) {
	media, typeName, fileName, ext := extractMediaInfo(item)
	if media == nil {
		return nil, fmt.Errorf("no media in item type %d", item.Type)
	}
	if strings.TrimSpace(media.EncryptQueryParam) == "" || strings.TrimSpace(media.AesKey) == "" {
		return nil, fmt.Errorf("missing cdn params for %s", typeName)
	}

	cdnBase := strings.TrimSpace(b.cdnBaseURL)
	if cdnBase == "" {
		return nil, fmt.Errorf("cdn_base_url not configured")
	}

	plaintext, err := downloadFromCdn(ctx, cdnBase, media.EncryptQueryParam, media.AesKey)
	if err != nil {
		return nil, fmt.Errorf("download %s: %w", typeName, err)
	}

	savePath, err := b.writeMediaFile(plaintext, fileName, ext)
	if err != nil {
		return nil, fmt.Errorf("save %s: %w", typeName, err)
	}

	log.Printf("weixin: saved inbound %s -> %s (%d bytes)", typeName, savePath, len(plaintext))
	return &InboundAttachment{Type: typeName, FilePath: savePath, FileName: fileName}, nil
}

// extractMediaInfo returns the cdnMedia, type name, original file name, and fallback extension for a media item.
func extractMediaInfo(item messageItem) (media *cdnMedia, typeName, fileName, ext string) {
	switch item.Type {
	case itemTypeImage:
		if item.ImageItem == nil {
			return nil, "", "", ""
		}
		m := item.ImageItem.Media
		// Official openclaw-weixin: image_item.aeskey (hex) takes priority over media.aes_key.
		if m != nil && strings.TrimSpace(item.ImageItem.AesKeyHex) != "" {
			hexKey := strings.TrimSpace(item.ImageItem.AesKeyHex)
			m = &cdnMedia{
				EncryptQueryParam: m.EncryptQueryParam,
				AesKey:            hexKey,
				EncryptType:       m.EncryptType,
			}
		}
		return m, "image", "", ".jpg"
	case itemTypeVoice:
		if item.VoiceItem == nil {
			return nil, "", "", ""
		}
		return item.VoiceItem.Media, "voice", "", ".amr"
	case itemTypeFile:
		if item.FileItem == nil {
			return nil, "", "", ""
		}
		fn := strings.TrimSpace(item.FileItem.FileName)
		e := filepath.Ext(fn)
		if e == "" {
			e = ".bin"
		}
		return item.FileItem.Media, "file", fn, e
	case itemTypeVideo:
		if item.VideoItem == nil {
			return nil, "", "", ""
		}
		return item.VideoItem.Media, "video", "", ".mp4"
	default:
		return nil, "", "", ""
	}
}

// writeMediaFile saves plaintext bytes to workspace/weixin/{date}/{ts}-{rand}.{ext}.
func (b *weixinBot) writeMediaFile(data []byte, fileName, ext string) (string, error) {
	ws := ""
	if b != nil && b.wp != nil {
		ws = strings.TrimSpace(b.wp.cfg.Workspace)
	}
	if ws == "" {
		ws = "/tmp/dmr-weixin-media"
	}

	dateDir := filepath.Join(ws, "weixin", time.Now().Format("2006-01-02"))
	if err := os.MkdirAll(dateDir, 0755); err != nil {
		return "", fmt.Errorf("mkdir %s: %w", dateDir, err)
	}

	// Use original file name if available, otherwise generate one.
	var base string
	if fileName != "" {
		// Prefix with timestamp to avoid collisions.
		base = fmt.Sprintf("%d-%s", time.Now().UnixMilli(), sanitizeFileName(fileName))
	} else {
		var rb [4]byte
		_, _ = rand.Read(rb[:])
		base = fmt.Sprintf("%d-%s%s", time.Now().UnixMilli(), hex.EncodeToString(rb[:]), ext)
	}

	savePath := filepath.Join(dateDir, base)
	if err := os.WriteFile(savePath, data, 0644); err != nil {
		return "", err
	}
	return savePath, nil
}

// sanitizeFileName removes path separators and trims spaces from a file name.
func sanitizeFileName(name string) string {
	name = strings.TrimSpace(name)
	name = strings.ReplaceAll(name, "/", "_")
	name = strings.ReplaceAll(name, "\\", "_")
	return name
}

func (b *weixinBot) setRecentMedia(peerID string, atts []InboundAttachment) {
	if len(atts) == 0 || b == nil {
		return
	}
	b.recentMediaMu.Lock()
	defer b.recentMediaMu.Unlock()
	if b.recentMediaByPeer == nil {
		b.recentMediaByPeer = make(map[string][]InboundAttachment)
	}
	existing := b.recentMediaByPeer[peerID]
	existing = append(existing, atts...)
	const maxRecent = 20
	if len(existing) > maxRecent {
		existing = existing[len(existing)-maxRecent:]
	}
	b.recentMediaByPeer[peerID] = existing
}

// popRecentMedia retrieves and clears all saved attachments for a peer.
func (b *weixinBot) popRecentMedia(peerID string) []InboundAttachment {
	if b == nil {
		return nil
	}
	b.recentMediaMu.Lock()
	defer b.recentMediaMu.Unlock()
	atts := b.recentMediaByPeer[peerID]
	delete(b.recentMediaByPeer, peerID)
	return atts
}

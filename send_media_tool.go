package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// sendMediaToolParamsJSON returns the JSON schema for weixinSendMedia tool
func sendMediaToolParamsJSON() string {
	schema := map[string]any{
		"type":     "object",
		"required": []string{"file_path"},
		"properties": map[string]any{
			"file_path": map[string]any{
				"type":        "string",
				"description": "Local file path or remote URL (http/https) to send.",
			},
			"text": map[string]any{
				"type":        "string",
				"description": "Optional caption text to send with the media.",
			},
			"media_type": map[string]any{
				"type":        "string",
				"enum":        []string{"auto", "image", "video", "file"},
				"description": "Media type: auto (detect from extension), image, video, or file. Default: auto.",
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

// sendImageMessageWeixin sends an image message
func (p *WeixinPlugin) sendImageMessageWeixin(ctx context.Context, peerID, contextToken, text string, uploaded *UploadedFileInfo) error {
	if contextToken == "" {
		return fmt.Errorf("contextToken is required")
	}

	// Convert AES key from hex to base64
	aesKeyBase64 := base64.StdEncoding.EncodeToString([]byte(uploaded.AesKey))

	items := []messageItem{}
	if text != "" {
		items = append(items, messageItem{
			Type:     itemTypeText,
			TextItem: &textItem{Text: text},
		})
	}

	items = append(items, messageItem{
		Type: itemTypeImage,
		ImageItem: &imageItem{
			Media: &cdnMedia{
				EncryptQueryParam: uploaded.DownloadEncryptedQueryParam,
				AesKey:            aesKeyBase64,
				EncryptType:       1,
			},
			MidSize: int(uploaded.FileSizeCiphertext),
			HdSize:  int(uploaded.FileSizeCiphertext),
		},
	})

	// Send each item separately
	for _, item := range items {
		msg := &weixinMessage{
			FromUserID:   "",
			ToUserID:     peerID,
			ClientID:     p.generateClientID(),
			MessageType:  msgTypeBot,
			MessageState: 2, // FINISH
			ItemList:     []messageItem{item},
			ContextToken: contextToken,
			SessionID:    sessionIDJSON(p.sessionIDForPeer(peerID)),
		}

		if err := p.sendMessageAPI(ctx, msg); err != nil {
			return err
		}
	}

	return nil
}

// sendVideoMessageWeixin sends a video message
func (p *WeixinPlugin) sendVideoMessageWeixin(ctx context.Context, peerID, contextToken, text string, uploaded *UploadedFileInfo) error {
	if contextToken == "" {
		return fmt.Errorf("contextToken is required")
	}

	aesKeyBase64 := base64.StdEncoding.EncodeToString([]byte(uploaded.AesKey))

	items := []messageItem{}
	if text != "" {
		items = append(items, messageItem{
			Type:     itemTypeText,
			TextItem: &textItem{Text: text},
		})
	}

	items = append(items, messageItem{
		Type: itemTypeVideo,
		VideoItem: &videoItem{
			Media: &cdnMedia{
				EncryptQueryParam: uploaded.DownloadEncryptedQueryParam,
				AesKey:            aesKeyBase64,
				EncryptType:       1,
			},
			VideoSize: int(uploaded.FileSizeCiphertext),
		},
	})

	for _, item := range items {
		msg := &weixinMessage{
			FromUserID:   "",
			ToUserID:     peerID,
			ClientID:     p.generateClientID(),
			MessageType:  msgTypeBot,
			MessageState: 2,
			ItemList:     []messageItem{item},
			ContextToken: contextToken,
			SessionID:    sessionIDJSON(p.sessionIDForPeer(peerID)),
		}

		if err := p.sendMessageAPI(ctx, msg); err != nil {
			return err
		}
	}

	return nil
}

// sendFileMessageWeixin sends a file message
func (p *WeixinPlugin) sendFileMessageWeixin(ctx context.Context, peerID, contextToken, text, fileName string, uploaded *UploadedFileInfo) error {
	if contextToken == "" {
		return fmt.Errorf("contextToken is required")
	}

	aesKeyBase64 := base64.StdEncoding.EncodeToString([]byte(uploaded.AesKey))

	items := []messageItem{}
	if text != "" {
		items = append(items, messageItem{
			Type:     itemTypeText,
			TextItem: &textItem{Text: text},
		})
	}

	items = append(items, messageItem{
		Type: itemTypeFile,
		FileItem: &fileItem{
			Media: &cdnMedia{
				EncryptQueryParam: uploaded.DownloadEncryptedQueryParam,
				AesKey:            aesKeyBase64,
				EncryptType:       1,
			},
			FileName: fileName,
			Len:      fmt.Sprintf("%d", uploaded.FileSize),
		},
	})

	for _, item := range items {
		msg := &weixinMessage{
			FromUserID:   "",
			ToUserID:     peerID,
			ClientID:     p.generateClientID(),
			MessageType:  msgTypeBot,
			MessageState: 2,
			ItemList:     []messageItem{item},
			ContextToken: contextToken,
			SessionID:    sessionIDJSON(p.sessionIDForPeer(peerID)),
		}

		if err := p.sendMessageAPI(ctx, msg); err != nil {
			return err
		}
	}

	return nil
}

// sendWeixinMediaFile uploads and sends a media file
func (p *WeixinPlugin) sendWeixinMediaFile(ctx context.Context, filePath, peerID, contextToken, text, mediaType string) error {
	// Determine media type if auto
	if mediaType == "" || mediaType == "auto" {
		mime := getMimeFromFilename(filePath)
		if strings.HasPrefix(mime, "video/") {
			mediaType = "video"
		} else if strings.HasPrefix(mime, "image/") {
			mediaType = "image"
		} else {
			mediaType = "file"
		}
	}

	// Upload file
	var uploaded *UploadedFileInfo
	var err error

	switch mediaType {
	case "image":
		uploaded, err = p.uploadImageToWeixin(ctx, filePath, peerID)
	case "video":
		uploaded, err = p.uploadVideoToWeixin(ctx, filePath, peerID)
	case "file":
		uploaded, err = p.uploadFileToWeixin(ctx, filePath, peerID)
	default:
		return fmt.Errorf("unsupported media_type: %s", mediaType)
	}

	if err != nil {
		return fmt.Errorf("upload failed: %w", err)
	}

	// Send message
	switch mediaType {
	case "image":
		return p.sendImageMessageWeixin(ctx, peerID, contextToken, text, uploaded)
	case "video":
		return p.sendVideoMessageWeixin(ctx, peerID, contextToken, text, uploaded)
	case "file":
		fileName := filepath.Base(filePath)
		return p.sendFileMessageWeixin(ctx, peerID, contextToken, text, fileName, uploaded)
	}

	return nil
}

// execSendMedia executes the weixinSendMedia tool
func (p *WeixinPlugin) execSendMedia(ctx context.Context, argsJSON string) (map[string]any, error) {
	var raw map[string]any
	if strings.TrimSpace(argsJSON) == "" {
		raw = map[string]any{}
	} else if err := json.Unmarshal([]byte(argsJSON), &raw); err != nil {
		return nil, fmt.Errorf("invalid tool arguments JSON: %w", err)
	}

	filePath := argStringTool(raw, "file_path")
	if filePath == "" {
		return nil, fmt.Errorf("file_path is required")
	}

	text := argStringTool(raw, "text")
	mediaType := argStringTool(raw, "media_type")
	tapeName := argStringTool(raw, "tape_name")
	peerArg := argStringTool(raw, "peer_id")

	// Handle remote URLs
	if strings.HasPrefix(filePath, "http://") || strings.HasPrefix(filePath, "https://") {
		tempDir := "/tmp/dmr-weixin-media"
		localPath, err := downloadRemoteFile(ctx, filePath, tempDir)
		if err != nil {
			return nil, fmt.Errorf("download remote file: %w", err)
		}
		defer os.Remove(localPath)
		filePath = localPath
	}

	// Check if file exists
	if _, err := os.Stat(filePath); err != nil {
		return nil, fmt.Errorf("file not found: %s", filePath)
	}

	// Determine peer ID
	job := p.getActiveJob()
	var peerID, contextToken string

	if job != nil {
		if tapeName != "" || peerArg != "" {
			return nil, fmt.Errorf("do not set tape_name or peer_id during a Weixin-triggered RunAgent")
		}
		peerID = job.PeerID
		contextToken = strings.TrimSpace(job.ContextToken)
		if contextToken == "" {
			contextToken = p.tokens.get(peerID)
		}
	} else {
		if tapeName != "" && peerArg != "" {
			return nil, fmt.Errorf("provide at most one of tape_name or peer_id")
		}
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
			return nil, fmt.Errorf("weixinSendMedia requires tape_name or peer_id when not in a Weixin-triggered job")
		}
		contextToken = p.tokens.get(peerID)
		if contextToken == "" {
			return nil, fmt.Errorf("no cached context_token for peer %q; user must message the bot first", peerID)
		}
	}

	// Send media file
	if err := p.sendWeixinMediaFile(ctx, filePath, peerID, contextToken, text, mediaType); err != nil {
		return nil, err
	}

	return map[string]any{"ok": true, "peer_id": peerID, "media_type": mediaType}, nil
}

// generateClientID generates a unique client ID for messages
func (p *WeixinPlugin) generateClientID() string {
	return p.newClientID()
}

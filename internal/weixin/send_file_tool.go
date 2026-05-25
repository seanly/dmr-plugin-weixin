package weixin

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// sendFileToolParamsJSON returns the JSON schema for weixinSendFile tool.
func sendFileToolParamsJSON() string {
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
				"description": "Optional caption text to send with the file.",
			},
			"file_type": map[string]any{
				"type":        "string",
				"enum":        []string{"auto", "image", "video", "file", "1", "2", "3", "4"},
				"description": "Send-as kind: native image, video, or generic file attachment, or auto (infer). Matches ilink CDN media_type integers 1 image, 2 video, 3 file, 4 voice (voice not supported outbound). Prefer names; numeric strings permitted.",
			},
			"media_type": map[string]any{
				"type":        "string",
				"enum":        []string{"auto", "image", "video", "file", "voice", "1", "2", "3", "4"},
				"description": "Synonym for file_type (ilink media_type semantics). Must match file_type when both are set.",
			},
			"tape_name": map[string]any{
				"type":        "string",
				"description": "Cron/off-session: weixin:p2p:<peer> or weixin:<bot_id>:p2p:<peer> when several bots.",
			},
			"peer_id": map[string]any{
				"type":        "string",
				"description": "Cron/off-session: raw peer id (requires weixin_bot when multiple bots).",
			},
			"weixin_bot": map[string]any{
				"type":        "string",
				"description": "Bots[].id; required with peer_id when multiple bots.",
			},
		},
	}
	b, _ := json.Marshal(schema)
	return string(b)
}

// mergeOutboundKind returns the effective send-as kind from optional file_type and media_type arguments.
func mergeOutboundKind(raw map[string]any) (kind string, err error) {
	set1, k1, err := parseOutboundKindArg(raw, "file_type")
	if err != nil {
		return "", err
	}
	set2, k2, err := parseOutboundKindArg(raw, "media_type")
	if err != nil {
		return "", err
	}
	if !set1 && !set2 {
		return "", nil
	}
	if !set1 {
		return k2, nil
	}
	if !set2 {
		return k1, nil
	}
	if k1 != k2 {
		return "", fmt.Errorf("file_type %q conflicts with media_type %q", k1, k2)
	}
	return k1, nil
}

// parseOutboundKindArg reads one of file_type or media_type — same allowed values including ilink numeric strings 1–4.
func parseOutboundKindArg(raw map[string]any, key string) (set bool, kind string, err error) {
	s := strings.TrimSpace(strings.ToLower(argStringTool(raw, key)))
	if s == "" {
		return false, "", nil
	}
	switch s {
	case "auto":
		return true, "auto", nil
	case "image", "1":
		return true, "image", nil
	case "video", "2":
		return true, "video", nil
	case "file", "3":
		return true, "file", nil
	case "voice", "4":
		return false, "", fmt.Errorf("%s: outbound voice (ilink type 4) is not implemented for weixinSendFile", key)
	default:
		return false, "", fmt.Errorf("%s: unsupported value %q (use auto, image, video, file, voice, or integers 1–4 per ilink CDN)", key, s)
	}
}

// sendImageMessageWeixin sends an image message
func (b *weixinBot) sendImageMessageWeixin(ctx context.Context, peerID, contextToken, text string, uploaded *UploadedFileInfo) error {
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
			ClientID:     b.wp.newClientID(),
			MessageType:  msgTypeBot,
			MessageState: 2, // FINISH
			ItemList:     []messageItem{item},
			ContextToken: contextToken,
			SessionID:    sessionIDJSON(b.sessionIDForPeer(peerID)),
		}

		if err := b.sendMessageAPI(ctx, msg); err != nil {
			return err
		}
	}

	return nil
}

// sendVideoMessageWeixin sends a video message
func (b *weixinBot) sendVideoMessageWeixin(ctx context.Context, peerID, contextToken, text string, uploaded *UploadedFileInfo) error {
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
			ClientID:     b.wp.newClientID(),
			MessageType:  msgTypeBot,
			MessageState: 2,
			ItemList:     []messageItem{item},
			ContextToken: contextToken,
			SessionID:    sessionIDJSON(b.sessionIDForPeer(peerID)),
		}

		if err := b.sendMessageAPI(ctx, msg); err != nil {
			return err
		}
	}

	return nil
}

// sendFileMessageWeixin sends a file message
func (b *weixinBot) sendFileMessageWeixin(ctx context.Context, peerID, contextToken, text, fileName string, uploaded *UploadedFileInfo) error {
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
			ClientID:     b.wp.newClientID(),
			MessageType:  msgTypeBot,
			MessageState: 2,
			ItemList:     []messageItem{item},
			ContextToken: contextToken,
			SessionID:    sessionIDJSON(b.sessionIDForPeer(peerID)),
		}

		if err := b.sendMessageAPI(ctx, msg); err != nil {
			return err
		}
	}

	return nil
}

// sendWeixinFilePayload uploads to CDN then sends image, video, or generic file item on Weixin (ilink).
// The returned string is the resolved kind ("image"|"video"|"file").
func (b *weixinBot) sendWeixinFilePayload(ctx context.Context, filePath, peerID, contextToken, text, fileKind string) (resolvedKind string, err error) {
	// Resolve kind when auto
	if fileKind == "" || fileKind == "auto" {
		mime := getMimeFromFilename(filePath)
		if strings.HasPrefix(mime, "video/") {
			fileKind = "video"
		} else if strings.HasPrefix(mime, "image/") {
			fileKind = "image"
		} else {
			fileKind = "file"
		}
	}

	var uploaded *UploadedFileInfo

	switch fileKind {
	case "image":
		uploaded, err = b.uploadImageToWeixin(ctx, filePath, peerID)
	case "video":
		uploaded, err = b.uploadVideoToWeixin(ctx, filePath, peerID)
	case "file":
		uploaded, err = b.uploadFileToWeixin(ctx, filePath, peerID)
	default:
		return "", fmt.Errorf("unsupported file_type/media_type: %s", fileKind)
	}

	if err != nil {
		return "", fmt.Errorf("upload failed: %w", err)
	}

	switch fileKind {
	case "image":
		return fileKind, b.sendImageMessageWeixin(ctx, peerID, contextToken, text, uploaded)
	case "video":
		return fileKind, b.sendVideoMessageWeixin(ctx, peerID, contextToken, text, uploaded)
	case "file":
		fileName := filepath.Base(filePath)
		return fileKind, b.sendFileMessageWeixin(ctx, peerID, contextToken, text, fileName, uploaded)
	}

	return "", nil
}

// execSendFile executes weixinSendFile.
func (p *WeixinPlugin) execSendFile(ctx context.Context, argsJSON string, toolCtx map[string]any, sessionTape string) (map[string]any, error) {
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
	fileKind, errKind := mergeOutboundKind(raw)
	if errKind != nil {
		return nil, errKind
	}
	tapeName := argStringTool(raw, "tape_name")
	peerArg := argStringTool(raw, "peer_id")
	weiBotArg := argStringTool(raw, "weixin_bot")

	if strings.HasPrefix(filePath, "http://") || strings.HasPrefix(filePath, "https://") {
		tempDir := "/tmp/dmr-weixin-files"
		localPath, err := downloadRemoteFile(ctx, filePath, tempDir)
		if err != nil {
			return nil, fmt.Errorf("download remote file: %w", err)
		}
		defer os.Remove(localPath)
		filePath = localPath
	}

	if _, err := os.Stat(filePath); err != nil {
		return nil, fmt.Errorf("file not found: %s", filePath)
	}

	ctxPeerID, _ := toolCtx["peer_id"].(string)
	ctxToken, _ := toolCtx["context_token"].(string)
	var peerID, contextToken string
	var bot *weixinBot

	if ctxPeerID != "" {
		if tapeName != "" || peerArg != "" {
			return nil, fmt.Errorf("do not set tape_name or peer_id during a Weixin-triggered RunAgent")
		}
		peerID = ctxPeerID
		contextToken = strings.TrimSpace(ctxToken)
		var err error
		bot, err = p.botForToolContext(toolCtx, sessionTape)
		if err != nil {
			return nil, err
		}
		if contextToken == "" {
			contextToken = bot.tokens.get(peerID)
		}
	} else {
		if tapeName != "" && peerArg != "" {
			return nil, fmt.Errorf("provide at most one of tape_name or peer_id")
		}
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
				return nil, fmt.Errorf("weixinSendFile requires weixin_bot when peer_id is used and multiple bots are configured")
			}
		default:
			return nil, fmt.Errorf("weixinSendFile requires tape_name or peer_id when not in a Weixin-triggered job")
		}
		var err error
		bot, err = p.botByTapeLabel(botLabelHint)
		if err != nil {
			return nil, err
		}
		contextToken = bot.tokens.get(peerID)
		if contextToken == "" {
			return nil, fmt.Errorf("no cached context_token for peer %q; user must message this bot first", peerID)
		}
	}

	resolved, err := bot.sendWeixinFilePayload(ctx, filePath, peerID, contextToken, text, fileKind)
	if err != nil {
		return nil, err
	}

	return map[string]any{"ok": true, "peer_id": peerID, "file_type": resolved}, nil
}

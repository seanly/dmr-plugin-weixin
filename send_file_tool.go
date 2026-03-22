package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"
)

const maxSendFileNameRunes = 200

func sendFileToolParamsJSON() string {
	schema := map[string]any{
		"type":     "object",
		"required": []string{"path"},
		"properties": map[string]any{
			"path": map[string]any{
				"type":        "string",
				"description": "Local file path; resolved under send_file_root or cwd.",
			},
			"caption": map[string]any{
				"type":        "string",
				"description": "Optional short text before the file.",
			},
			"filename": map[string]any{
				"type":        "string",
				"description": "Optional display name; default basename of path.",
			},
		},
	}
	b, _ := json.Marshal(schema)
	return string(b)
}

func sanitizeFileNameWX(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return "file.bin"
	}
	name = filepath.Base(name)
	name = strings.ReplaceAll(name, string(filepath.Separator), "_")
	if name == "" || name == "." || name == ".." {
		return "file.bin"
	}
	if utf8.RuneCountInString(name) > maxSendFileNameRunes {
		runes := []rune(name)
		name = string(runes[:maxSendFileNameRunes])
	}
	return name
}

func enforcePathUnderRootWX(pathAbs, rootAbs string) error {
	rel, err := filepath.Rel(rootAbs, pathAbs)
	if err != nil {
		return fmt.Errorf("path not under allowed root: %w", err)
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return fmt.Errorf("path escapes allowed root directory")
	}
	return nil
}

func resolveSendFilePathWX(pathStr, rootFromCfg string) (absPath string, err error) {
	pathStr = strings.TrimSpace(pathStr)
	if pathStr == "" {
		return "", fmt.Errorf("path is empty")
	}
	root := strings.TrimSpace(rootFromCfg)
	var rootAbs string
	if root != "" {
		rootAbs, err = filepath.Abs(filepath.Clean(root))
	} else {
		var cwd string
		cwd, err = os.Getwd()
		if err != nil {
			return "", fmt.Errorf("getwd: %w", err)
		}
		rootAbs, err = filepath.Abs(cwd)
	}
	if err != nil {
		return "", fmt.Errorf("resolve root: %w", err)
	}
	cleaned := filepath.Clean(pathStr)
	var pathAbs string
	if filepath.IsAbs(cleaned) {
		pathAbs = cleaned
	} else {
		pathAbs = filepath.Join(rootAbs, cleaned)
	}
	pathAbs, err = filepath.Abs(pathAbs)
	if err != nil {
		return "", err
	}
	if err := enforcePathUnderRootWX(pathAbs, rootAbs); err != nil {
		return "", err
	}
	return pathAbs, nil
}

func (p *WeixinPlugin) execSendFile(ctx context.Context, argsJSON string) (map[string]any, error) {
	job := p.getActiveJob()
	if job == nil {
		return nil, fmt.Errorf("weixin.send_file only works during a Weixin-triggered RunAgent")
	}
	var raw map[string]any
	if strings.TrimSpace(argsJSON) == "" {
		raw = map[string]any{}
	} else if err := json.Unmarshal([]byte(argsJSON), &raw); err != nil {
		return nil, fmt.Errorf("invalid tool arguments JSON: %w", err)
	}
	pathStr := argStringTool(raw, "path")
	if pathStr == "" {
		return nil, fmt.Errorf("path is required")
	}
	filenameArg := argStringTool(raw, "filename")
	caption := argStringTool(raw, "caption")
	maxBytes := p.cfg.sendFileMaxBytes()

	tok := strings.TrimSpace(job.ContextToken)
	if tok == "" {
		tok = p.tokens.get(job.PeerID)
	}
	if tok == "" {
		return nil, fmt.Errorf("missing context_token for outbound file")
	}

	abs, err := resolveSendFilePathWX(pathStr, p.cfg.SendFileRoot)
	if err != nil {
		return nil, err
	}
	fi, err := os.Stat(abs)
	if err != nil {
		return nil, fmt.Errorf("stat path: %w", err)
	}
	if fi.IsDir() {
		return nil, fmt.Errorf("path is a directory")
	}
	if fi.Size() > maxBytes {
		return nil, fmt.Errorf("file size %d exceeds limit %d", fi.Size(), maxBytes)
	}

	displayName := sanitizeFileNameWX(filenameArg)
	if displayName == "file.bin" || filenameArg == "" {
		displayName = sanitizeFileNameWX(filepath.Base(abs))
	}

	if caption != "" {
		_ = p.sendTextToPeer(ctx, job.PeerID, tok, caption, false)
	}

	f, err := os.Open(abs)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	plain, err := io.ReadAll(io.LimitReader(f, fi.Size()))
	if err != nil {
		return nil, err
	}

	mime := mimeFromFilename(abs)
	var errSend error
	switch {
	case strings.HasPrefix(mime, "video/"):
		up, e := p.uploadMediaToCDN(ctx, job.PeerID, plain, uploadMediaVideo)
		if e != nil {
			return nil, e
		}
		errSend = p.sendVideoMessage(ctx, job.PeerID, tok, "", up)
	case strings.HasPrefix(mime, "image/"):
		up, e := p.uploadMediaToCDN(ctx, job.PeerID, plain, uploadMediaImage)
		if e != nil {
			return nil, e
		}
		errSend = p.sendImageMessage(ctx, job.PeerID, tok, "", up)
	default:
		up, e := p.uploadMediaToCDN(ctx, job.PeerID, plain, uploadMediaFile)
		if e != nil {
			return nil, e
		}
		errSend = p.sendFileAttachmentMessage(ctx, job.PeerID, tok, "", displayName, up)
	}
	if errSend != nil {
		return nil, errSend
	}
	return map[string]any{
		"ok":      true,
		"path":    abs,
		"peer_id": job.PeerID,
	}, nil
}

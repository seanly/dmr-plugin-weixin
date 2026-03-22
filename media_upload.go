package main

import (
	"context"
	"crypto/md5"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// uploadMediaToCdn is the common upload pipeline for all media types
func (p *WeixinPlugin) uploadMediaToCdn(ctx context.Context, filePath, toUserID string, mediaType int) (*UploadedFileInfo, error) {
	// Read file
	plaintext, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("read file: %w", err)
	}

	rawsize := int64(len(plaintext))

	// Calculate MD5
	hash := md5.Sum(plaintext)
	rawfilemd5 := hex.EncodeToString(hash[:])

	// Calculate ciphertext size
	filesize := aesEcbPaddedSize(rawsize)

	// Generate random filekey (16 bytes hex)
	filekeyBytes := make([]byte, 16)
	if _, err := rand.Read(filekeyBytes); err != nil {
		return nil, fmt.Errorf("generate filekey: %w", err)
	}
	filekey := hex.EncodeToString(filekeyBytes)

	// Generate random AES key (16 bytes)
	aesKey := make([]byte, 16)
	if _, err := rand.Read(aesKey); err != nil {
		return nil, fmt.Errorf("generate aeskey: %w", err)
	}

	// Get upload URL from API
	uploadReq := getUploadUrlReq{
		Filekey:     filekey,
		MediaType:   mediaType,
		ToUserID:    toUserID,
		Rawsize:     rawsize,
		Rawfilemd5:  rawfilemd5,
		Filesize:    filesize,
		NoNeedThumb: true,
		AesKey:      hex.EncodeToString(aesKey),
		BaseInfo:    p.buildBaseInfo(),
	}

	raw, err := p.postJSON(ctx, "ilink/bot/getuploadurl", uploadReq, defaultAPITimeout)
	if err != nil {
		return nil, fmt.Errorf("getuploadurl: %w", err)
	}

	var uploadResp getUploadUrlResp
	if err := json.Unmarshal(raw, &uploadResp); err != nil {
		return nil, fmt.Errorf("parse getuploadurl response: %w", err)
	}

	if uploadResp.UploadParam == "" {
		return nil, fmt.Errorf("getuploadurl returned no upload_param")
	}

	// Upload to CDN
	downloadParam, err := p.uploadBufferToCdn(ctx, plaintext, uploadResp.UploadParam, filekey, aesKey, mediaType)
	if err != nil {
		return nil, fmt.Errorf("cdn upload: %w", err)
	}

	return &UploadedFileInfo{
		Filekey:                    filekey,
		DownloadEncryptedQueryParam: downloadParam,
		AesKey:                     hex.EncodeToString(aesKey),
		FileSize:                   rawsize,
		FileSizeCiphertext:         filesize,
	}, nil
}

// uploadImageToWeixin uploads an image file
func (p *WeixinPlugin) uploadImageToWeixin(ctx context.Context, filePath, toUserID string) (*UploadedFileInfo, error) {
	return p.uploadMediaToCdn(ctx, filePath, toUserID, UploadMediaTypeImage)
}

// uploadVideoToWeixin uploads a video file
func (p *WeixinPlugin) uploadVideoToWeixin(ctx context.Context, filePath, toUserID string) (*UploadedFileInfo, error) {
	return p.uploadMediaToCdn(ctx, filePath, toUserID, UploadMediaTypeVideo)
}

// uploadFileToWeixin uploads a generic file attachment
func (p *WeixinPlugin) uploadFileToWeixin(ctx context.Context, filePath, toUserID string) (*UploadedFileInfo, error) {
	return p.uploadMediaToCdn(ctx, filePath, toUserID, UploadMediaTypeFile)
}

// getMimeFromFilename returns MIME type based on file extension
func getMimeFromFilename(filename string) string {
	ext := strings.ToLower(filepath.Ext(filename))
	switch ext {
	case ".jpg", ".jpeg":
		return "image/jpeg"
	case ".png":
		return "image/png"
	case ".gif":
		return "image/gif"
	case ".webp":
		return "image/webp"
	case ".bmp":
		return "image/bmp"
	case ".mp4":
		return "video/mp4"
	case ".mov":
		return "video/quicktime"
	case ".avi":
		return "video/x-msvideo"
	case ".mkv":
		return "video/x-matroska"
	case ".webm":
		return "video/webm"
	case ".pdf":
		return "application/pdf"
	case ".doc":
		return "application/msword"
	case ".docx":
		return "application/vnd.openxmlformats-officedocument.wordprocessingml.document"
	case ".xls":
		return "application/vnd.ms-excel"
	case ".xlsx":
		return "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"
	case ".zip":
		return "application/zip"
	case ".txt":
		return "text/plain"
	default:
		return "application/octet-stream"
	}
}

// downloadRemoteFile downloads a remote URL to a local temp file
func downloadRemoteFile(ctx context.Context, urlStr, destDir string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, urlStr, nil)
	if err != nil {
		return "", err
	}

	client := &http.Client{Timeout: defaultAPITimeout}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return "", fmt.Errorf("download failed: status %d", resp.StatusCode)
	}

	// Read response body
	buf, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	// Create destination directory
	if err := os.MkdirAll(destDir, 0755); err != nil {
		return "", err
	}

	// Determine file extension from Content-Type or URL
	ext := getExtensionFromContentType(resp.Header.Get("Content-Type"))
	if ext == "" {
		ext = filepath.Ext(urlStr)
	}
	if ext == "" {
		ext = ".bin"
	}

	// Generate temp filename
	tempFile := filepath.Join(destDir, fmt.Sprintf("weixin-remote-%d%s", time.Now().UnixNano(), ext))

	// Write to file
	if err := os.WriteFile(tempFile, buf, 0644); err != nil {
		return "", err
	}

	return tempFile, nil
}

// getExtensionFromContentType returns file extension based on Content-Type
func getExtensionFromContentType(contentType string) string {
	contentType = strings.ToLower(strings.TrimSpace(contentType))
	if idx := strings.Index(contentType, ";"); idx > 0 {
		contentType = contentType[:idx]
	}

	switch contentType {
	case "image/jpeg":
		return ".jpg"
	case "image/png":
		return ".png"
	case "image/gif":
		return ".gif"
	case "image/webp":
		return ".webp"
	case "video/mp4":
		return ".mp4"
	case "video/quicktime":
		return ".mov"
	case "application/pdf":
		return ".pdf"
	default:
		return ""
	}
}

package main

import (
	"bytes"
	"context"
	"crypto/aes"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

// UploadMediaType constants match openclaw-weixin types.ts
const (
	UploadMediaTypeImage = 1
	UploadMediaTypeVideo = 2
	UploadMediaTypeFile  = 3
	UploadMediaTypeVoice = 4
)

const uploadMaxRetries = 3

// UploadedFileInfo contains the result of uploading a file to CDN
type UploadedFileInfo struct {
	Filekey                    string
	DownloadEncryptedQueryParam string
	AesKey                     string // hex-encoded
	FileSize                   int64
	FileSizeCiphertext         int64
}

// getUploadUrlReq matches openclaw-weixin types.ts GetUploadUrlReq
type getUploadUrlReq struct {
	Filekey        string   `json:"filekey,omitempty"`
	MediaType      int      `json:"media_type,omitempty"`
	ToUserID       string   `json:"to_user_id,omitempty"`
	Rawsize        int64    `json:"rawsize,omitempty"`
	Rawfilemd5     string   `json:"rawfilemd5,omitempty"`
	Filesize       int64    `json:"filesize,omitempty"`
	NoNeedThumb    bool     `json:"no_need_thumb,omitempty"`
	AesKey         string   `json:"aeskey,omitempty"`
	BaseInfo       baseInfo `json:"base_info"`
}

// getUploadUrlResp matches openclaw-weixin types.ts GetUploadUrlResp
type getUploadUrlResp struct {
	UploadParam      string `json:"upload_param,omitempty"`
	ThumbUploadParam string `json:"thumb_upload_param,omitempty"`
}

// pkcs7Pad adds PKCS7 padding to match AES block size
func pkcs7Pad(data []byte, blockSize int) []byte {
	padding := blockSize - len(data)%blockSize
	padtext := bytes.Repeat([]byte{byte(padding)}, padding)
	return append(data, padtext...)
}

// encryptAesEcb encrypts data using AES-128-ECB with PKCS7 padding
func encryptAesEcb(plaintext []byte, key []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	plaintext = pkcs7Pad(plaintext, aes.BlockSize)
	ciphertext := make([]byte, len(plaintext))

	// ECB mode: encrypt each block independently
	for i := 0; i < len(plaintext); i += aes.BlockSize {
		block.Encrypt(ciphertext[i:i+aes.BlockSize], plaintext[i:i+aes.BlockSize])
	}
	return ciphertext, nil
}

// aesEcbPaddedSize calculates the ciphertext size after AES-128-ECB encryption
func aesEcbPaddedSize(plaintextSize int64) int64 {
	blockSize := int64(aes.BlockSize)
	padding := blockSize - (plaintextSize % blockSize)
	return plaintextSize + padding
}

// uploadBufferToCdn uploads encrypted buffer to Weixin CDN
func (p *WeixinPlugin) uploadBufferToCdn(ctx context.Context, buf []byte, uploadParam, filekey string, aesKey []byte, mediaType int) (string, error) {
	cdnBaseURL := strings.TrimSpace(p.cfg.CDNBaseURL)
	if cdnBaseURL == "" {
		return "", fmt.Errorf("cdn_base_url not configured")
	}

	// Encrypt the buffer
	ciphertext, err := encryptAesEcb(buf, aesKey)
	if err != nil {
		return "", fmt.Errorf("encrypt: %w", err)
	}

	// Build CDN upload URL
	cdnURL, err := buildCdnUploadUrl(cdnBaseURL, uploadParam, filekey, mediaType)
	if err != nil {
		return "", err
	}

	var downloadParam string
	var lastErr error

	// Retry up to uploadMaxRetries times
	for attempt := 1; attempt <= uploadMaxRetries; attempt++ {
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, cdnURL, bytes.NewReader(ciphertext))
		if err != nil {
			return "", err
		}
		req.Header.Set("Content-Type", "application/octet-stream")

		client := &http.Client{Timeout: defaultAPITimeout}
		resp, err := client.Do(req)
		if err != nil {
			lastErr = err
			if attempt < uploadMaxRetries {
				continue
			}
			break
		}
		defer resp.Body.Close()

		// 4xx errors: fail immediately
		if resp.StatusCode >= 400 && resp.StatusCode < 500 {
			body, _ := io.ReadAll(resp.Body)
			errMsg := resp.Header.Get("x-error-message")
			if errMsg == "" {
				errMsg = string(body)
			}
			return "", fmt.Errorf("CDN client error %d: %s", resp.StatusCode, errMsg)
		}

		// Non-200 status: retry
		if resp.StatusCode != 200 {
			errMsg := resp.Header.Get("x-error-message")
			if errMsg == "" {
				errMsg = fmt.Sprintf("status %d", resp.StatusCode)
			}
			lastErr = fmt.Errorf("CDN server error: %s", errMsg)
			if attempt < uploadMaxRetries {
				continue
			}
			break
		}

		// Success: get download param from header
		downloadParam = resp.Header.Get("x-encrypted-param")
		if downloadParam == "" {
			lastErr = fmt.Errorf("CDN response missing x-encrypted-param header")
			if attempt < uploadMaxRetries {
				continue
			}
			break
		}

		return downloadParam, nil
	}

	if lastErr != nil {
		return "", fmt.Errorf("CDN upload failed after %d attempts: %w", uploadMaxRetries, lastErr)
	}
	return "", fmt.Errorf("CDN upload failed after %d attempts", uploadMaxRetries)
}

// buildCdnUploadUrl constructs the CDN upload URL
func buildCdnUploadUrl(cdnBaseURL, uploadParam, filekey string, mediaType int) (string, error) {
	base := ensureTrailingSlash(cdnBaseURL)
	u, err := url.Parse(base)
	if err != nil {
		return "", err
	}

	// Map media type to filetype parameter
	filetype := "file"
	switch mediaType {
	case UploadMediaTypeImage:
		filetype = "image"
	case UploadMediaTypeVideo:
		filetype = "video"
	case UploadMediaTypeFile:
		filetype = "file"
	case UploadMediaTypeVoice:
		filetype = "voice"
	}

	// Append upload path with query params
	q := u.Query()
	q.Set("encrypted_query_param", uploadParam)
	q.Set("filekey", filekey)
	q.Set("filetype", filetype)
	u.RawQuery = q.Encode()
	u.Path = strings.TrimSuffix(u.Path, "/") + "/upload"

	return u.String(), nil
}

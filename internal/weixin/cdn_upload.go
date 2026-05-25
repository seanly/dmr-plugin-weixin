package weixin

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
	UploadFullURL    string `json:"upload_full_url,omitempty"`
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

// uploadBufferToCdn uploads ciphertext to Weixin CDN (openclaw-weixin parity).
// If uploadFullURL is non-empty (from getuploadurl.upload_full_url), it is POSTed directly; otherwise the URL is built from cdn_base_url + upload_param + filekey.
func (b *weixinBot) uploadBufferToCdn(ctx context.Context, buf []byte, uploadFullURL, uploadParam, filekey string, aesKey []byte) (string, error) {
	uploadFullURL = strings.TrimSpace(uploadFullURL)
	uploadParam = strings.TrimSpace(uploadParam)

	// Encrypt the buffer (same path as Tencent openclaw-weixin uploadBufferToCdn)
	ciphertext, err := encryptAesEcb(buf, aesKey)
	if err != nil {
		return "", fmt.Errorf("encrypt: %w", err)
	}

	var cdnURL string
	if uploadFullURL != "" {
		cdnURL = uploadFullURL
	} else if uploadParam != "" {
		base := strings.TrimSpace(b.cdnBaseURL)
		if base == "" {
			return "", fmt.Errorf("cdn_base_url not configured (getuploadurl did not return upload_full_url)")
		}
		cdnURL, err = buildCdnUploadUrl(base, uploadParam, filekey)
		if err != nil {
			return "", err
		}
	} else {
		return "", fmt.Errorf("CDN upload URL missing (need upload_full_url or upload_param)")
	}

	var downloadParam string
	var lastErr error

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

		bodyLimited, readErr := io.ReadAll(io.LimitReader(resp.Body, 8192))
		_ = resp.Body.Close()

		if readErr != nil {
			lastErr = readErr
			if attempt < uploadMaxRetries {
				continue
			}
			break
		}

		if resp.StatusCode >= 400 && resp.StatusCode < 500 {
			return "", fmt.Errorf("CDN client error %d: %s", resp.StatusCode, formatCdnErrorDetail(resp.Header, bodyLimited))
		}

		if resp.StatusCode != 200 {
			errMsg := formatCdnErrorDetail(resp.Header, bodyLimited)
			lastErr = fmt.Errorf("CDN server error: status %d: %s", resp.StatusCode, errMsg)
			if attempt < uploadMaxRetries {
				continue
			}
			break
		}

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

// formatCdnErrorDetail mirrors openclaw-weixin: header x-error-message, else truncated body/content-type hint.
func formatCdnErrorDetail(h http.Header, body []byte) string {
	if s := strings.TrimSpace(h.Get("x-error-message")); s != "" {
		return s
	}
	raw := strings.TrimSpace(string(body))
	if raw != "" {
		const maxSnippet = 512
		runes := []rune(raw)
		if len(runes) > maxSnippet {
			raw = string(runes[:maxSnippet]) + "…"
		}
		return raw
	}
	ct := strings.TrimSpace(h.Get("Content-Type"))
	if ct != "" {
		return fmt.Sprintf("(empty body, content-type=%q)", ct)
	}
	return "(empty body)"
}

// buildCdnUploadUrl matches openclaw-weixin cdn-url.ts buildCdnUploadUrl: only encrypted_query_param and filekey — no extra filetype query.
func buildCdnUploadUrl(cdnBaseURL, uploadParam, filekey string) (string, error) {
	base := strings.TrimSuffix(strings.TrimSpace(cdnBaseURL), "/")
	if base == "" {
		return "", fmt.Errorf("cdn base url empty")
	}
	ep := url.QueryEscape(uploadParam)
	fk := url.QueryEscape(filekey)
	return fmt.Sprintf("%s/upload?encrypted_query_param=%s&filekey=%s", base, ep, fk), nil
}

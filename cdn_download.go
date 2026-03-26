package main

import (
	"context"
	"crypto/aes"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

// decryptAesEcb decrypts AES-128-ECB ciphertext and removes PKCS7 padding.
func decryptAesEcb(ciphertext, key []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	if len(ciphertext) == 0 || len(ciphertext)%aes.BlockSize != 0 {
		return nil, fmt.Errorf("ciphertext length %d not a multiple of block size", len(ciphertext))
	}
	plaintext := make([]byte, len(ciphertext))
	for i := 0; i < len(ciphertext); i += aes.BlockSize {
		block.Decrypt(plaintext[i:i+aes.BlockSize], ciphertext[i:i+aes.BlockSize])
	}
	return pkcs7Unpad(plaintext)
}

// pkcs7Unpad removes PKCS7 padding.
func pkcs7Unpad(data []byte) ([]byte, error) {
	if len(data) == 0 {
		return nil, fmt.Errorf("empty data")
	}
	padding := int(data[len(data)-1])
	if padding == 0 || padding > aes.BlockSize || padding > len(data) {
		return nil, fmt.Errorf("invalid PKCS7 padding %d", padding)
	}
	for i := len(data) - padding; i < len(data); i++ {
		if data[i] != byte(padding) {
			return nil, fmt.Errorf("invalid PKCS7 padding byte at %d", i)
		}
	}
	return data[:len(data)-padding], nil
}

// buildCdnDownloadUrl constructs the CDN download URL from base URL and encrypted query param.
func buildCdnDownloadUrl(cdnBaseURL, encryptQueryParam string) (string, error) {
	base := ensureTrailingSlash(cdnBaseURL)
	u, err := url.Parse(base)
	if err != nil {
		return "", err
	}
	q := u.Query()
	q.Set("encrypted_query_param", encryptQueryParam)
	u.RawQuery = q.Encode()
	u.Path = strings.TrimSuffix(u.Path, "/") + "/download"
	return u.String(), nil
}

// downloadFromCdn downloads and decrypts a file from Weixin CDN.
// aesKeyField is the aes_key value from the inbound cdnMedia — it may be
// base64-encoded hex (44 chars, as produced by sendmessage) or raw hex (32 chars).
func downloadFromCdn(ctx context.Context, cdnBaseURL, encryptQueryParam, aesKeyField string) ([]byte, error) {
	cdnURL, err := buildCdnDownloadUrl(cdnBaseURL, encryptQueryParam)
	if err != nil {
		return nil, fmt.Errorf("build cdn download url: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, cdnURL, nil)
	if err != nil {
		return nil, err
	}

	client := &http.Client{Timeout: defaultAPITimeout}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("cdn download %d: %s", resp.StatusCode, string(body))
	}

	ciphertext, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read cdn response: %w", err)
	}

	key, err := decodeAesKey(aesKeyField)
	if err != nil {
		return nil, fmt.Errorf("decode aes key: %w", err)
	}

	return decryptAesEcb(ciphertext, key)
}

// decodeAesKey handles the two encodings seen in the wild:
//   - base64 of hex string (len 44): base64 → 32-char hex → 16 bytes
//   - raw hex (len 32): hex → 16 bytes
//   - raw 16 bytes (len 16, non-printable): used as-is
func decodeAesKey(field string) ([]byte, error) {
	field = strings.TrimSpace(field)
	if len(field) == 16 {
		// Already raw 16 bytes.
		return []byte(field), nil
	}
	if len(field) == 32 {
		// Raw hex.
		return hex.DecodeString(field)
	}
	// Try base64 decode first (covers len 44 and other base64 lengths).
	decoded, err := base64.StdEncoding.DecodeString(field)
	if err != nil {
		return nil, fmt.Errorf("aes key not valid base64 or hex (len %d): %w", len(field), err)
	}
	// decoded could be the hex string (32 bytes) or raw key (16 bytes).
	if len(decoded) == 16 {
		return decoded, nil
	}
	if len(decoded) == 32 {
		return hex.DecodeString(string(decoded))
	}
	return nil, fmt.Errorf("aes key decoded to unexpected length %d", len(decoded))
}

package main

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

func buildCdnUploadURL(cdnBase, uploadParam, filekey string) string {
	base := strings.TrimSuffix(strings.TrimSpace(cdnBase), "/")
	u := fmt.Sprintf("%s/upload?encrypted_query_param=%s&filekey=%s",
		base,
		url.QueryEscape(uploadParam),
		url.QueryEscape(filekey),
	)
	return u
}

func uploadBufferToCDN(ctx context.Context, cdnBase, uploadParam, filekey string, ciphertext []byte, aesKey []byte) (downloadParam string, err error) {
	u := buildCdnUploadURL(cdnBase, uploadParam, filekey)
	const maxRetries = 3
	var lastErr error
	for attempt := 1; attempt <= maxRetries; attempt++ {
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, u, bytes.NewReader(ciphertext))
		if err != nil {
			return "", err
		}
		req.Header.Set("Content-Type", "application/octet-stream")
		client := &http.Client{Timeout: 120 * time.Second}
		resp, err := client.Do(req)
		if err != nil {
			lastErr = err
			continue
		}
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if resp.StatusCode >= 400 && resp.StatusCode < 500 {
			return "", fmt.Errorf("CDN upload client error %d: %s", resp.StatusCode, string(body))
		}
		if resp.StatusCode != 200 {
			lastErr = fmt.Errorf("CDN upload server error %d", resp.StatusCode)
			continue
		}
		dp := resp.Header.Get("x-encrypted-param")
		if dp == "" {
			lastErr = fmt.Errorf("CDN response missing x-encrypted-param")
			continue
		}
		_ = aesKey
		return dp, nil
	}
	if lastErr != nil {
		return "", lastErr
	}
	return "", fmt.Errorf("CDN upload failed after %d attempts", maxRetries)
}

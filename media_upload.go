package main

import (
	"context"
	"crypto/md5"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"path/filepath"
	"strings"
	"time"
)

type uploadedFileInfo struct {
	Filekey                    string
	DownloadEncryptedQueryParam string
	AESKeyHex                  string
	FileSize                   int
	FileSizeCiphertext         int
}

func randomHex(n int) string {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return fmt.Sprintf("%016x", time.Now().UnixNano())
	}
	return hex.EncodeToString(b)
}

func (p *WeixinPlugin) uploadMediaToCDN(ctx context.Context, peerID string, plaintext []byte, mediaType int) (*uploadedFileInfo, error) {
	rawsize := len(plaintext)
	sum := md5.Sum(plaintext)
	rawfilemd5 := hex.EncodeToString(sum[:])
	filesize := aesEcbPaddedSize(rawsize)
	filekey := randomHex(16)
	aeskey := make([]byte, 16)
	if _, err := rand.Read(aeskey); err != nil {
		return nil, err
	}
	aeskeyHex := hex.EncodeToString(aeskey)

	resp, err := p.getUploadURL(ctx, getUploadURLReq{
		Filekey:     filekey,
		MediaType:   mediaType,
		ToUserID:    peerID,
		Rawsize:     rawsize,
		Rawfilemd5:  rawfilemd5,
		Filesize:    filesize,
		NoNeedThumb: true,
		Aeskey:      aeskeyHex,
	})
	if err != nil {
		return nil, err
	}
	if resp.UploadParam == "" {
		return nil, fmt.Errorf("getuploadurl: empty upload_param")
	}
	ciphertext, err := encryptAESECBPKCS7(plaintext, aeskey)
	if err != nil {
		return nil, err
	}
	cdnBase := strings.TrimSpace(p.cfg.CDNBaseURL)
	if cdnBase == "" {
		return nil, fmt.Errorf("cdn_base_url is required for file upload")
	}
	downloadParam, err := uploadBufferToCDN(ctx, cdnBase, resp.UploadParam, filekey, ciphertext, aeskey)
	if err != nil {
		return nil, err
	}
	return &uploadedFileInfo{
		Filekey:                     filekey,
		DownloadEncryptedQueryParam: downloadParam,
		AESKeyHex:                   aeskeyHex,
		FileSize:                    rawsize,
		FileSizeCiphertext:         filesize,
	}, nil
}

func mimeFromFilename(name string) string {
	ext := strings.ToLower(strings.TrimPrefix(filepath.Ext(name), "."))
	switch ext {
	case "png", "jpg", "jpeg", "gif", "webp", "bmp":
		return "image/" + ext
	case "mp4", "mov", "webm":
		return "video/" + ext
	default:
		return "application/octet-stream"
	}
}

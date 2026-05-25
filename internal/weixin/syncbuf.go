package weixin

import (
	"os"
	"path/filepath"
	"strings"
)

func (b *weixinBot) syncBufPath() string {
	base := ""
	if b != nil && b.wp != nil {
		base = strings.TrimSpace(b.wp.cfg.ConfigBaseDir)
	}
	acc := "default"
	if b != nil {
		acc = strings.TrimSpace(b.accountID)
		if acc == "" {
			acc = "default"
		}
	}
	name := "weixin_" + acc + "_get_updates_buf.txt"
	if base != "" {
		dir := filepath.Join(base, ".dmr-weixin")
		return filepath.Join(dir, name)
	}
	dir, err := os.UserCacheDir()
	if err != nil {
		dir = os.TempDir()
	}
	return filepath.Join(dir, "dmr-weixin", name)
}

func (b *weixinBot) loadSyncBuf() string {
	path := b.syncBufPath()
	raw, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return string(raw)
}

func (b *weixinBot) saveSyncBuf(buf string) {
	if buf == "" {
		return
	}
	path := b.syncBufPath()
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return
	}
	_ = os.WriteFile(path, []byte(buf), 0o600)
}

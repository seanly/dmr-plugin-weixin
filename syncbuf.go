package main

import (
	"os"
	"path/filepath"
	"strings"
)

func (p *WeixinPlugin) syncBufPath() string {
	base := strings.TrimSpace(p.cfg.ConfigBaseDir)
	acc := strings.TrimSpace(p.cfg.AccountID)
	if acc == "" {
		acc = "default"
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

func (p *WeixinPlugin) loadSyncBuf() string {
	path := p.syncBufPath()
	b, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return string(b)
}

func (p *WeixinPlugin) saveSyncBuf(buf string) {
	if buf == "" {
		return
	}
	path := p.syncBufPath()
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return
	}
	_ = os.WriteFile(path, []byte(buf), 0o600)
}

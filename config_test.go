package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestMergeWeixinCredentials(t *testing.T) {
	dir := t.TempDir()
	credPath := filepath.Join(dir, "c.json")
	if err := os.WriteFile(credPath, []byte(`{
  "gateway_base_url": "https://gw.from.file",
  "cdn_base_url": "https://cdn.from.file",
  "token": "secret-from-file"
}`), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg := WeixinConfig{
		ConfigBaseDir:   dir,
		GatewayBaseURL:  "https://yaml-gw",
		CDNBaseURL:      "https://yaml-cdn",
		Token:           "yaml-token",
		CredentialsPath: "c.json",
	}
	if err := mergeWeixinCredentials(&cfg); err != nil {
		t.Fatal(err)
	}
	if cfg.GatewayBaseURL != "https://gw.from.file" || cfg.CDNBaseURL != "https://cdn.from.file" || cfg.Token != "secret-from-file" {
		t.Fatalf("%+v", cfg)
	}
}

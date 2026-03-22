package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const defaultSendFileMaxBytes = 30 * 1024 * 1024 // 30 MiB

const pluginVersion = "0.1.0"

// WeixinConfig is loaded from the plugin InitRequest.ConfigJSON (DMR merges YAML into JSON).
// gateway_base_url, cdn_base_url, and token have no built-in defaults — set them to match your ilink/bot gateway deployment.
type WeixinConfig struct {
	ConfigBaseDir string `json:"config_base_dir"`
	// GatewayBaseURL is the ilink HTTP API root; paths like ilink/bot/getupdates are appended.
	GatewayBaseURL string `json:"gateway_base_url"`
	// CDNBaseURL is the origin for media /upload and /download used by that gateway.
	CDNBaseURL string `json:"cdn_base_url"`
	// Token is sent as Authorization: Bearer <token> (do not include the "Bearer " prefix in YAML).
	Token string `json:"token"`
	// CredentialsPath is optional JSON written by dmr-weixin-login; non-empty fields overlay gateway_base_url, cdn_base_url, token.
	CredentialsPath string `json:"credentials_path"`
	// SKRouteTag optional header SKRouteTag.
	SKRouteTag string `json:"sk_route_tag"`
	// AccountID isolates sync buf file and logs; default "default".
	AccountID string `json:"account_id"`
	AllowFrom            []string `json:"allow_from"`
	ApprovalTimeoutSec   int      `json:"approval_timeout_sec"`
	DedupTTLMinutes      int      `json:"dedup_ttl_minutes"`
	SendFileMaxBytes     int      `json:"send_file_max_bytes"`
	SendFileRoot         string   `json:"send_file_root"`
	ExtraPrompt          string   `json:"extra_prompt"`
	ExtraPromptFile      string   `json:"extra_prompt_file"`
	DmrRestartTrigger    string   `json:"dmr_restart_trigger"`
	DmrRestartToken      string   `json:"dmr_restart_token"`
}

func defaultWeixinConfig() WeixinConfig {
	return WeixinConfig{
		ApprovalTimeoutSec: 300,
		DedupTTLMinutes:    10,
		AccountID:          "default",
	}
}

func parseWeixinConfig(jsonStr string) (WeixinConfig, error) {
	cfg := defaultWeixinConfig()
	if jsonStr == "" {
		return cfg, nil
	}
	if err := json.Unmarshal([]byte(jsonStr), &cfg); err != nil {
		return cfg, err
	}
	if cfg.ApprovalTimeoutSec <= 0 {
		cfg.ApprovalTimeoutSec = 300
	}
	if cfg.DedupTTLMinutes <= 0 {
		cfg.DedupTTLMinutes = 10
	}
	if cfg.SendFileMaxBytes <= 0 {
		cfg.SendFileMaxBytes = defaultSendFileMaxBytes
	}
	if strings.TrimSpace(cfg.AccountID) == "" {
		cfg.AccountID = "default"
	}
	if err := mergeWeixinCredentials(&cfg); err != nil {
		return cfg, err
	}
	return cfg, nil
}

type weixinCredentialsFile struct {
	GatewayBaseURL string `json:"gateway_base_url"`
	CDNBaseURL     string `json:"cdn_base_url"`
	Token          string `json:"token"`
}

func expandHomePath(p string) string {
	p = strings.TrimSpace(p)
	if strings.HasPrefix(p, "~/") {
		if h, err := os.UserHomeDir(); err == nil && h != "" {
			return filepath.Join(h, strings.TrimPrefix(p, "~/"))
		}
	}
	return p
}

func mergeWeixinCredentials(cfg *WeixinConfig) error {
	p := strings.TrimSpace(cfg.CredentialsPath)
	if p == "" {
		return nil
	}
	p = expandHomePath(p)
	abs := resolveExtraPromptPath(p, cfg.ConfigBaseDir)
	b, err := os.ReadFile(abs)
	if err != nil {
		return fmt.Errorf("credentials_path %q: %w", p, err)
	}
	var f weixinCredentialsFile
	if err := json.Unmarshal(b, &f); err != nil {
		return fmt.Errorf("credentials_path %q: %w", p, err)
	}
	if s := strings.TrimSpace(f.GatewayBaseURL); s != "" {
		cfg.GatewayBaseURL = s
	}
	if s := strings.TrimSpace(f.CDNBaseURL); s != "" {
		cfg.CDNBaseURL = s
	}
	if s := strings.TrimSpace(f.Token); s != "" {
		cfg.Token = s
	}
	return nil
}

func (c WeixinConfig) sendFileMaxBytes() int64 {
	if c.SendFileMaxBytes <= 0 {
		return int64(defaultSendFileMaxBytes)
	}
	return int64(c.SendFileMaxBytes)
}

func (c WeixinConfig) approvalTimeout() time.Duration {
	return time.Duration(c.ApprovalTimeoutSec) * time.Second
}

func (c WeixinConfig) dedupTTL() time.Duration {
	return time.Duration(c.DedupTTLMinutes) * time.Minute
}

func resolveExtraPromptPath(path, configBaseDir string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return ""
	}
	if filepath.IsAbs(path) {
		return filepath.Clean(path)
	}
	base := strings.TrimSpace(configBaseDir)
	if base != "" {
		return filepath.Clean(filepath.Join(base, path))
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return filepath.Clean(path)
	}
	return abs
}

func buildResolvedExtraPrompt(cfg WeixinConfig) (string, error) {
	var parts []string
	if fp := strings.TrimSpace(cfg.ExtraPromptFile); fp != "" {
		abs := resolveExtraPromptPath(fp, cfg.ConfigBaseDir)
		b, err := os.ReadFile(abs)
		if err != nil {
			return "", fmt.Errorf("extra_prompt_file %q: %w", fp, err)
		}
		if s := strings.TrimSpace(string(b)); s != "" {
			parts = append(parts, s)
		}
	}
	if s := strings.TrimSpace(cfg.ExtraPrompt); s != "" {
		parts = append(parts, s)
	}
	return strings.Join(parts, "\n\n"), nil
}

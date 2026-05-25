package weixin

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/seanly/dmr-plugin-weixin/internal/weixinlogin"
)

// WeixinBotConfig describes one bot / ilink credential set (YAML/TOML `[[plugins]] … [plugins.config]` / `bots` array).
type WeixinBotConfig struct {
	// ID is required when multiple bots are configured; it becomes `weixin:<id>:p2p:<peer>` tape segment.
	ID string `json:"id"`

	GatewayBaseURL   string   `json:"gateway_base_url"`
	CDNBaseURL       string   `json:"cdn_base_url"`
	Token            string   `json:"token"`
	CredentialsPath  string   `json:"credentials_path"`
	SKRouteTag       string   `json:"sk_route_tag"`
	ChannelVersion   string   `json:"channel_version"`
	AccountID        string   `json:"account_id"`
	AllowFrom        []string `json:"allow_from"`
}

// WeixinConfig is loaded from the plugin InitRequest.ConfigJSON (DMR serializes plugin config from TOML into JSON).
// Prefer `bots`; legacy single-bot flat keys are folded into bots[0] when `bots` is empty.
type WeixinConfig struct {
	ConfigBaseDir string `json:"config_base_dir"`
	Workspace     string `json:"workspace"`

	Bots []WeixinBotConfig `json:"bots"`

	// Legacy single-bot flat fields (used when bots is omitted).
	GatewayBaseURL   string   `json:"gateway_base_url"`
	CDNBaseURL       string   `json:"cdn_base_url"`
	Token            string   `json:"token"`
	CredentialsPath  string   `json:"credentials_path"`
	SKRouteTag       string   `json:"sk_route_tag"`
	ChannelVersion   string   `json:"channel_version"`
	AccountID        string   `json:"account_id"`
	AllowFrom        []string `json:"allow_from"`
	ApprovalTimeoutSec int    `json:"approval_timeout_sec"`
	DedupTTLMinutes    int    `json:"dedup_ttl_minutes"`
	ExtraPrompt        string `json:"extra_prompt"`
	ExtraPromptFile    string `json:"extra_prompt_file"`
}

func defaultWeixinConfig() WeixinConfig {
	return WeixinConfig{
		ApprovalTimeoutSec: 300,
		DedupTTLMinutes:    10,
	}
}

func parseWeixinConfig(jsonStr string) (WeixinConfig, error) {
	cfg := defaultWeixinConfig()
	if jsonStr == "" {
		if err := finalizeWeixinConfig(&cfg); err != nil {
			return cfg, err
		}
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
	if err := finalizeWeixinConfig(&cfg); err != nil {
		return cfg, err
	}
	return cfg, nil
}

func inheritLegacyIntoBot(dst *WeixinBotConfig, root *WeixinConfig) {
	if strings.TrimSpace(dst.GatewayBaseURL) == "" {
		dst.GatewayBaseURL = root.GatewayBaseURL
	}
	if strings.TrimSpace(dst.CDNBaseURL) == "" {
		dst.CDNBaseURL = root.CDNBaseURL
	}
	if strings.TrimSpace(dst.Token) == "" {
		dst.Token = root.Token
	}
	if strings.TrimSpace(dst.CredentialsPath) == "" {
		dst.CredentialsPath = root.CredentialsPath
	}
	if strings.TrimSpace(dst.SKRouteTag) == "" {
		dst.SKRouteTag = root.SKRouteTag
	}
	if strings.TrimSpace(dst.ChannelVersion) == "" {
		dst.ChannelVersion = root.ChannelVersion
	}
	if strings.TrimSpace(dst.AccountID) == "" {
		dst.AccountID = root.AccountID
	}
	if len(dst.AllowFrom) == 0 && len(root.AllowFrom) > 0 {
		dst.AllowFrom = append([]string(nil), root.AllowFrom...)
	}
}

func finalizeWeixinConfig(cfg *WeixinConfig) error {
	if len(cfg.Bots) == 0 {
		cfg.Bots = []WeixinBotConfig{{}}
	}
	for i := range cfg.Bots {
		inheritLegacyIntoBot(&cfg.Bots[i], cfg)
		if err := mergeWeixinCredentialsBot(&cfg.Bots[i], cfg.ConfigBaseDir); err != nil {
			return fmt.Errorf("weixin bot #%d credentials: %w", i, err)
		}
		if raw := strings.TrimSpace(cfg.Bots[i].ID); raw != "" {
			safe, err := weixinlogin.SanitizeLoginID(raw)
			if err != nil {
				return fmt.Errorf("weixin bot #%d id: %w", i, err)
			}
			cfg.Bots[i].ID = safe
		}
		if strings.TrimSpace(cfg.Bots[i].AccountID) == "" {
			cfg.Bots[i].AccountID = "default"
		}
	}
	if err := validateWeixinBots(cfg); err != nil {
		return err
	}

	// Mirror first bot flat fields onto root so older code paths keep working.
	b0 := cfg.Bots[0]
	cfg.GatewayBaseURL = b0.GatewayBaseURL
	cfg.CDNBaseURL = b0.CDNBaseURL
	cfg.Token = b0.Token
	cfg.CredentialsPath = b0.CredentialsPath
	cfg.SKRouteTag = b0.SKRouteTag
	cfg.ChannelVersion = b0.ChannelVersion
	cfg.AccountID = b0.AccountID
	cfg.AllowFrom = append([]string(nil), b0.AllowFrom...)
	return nil
}

func validateWeixinBots(cfg *WeixinConfig) error {
	if len(cfg.Bots) == 0 {
		return fmt.Errorf("weixin: no bots configured")
	}
	if len(cfg.Bots) > 1 {
		seen := make(map[string]struct{}, len(cfg.Bots))
		for i := range cfg.Bots {
			id := strings.TrimSpace(cfg.Bots[i].ID)
			if id == "" {
				return fmt.Errorf("weixin: bot #%d: id is required when multiple bots are configured", i)
			}
			if _, dup := seen[id]; dup {
				return fmt.Errorf("weixin: duplicate bot id %q", id)
			}
			seen[id] = struct{}{}
		}
	}
	for i := range cfg.Bots {
		if strings.TrimSpace(cfg.Bots[i].GatewayBaseURL) == "" {
			return fmt.Errorf("weixin: bot #%d: gateway_base_url is required", i)
		}
		if strings.TrimSpace(cfg.Bots[i].Token) == "" {
			return fmt.Errorf("weixin: bot #%d: token is required", i)
		}
	}
	return nil
}

type weixinCredentialsFile struct {
	GatewayBaseURL string `json:"gateway_base_url"`
	CDNBaseURL     string `json:"cdn_base_url"`
	Token          string `json:"token"`
	AccountID      string `json:"account_id"`
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

func mergeWeixinCredentialsBot(bot *WeixinBotConfig, configBaseDir string) error {
	p := strings.TrimSpace(bot.CredentialsPath)
	if p == "" {
		return nil
	}
	p = expandHomePath(p)
	abs := resolveExtraPromptPath(p, configBaseDir)
	b, err := os.ReadFile(abs)
	if err != nil {
		return fmt.Errorf("credentials_path %q: %w", p, err)
	}
	var f weixinCredentialsFile
	if err := json.Unmarshal(b, &f); err != nil {
		return fmt.Errorf("credentials_path %q: %w", p, err)
	}
	if s := strings.TrimSpace(f.GatewayBaseURL); s != "" {
		bot.GatewayBaseURL = s
	}
	if s := strings.TrimSpace(f.CDNBaseURL); s != "" {
		bot.CDNBaseURL = s
	}
	if s := strings.TrimSpace(f.Token); s != "" {
		bot.Token = s
	}
	if s := strings.TrimSpace(f.AccountID); s != "" {
		bot.AccountID = s
	}
	return nil
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

func resolvePathAgainstConfigDir(path, configBaseDir string) (string, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return "", fmt.Errorf("empty path")
	}
	path = expandHomePath(path)
	if filepath.IsAbs(path) {
		return filepath.Abs(filepath.Clean(path))
	}
	base := strings.TrimSpace(configBaseDir)
	if base == "" {
		return filepath.Abs(path)
	}
	return filepath.Abs(filepath.Join(base, path))
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

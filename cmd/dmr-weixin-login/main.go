// Command dmr-weixin-login: QR login to Weixin ilink and write credentials JSON for dmr-plugin-weixin.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/seanly/dmr-plugin-weixin/internal/weixinlogin"
)

type credFile struct {
	GatewayBaseURL string `json:"gateway_base_url"`
	CDNBaseURL     string `json:"cdn_base_url"`
	Token          string `json:"token"`
	IlinkBotID     string `json:"ilink_bot_id,omitempty"`
	IlinkUserID    string `json:"ilink_user_id,omitempty"`
	SavedAt        string `json:"saved_at"`
}

func main() {
	api := flag.String("api", weixinlogin.DefaultAPIBaseURL, "ilink API base URL")
	cdn := flag.String("cdn", weixinlogin.DefaultCDNBaseURL, "CDN base URL (written to credentials)")
	out := flag.String("o", defaultOutPath(), "output credentials JSON path")
	botType := flag.String("bot-type", weixinlogin.DefaultBotType, "bot_type query param")
	skTag := flag.String("sk-route-tag", "", "optional SKRouteTag header")
	wait := flag.Duration("wait", 8*time.Minute, "max time to wait for scan + confirm")
	noTermQR := flag.Bool("no-terminal-qr", false, "only print the liteapp URL, do not draw a QR in the terminal")
	flag.Parse()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	res, err := weixinlogin.Run(ctx, weixinlogin.Options{
		APIBaseURL:   *api,
		BotType:      *botType,
		SKRouteTag:   *skTag,
		TotalWait:    *wait,
		Stdout:       os.Stderr,
		NoTerminalQR: *noTermQR,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "login failed: %v\n", err)
		os.Exit(1)
	}

	if *cdn != "" {
		res.CDNBaseURL = strings.TrimSpace(*cdn)
	}

	data := credFile{
		GatewayBaseURL: res.GatewayBaseURL,
		CDNBaseURL:     res.CDNBaseURL,
		Token:          res.Token,
		IlinkBotID:     res.IlinkBotID,
		IlinkUserID:    res.IlinkUserID,
		SavedAt:        time.Now().UTC().Format(time.RFC3339),
	}
	b, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		fmt.Fprintf(os.Stderr, "marshal: %v\n", err)
		os.Exit(1)
	}
	path := expandHome(*out)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		fmt.Fprintf(os.Stderr, "mkdir: %v\n", err)
		os.Exit(1)
	}
	if err := os.WriteFile(path, b, 0o600); err != nil {
		fmt.Fprintf(os.Stderr, "write %s: %v\n", path, err)
		os.Exit(1)
	}
	fmt.Fprintf(os.Stderr, "saved credentials to %s\n", path)
	fmt.Fprintf(os.Stderr, "set in dmr config:\n  credentials_path: %s\n", path)
}

func defaultOutPath() string {
	h, _ := os.UserHomeDir()
	if h == "" {
		return ".dmr/weixin/credentials.json"
	}
	return filepath.Join(h, ".dmr", "weixin", "credentials.json")
}

func expandHome(p string) string {
	if strings.HasPrefix(p, "~/") {
		h, _ := os.UserHomeDir()
		if h != "" {
			return filepath.Join(h, p[2:])
		}
	}
	return p
}

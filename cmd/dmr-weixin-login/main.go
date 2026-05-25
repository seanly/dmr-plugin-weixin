// dmr-weixin-login performs Tencent ilink QR login and writes credentials JSON for the weixin plugin.
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
	"time"

	"github.com/seanly/dmr-plugin-weixin/internal/weixinlogin"
)

func main() {
	wait := flag.Duration("wait", 8*time.Minute, "max time waiting for QR scan and confirmation")
	api := flag.String("api", "", "ilink API root (empty = "+weixinlogin.DefaultAPIBaseURL+")")
	botType := flag.String("bot-type", "", "ilink bot_type (empty = "+weixinlogin.DefaultBotType+")")
	sk := flag.String("sk-route-tag", "", "optional SKRouteTag header")
	id := flag.String("id", "", "account label: default credentials path becomes ~/.dmr/var/lib/weixin/<id>/credentials.json ; also writes account_id in JSON (omit for ~/.dmr/var/lib/weixin/credentials.json)")
	out := flag.String("out", "", "path to write credentials JSON (empty = standard path derived from --id)")
	noTQ := flag.Bool("no-terminal-qr", false, "do not render QR as Unicode blocks on stdout")

	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage:\n\n  %s [flags]\n\nScans QR with WeChat, then writes credential overlay for DMR's weixin plugin.\nFlags:\n", filepath.Base(os.Args[0]))
		flag.PrintDefaults()
	}
	flag.Parse()

	safeID, err := weixinlogin.SanitizeLoginID(*id)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		os.Exit(2)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	res, err := weixinlogin.Run(ctx, weixinlogin.Options{
		APIBaseURL:   strings.TrimSpace(*api),
		BotType:      strings.TrimSpace(*botType),
		SKRouteTag:   strings.TrimSpace(*sk),
		TotalWait:    *wait,
		Stdout:       os.Stderr,
		NoTerminalQR: *noTQ,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "login failed: %v\n", err)
		os.Exit(1)
	}

	payload := map[string]string{
		"gateway_base_url": strings.TrimSpace(res.GatewayBaseURL),
		"cdn_base_url":     strings.TrimSpace(res.CDNBaseURL),
		"token":            strings.TrimSpace(res.Token),
	}
	if safeID != "" {
		payload["account_id"] = safeID
	}
	b, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		fmt.Fprintf(os.Stderr, "encode json: %v\n", err)
		os.Exit(1)
	}
	outPath := strings.TrimSpace(*out)
	if outPath == "" {
		outPath = weixinlogin.DefaultCredentialsPath(safeID)
	}
	if err := os.MkdirAll(filepath.Dir(outPath), 0700); err != nil {
		fmt.Fprintf(os.Stderr, "mkdir %q: %v\n", filepath.Dir(outPath), err)
		os.Exit(1)
	}
	if err := os.WriteFile(outPath, append(b, '\n'), 0600); err != nil {
		fmt.Fprintf(os.Stderr, "write %q: %v\n", outPath, err)
		os.Exit(1)
	}
	fmt.Fprintf(os.Stderr, " wrote %s (ilink_bot_id=%s ilink_user_id=%s)\n", outPath, res.IlinkBotID, res.IlinkUserID)
}

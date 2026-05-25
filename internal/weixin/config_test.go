package weixin

import (
	"os"
	"path/filepath"
	"testing"
)

func TestMergeWeixinCredentialsBot(t *testing.T) {
	dir := t.TempDir()
	credPath := filepath.Join(dir, "c.json")
	if err := os.WriteFile(credPath, []byte(`{
  "gateway_base_url": "https://gw.from.file",
  "cdn_base_url": "https://cdn.from.file",
  "token": "secret-from-file"
}`), 0o600); err != nil {
		t.Fatal(err)
	}
	bot := WeixinBotConfig{
		GatewayBaseURL:   "https://yaml-gw",
		CDNBaseURL:       "https://yaml-cdn",
		Token:            "yaml-token",
		CredentialsPath:  "c.json",
	}
	if err := mergeWeixinCredentialsBot(&bot, dir); err != nil {
		t.Fatal(err)
	}
	if bot.GatewayBaseURL != "https://gw.from.file" || bot.CDNBaseURL != "https://cdn.from.file" || bot.Token != "secret-from-file" {
		t.Fatalf("%+v", bot)
	}

	credWithAcct := filepath.Join(dir, "with_acct.json")
	if err := os.WriteFile(credWithAcct, []byte(`{"gateway_base_url":"https://g","cdn_base_url":"https://c","token":"t","account_id":"acct2"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	bot2 := WeixinBotConfig{AccountID: "yaml-default", CredentialsPath: "with_acct.json"}
	if err := mergeWeixinCredentialsBot(&bot2, dir); err != nil {
		t.Fatal(err)
	}
	if bot2.AccountID != "acct2" || bot2.GatewayBaseURL != "https://g" {
		t.Fatalf("%+v", bot2)
	}
}

package weixinlogin

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

var loginIDRegexp = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._-]{0,63}$`)

// SanitizeLoginID returns a filesystem-safe subdirectory name for ~/.dmr/var/lib/weixin/<id>/credentials.json .
// Empty input means "single default account" (no subdirectory). Non-empty IDs must match loginIDRegexp.
func SanitizeLoginID(s string) (string, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return "", nil
	}
	if loginIDRegexp.MatchString(s) {
		return s, nil
	}
	return "", fmt.Errorf("login --id must be empty or 1-64 chars matching [a-zA-Z0-9][a-zA-Z0-9._-]* (got %q)", s)
}

// DefaultCredentialsPath is the implicit -out location for dmr-weixin-login after a successful QR login.
// id must already be sanitized (SanitizeLoginID); empty id uses ~/.dmr/var/lib/weixin/credentials.json .
func DefaultCredentialsPath(id string) string {
	h, err := os.UserHomeDir()
	if err != nil || strings.TrimSpace(h) == "" {
		if strings.TrimSpace(id) == "" {
			return filepath.Join(os.TempDir(), "weixin-credentials.json")
		}
		return filepath.Join(os.TempDir(), "dmr-weixin", id, "credentials.json")
	}
	base := filepath.Join(h, ".dmr", "var", "lib", "weixin")
	if strings.TrimSpace(id) == "" {
		return filepath.Join(base, "credentials.json")
	}
	return filepath.Join(base, id, "credentials.json")
}

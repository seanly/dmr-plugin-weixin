package weixinlogin

import (
	"fmt"
	"io"
	"strings"

	"github.com/mdp/qrterminal/v3"
	"rsc.io/qr"
)

// WriteTerminalQR renders payload as a scannable QR on w (e.g. os.Stderr).
// Use the full liteapp URL from get_bot_qrcode (qrcode_img_content): system browsers
// often do not paint that page’s QR; encoding the same URL here works with WeChat scan.
func WriteTerminalQR(w io.Writer, payload string) {
	if w == nil {
		return
	}
	s := strings.TrimSpace(payload)
	if s == "" {
		return
	}
	if _, err := qr.Encode(s, qr.L); err != nil {
		_, _ = fmt.Fprintf(w, "(terminal QR skipped: payload too long or invalid for QR: %v)\n", err)
		return
	}
	_, _ = fmt.Fprintln(w)
	qrterminal.GenerateHalfBlock(s, qrterminal.L, w)
	_, _ = fmt.Fprintln(w)
}

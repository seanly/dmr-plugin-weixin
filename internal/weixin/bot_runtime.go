package weixin

import (
	"fmt"
	"strings"
	"sync"

	"github.com/seanly/dmr-plugin-weixin/internal/weixinlogin"
)

// weixinBot holds one bot identity's gateway credentials, outbound state caches, and API helpers.
type weixinBot struct {
	wp *WeixinPlugin

	// tapeLabel is the routing segment in weixin:<label>:p2p:<peer> ; empty uses legacy weixin:p2p:<peer> only when exactly one bot is configured.
	tapeLabel string

	gatewayBaseURL string
	cdnBaseURL     string
	token          string
	skRouteTag     string
	channelVer     string
	accountID      string
	allowFrom      []string

	tokens *contextTokenStore

	sessionMu         sync.Mutex
	lastSessionByPeer map[string]string

	typingMu     sync.Mutex
	typingByPeer map[string]string

	recentMediaMu     sync.Mutex
	recentMediaByPeer map[string][]InboundAttachment
}

func newWeixinBots(p *WeixinPlugin, cfg WeixinConfig) ([]*weixinBot, error) {
	if len(cfg.Bots) == 0 {
		return nil, fmt.Errorf("weixin: no bots configured")
	}
	out := make([]*weixinBot, 0, len(cfg.Bots))
	for i := range cfg.Bots {
		b := cfg.Bots[i]
		rawLabel := strings.TrimSpace(b.ID)
		if rawLabel != "" {
			safe, err := weixinlogin.SanitizeLoginID(rawLabel)
			if err != nil {
				return nil, fmt.Errorf("weixin: bot #%d invalid id: %w", i, err)
			}
			rawLabel = safe
		}
		wb := &weixinBot{
			wp:                  p,
			tapeLabel:           rawLabel,
			gatewayBaseURL:      strings.TrimSpace(b.GatewayBaseURL),
			cdnBaseURL:          strings.TrimSpace(b.CDNBaseURL),
			token:               strings.TrimSpace(b.Token),
			skRouteTag:          strings.TrimSpace(b.SKRouteTag),
			channelVer:          strings.TrimSpace(b.ChannelVersion),
			accountID:           strings.TrimSpace(b.AccountID),
			allowFrom:           append([]string(nil), b.AllowFrom...),
			tokens:              newContextTokenStore(),
			lastSessionByPeer:   make(map[string]string),
			typingByPeer:        make(map[string]string),
			recentMediaByPeer:   make(map[string][]InboundAttachment),
		}
		if wb.accountID == "" {
			wb.accountID = "default"
		}
		out = append(out, wb)
	}
	return out, nil
}

func (p *WeixinPlugin) multiBot() bool {
	p.botsMu.RLock()
	defer p.botsMu.RUnlock()
	return len(p.bots) > 1
}

func (p *WeixinPlugin) botByTapeLabel(label string) (*weixinBot, error) {
	label = strings.TrimSpace(label)
	p.botsMu.RLock()
	defer p.botsMu.RUnlock()
	if len(p.bots) == 0 {
		return nil, fmt.Errorf("weixin: no bots initialized")
	}
	if len(p.bots) == 1 {
		b := p.bots[0]
		if label == "" || label == b.tapeLabel {
			return b, nil
		}
		return nil, fmt.Errorf("weixin: unknown bot id %q", label)
	}
	if label == "" {
		return nil, fmt.Errorf("weixin: tape must include bot id when multiple bots are configured (weixin:<id>:p2p:<peer>)")
	}
	for _, b := range p.bots {
		if b.tapeLabel == label {
			return b, nil
		}
	}
	return nil, fmt.Errorf("weixin: unknown bot id %q", label)
}

func (p *WeixinPlugin) tapeForPeer(peerID string, bot *weixinBot) string {
	if bot == nil {
		return legacyTape(peerID)
	}
	if bot.tapeLabel != "" {
		return fmt.Sprintf("weixin:%s:p2p:%s", bot.tapeLabel, peerID)
	}
	return legacyTape(peerID)
}

func legacyTape(peerID string) string {
	return "weixin:p2p:" + peerID
}

func approvalWaitKey(bot *weixinBot, peerID string) string {
	peerID = strings.TrimSpace(peerID)
	if bot == nil || bot.tapeLabel == "" {
		return peerID
	}
	return bot.tapeLabel + "\x1e" + peerID
}

func dedupScopedKey(bot *weixinBot, raw string, multi bool) string {
	if raw == "" {
		return ""
	}
	if !multi || bot == nil || bot.tapeLabel == "" {
		return raw
	}
	return bot.tapeLabel + "|" + raw
}

func (b *weixinBot) rememberSessionForPeer(peerID, sessionID string) {
	peerID = strings.TrimSpace(peerID)
	sessionID = strings.TrimSpace(sessionID)
	if peerID == "" || sessionID == "" {
		return
	}
	b.sessionMu.Lock()
	defer b.sessionMu.Unlock()
	if b.lastSessionByPeer == nil {
		b.lastSessionByPeer = make(map[string]string)
	}
	b.lastSessionByPeer[peerID] = sessionID
}

func (b *weixinBot) sessionIDForPeer(peerID string) string {
	b.sessionMu.Lock()
	defer b.sessionMu.Unlock()
	if b.lastSessionByPeer == nil {
		return ""
	}
	return strings.TrimSpace(b.lastSessionByPeer[strings.TrimSpace(peerID)])
}

func (b *weixinBot) rememberTypingTicket(peerID, ticket string) {
	peerID = strings.TrimSpace(peerID)
	ticket = strings.TrimSpace(ticket)
	if peerID == "" {
		return
	}
	b.typingMu.Lock()
	defer b.typingMu.Unlock()
	if b.typingByPeer == nil {
		b.typingByPeer = make(map[string]string)
	}
	if ticket != "" {
		b.typingByPeer[peerID] = ticket
	}
}

func (b *weixinBot) typingTicketForPeer(peerID string) string {
	b.typingMu.Lock()
	defer b.typingMu.Unlock()
	if b.typingByPeer == nil {
		return ""
	}
	return strings.TrimSpace(b.typingByPeer[strings.TrimSpace(peerID)])
}

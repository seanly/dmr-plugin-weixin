package weixin

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/rpc"
	"strings"
	"sync"

	"github.com/seanly/dmr/pkg/plugin/proto"
)

// WeixinPlugin implements proto.DMRPluginInterface and proto.HostClientSetter.
type WeixinPlugin struct {
	cfg WeixinConfig

	hostMu     sync.Mutex
	hostClient *rpc.Client

	runMu    sync.Mutex
	runCtx   context.Context
	cancel   context.CancelFunc
	shutdown sync.Once

	dedup    *deduper
	approver *WeixinApprover
	queues   *queueManager
	tokens   *contextTokenStore

	// lastSessionByPeer stores session_id from inbound msgs; outbound sendmessage may need it for file/media delivery.
	sessionMu         sync.Mutex
	lastSessionByPeer map[string]string

	// typingByPeer: typing_ticket from last successful getconfig per peer; used with sendtyping around sendmessage.
	typingMu     sync.Mutex
	typingByPeer map[string]string

	// recentMediaByPeer: last saved media attachments per peer; used as fallback
	// when ref_msg doesn't carry CDN data (gateway limitation).
	recentMediaMu     sync.Mutex
	recentMediaByPeer map[string][]InboundAttachment

	extraRunPrompt string
}

func NewWeixinPlugin() *WeixinPlugin {
	p := &WeixinPlugin{
		cfg:    defaultWeixinConfig(),
		tokens: newContextTokenStore(),
	}
	p.approver = newWeixinApprover(p)
	p.queues = newQueueManager(p)
	return p
}

func (p *WeixinPlugin) rememberSessionForPeer(peerID, sessionID string) {
	peerID = strings.TrimSpace(peerID)
	sessionID = strings.TrimSpace(sessionID)
	if peerID == "" || sessionID == "" {
		return
	}
	p.sessionMu.Lock()
	defer p.sessionMu.Unlock()
	if p.lastSessionByPeer == nil {
		p.lastSessionByPeer = make(map[string]string)
	}
	p.lastSessionByPeer[peerID] = sessionID
}

func (p *WeixinPlugin) sessionIDForPeer(peerID string) string {
	p.sessionMu.Lock()
	defer p.sessionMu.Unlock()
	if p.lastSessionByPeer == nil {
		return ""
	}
	return strings.TrimSpace(p.lastSessionByPeer[strings.TrimSpace(peerID)])
}

func (p *WeixinPlugin) rememberTypingTicket(peerID, ticket string) {
	peerID = strings.TrimSpace(peerID)
	ticket = strings.TrimSpace(ticket)
	if peerID == "" {
		return
	}
	p.typingMu.Lock()
	defer p.typingMu.Unlock()
	if p.typingByPeer == nil {
		p.typingByPeer = make(map[string]string)
	}
	if ticket != "" {
		p.typingByPeer[peerID] = ticket
	}
}

func (p *WeixinPlugin) typingTicketForPeer(peerID string) string {
	p.typingMu.Lock()
	defer p.typingMu.Unlock()
	if p.typingByPeer == nil {
		return ""
	}
	return strings.TrimSpace(p.typingByPeer[strings.TrimSpace(peerID)])
}

func (p *WeixinPlugin) SetHostClient(client any) {
	c, ok := client.(*rpc.Client)
	if !ok || c == nil {
		log.Printf("weixin: SetHostClient: unexpected client type %T", client)
		return
	}
	p.hostMu.Lock()
	p.hostClient = c
	p.hostMu.Unlock()
	log.Printf("weixin: host RPC client attached")
}

func (p *WeixinPlugin) Init(req *proto.InitRequest, resp *proto.InitResponse) error {
	cfg, err := parseWeixinConfig(req.ConfigJSON)
	if err != nil {
		return fmt.Errorf("parse config: %w", err)
	}
	p.cfg = cfg

	if strings.TrimSpace(cfg.GatewayBaseURL) == "" {
		return fmt.Errorf("weixin: gateway_base_url is required")
	}
	if strings.TrimSpace(cfg.Token) == "" {
		return fmt.Errorf("weixin: token is required")
	}

	resolvedExtra, err := buildResolvedExtraPrompt(cfg)
	if err != nil {
		return fmt.Errorf("weixin: %w", err)
	}
	p.extraRunPrompt = resolvedExtra
	if resolvedExtra != "" {
		log.Printf("weixin: extra run prompt enabled (%d bytes)", len(resolvedExtra))
	}

	p.dedup = newDeduper(cfg.dedupTTL())

	p.runMu.Lock()
	if p.cancel != nil {
		p.cancel()
	}
	ctx, cancel := context.WithCancel(context.Background())
	p.runCtx = ctx
	p.cancel = cancel
	p.runMu.Unlock()

	go p.monitorLoop(ctx)
	return nil
}

func (p *WeixinPlugin) Shutdown(req *proto.ShutdownRequest, resp *proto.ShutdownResponse) error {
	p.shutdown.Do(func() {
		p.runMu.Lock()
		if p.cancel != nil {
			p.cancel()
			p.cancel = nil
		}
		p.runMu.Unlock()
		if p.queues != nil {
			p.queues.shutdown()
		}
	})
	return nil
}

func (p *WeixinPlugin) RequestApproval(req *proto.ApprovalRequest, resp *proto.ApprovalResult) error {
	if p.approver == nil {
		resp.Choice = choiceDenied
		return nil
	}
	p.approver.handleSingle(req, resp)
	return nil
}

func (p *WeixinPlugin) RequestBatchApproval(req *proto.BatchApprovalRequest, resp *proto.BatchApprovalResult) error {
	if p.approver == nil {
		resp.Choice = choiceDenied
		return nil
	}
	p.approver.handleBatch(req, resp)
	return nil
}

// ProvideSystemPrompt returns an optional system prompt fragment (inbound UX hints stay in composeRunPrompt).
func (p *WeixinPlugin) ProvideSystemPrompt(req *proto.ProvideSystemPromptRequest, resp *proto.ProvideSystemPromptResponse) error {
	_ = req
	resp.Fragment = ""
	return nil
}

func (p *WeixinPlugin) ProvideTools(req *proto.ProvideToolsRequest, resp *proto.ProvideToolsResponse) error {
	resp.Tools = []proto.ToolDef{
		{
			Name:           "weixinSendText",
			Description:    "Send plain text to current Weixin peer, or use tape_name weixin:p2p:<id> / peer_id for cron-fired runs (no inbound context). Requires prior context_token (user messaged bot).",
			ParametersJSON: sendTextToolParamsJSON(),
			Group:          "extended",
			SearchHint:     "weixin, send, text, message, chat, im, 微信, 发送, 消息",
		},
		{
			Name:           "weixinSendFile",
			Description:    "Send files to a Weixin peer (image, video, generic attachment) from local path or http(s) URL. Use file_type and/or media_type: auto/image/video/file, or CDN ilink ints 1–4 (voice/4 outbound not supported); both keys must agree if both set.",
			ParametersJSON: sendFileToolParamsJSON(),
			Group:          "extended",
			SearchHint:     "weixin, send, file, media_type, file_type, attachment, image, video, cdn, 微信, 发送, 文件",
		},
	}
	log.Printf("weixin: ProvideTools -> weixinSendText, weixinSendFile")
	return nil
}

func (p *WeixinPlugin) CallTool(req *proto.CallToolRequest, resp *proto.CallToolResponse) error {
	ctx := context.Background()
	p.runMu.Lock()
	if p.runCtx != nil {
		ctx = p.runCtx
	}
	p.runMu.Unlock()

	// Parse context from the request (passed from RunAgent)
	toolCtx := make(map[string]any)
	if req.ContextJSON != "" {
		if err := json.Unmarshal([]byte(req.ContextJSON), &toolCtx); err != nil {
			log.Printf("weixin: CallTool %s failed to parse context JSON: %v", req.Name, err)
		}
	}

	// Extract peer_id from context or session tape
	peerID, _ := toolCtx["peer_id"].(string)
	if peerID == "" {
		// Fallback: try to extract from session tape (e.g., "weixin:p2p:wxid_xxx")
		peerID = weixinP2PTapeToPeerIDOrEmpty(req.SessionTape)
	}

	if peerID != "" {
		log.Printf("weixin: CallTool %s peer_id=%q", req.Name, peerID)
	} else {
		log.Printf("weixin: CallTool %s (no peer_id in context or tape %q)", req.Name, req.SessionTape)
	}

	switch req.Name {
	case "weixinSendText":
		result, err := p.execSendText(ctx, req.ArgsJSON, toolCtx)
		if err != nil {
			resp.Error = err.Error()
			return nil
		}
		b, err := json.Marshal(result)
		if err != nil {
			resp.Error = err.Error()
			return nil
		}
		resp.ResultJSON = string(b)
		return nil
	case "weixinSendFile":
		result, err := p.execSendFile(ctx, req.ArgsJSON, toolCtx)
		if err != nil {
			resp.Error = err.Error()
			return nil
		}
		b, err := json.Marshal(result)
		if err != nil {
			resp.Error = err.Error()
			return nil
		}
		resp.ResultJSON = string(b)
		return nil
	default:
		resp.Error = fmt.Sprintf("unknown tool: %s", req.Name)
		return nil
	}
}

// weixinP2PTapeToPeerIDOrEmpty extracts peer_id from weixin:p2p:<peer_id> tape name.
// Returns empty string if not a valid weixin p2p tape.
func weixinP2PTapeToPeerIDOrEmpty(tapeName string) string {
	const prefix = "weixin:p2p:"
	s := strings.TrimSpace(tapeName)
	if s == "" {
		return ""
	}
	if !strings.HasPrefix(s, prefix) {
		return ""
	}
	return strings.TrimSpace(s[len(prefix):])
}

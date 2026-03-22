package main

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

	activeJobMu sync.Mutex
	activeJob   *inboundJob

	// lastSessionByPeer stores session_id from inbound msgs; outbound sendmessage may need it for file/media delivery.
	sessionMu         sync.Mutex
	lastSessionByPeer map[string]string

	// typingByPeer: typing_ticket from last successful getconfig per peer; used with sendtyping around sendmessage.
	typingMu     sync.Mutex
	typingByPeer map[string]string

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

func (p *WeixinPlugin) setActiveJob(job *inboundJob) {
	p.activeJobMu.Lock()
	p.activeJob = job
	p.activeJobMu.Unlock()
}

func (p *WeixinPlugin) clearActiveJob() {
	p.activeJobMu.Lock()
	p.activeJob = nil
	p.activeJobMu.Unlock()
}

func (p *WeixinPlugin) getActiveJob() *inboundJob {
	p.activeJobMu.Lock()
	defer p.activeJobMu.Unlock()
	return p.activeJob
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
	if strings.TrimSpace(cfg.DmrRestartToken) != "" && len(cfg.AllowFrom) == 0 {
		return fmt.Errorf("weixin: dmr_restart_token requires allow_from")
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

func (p *WeixinPlugin) ProvideTools(req *proto.ProvideToolsRequest, resp *proto.ProvideToolsResponse) error {
	resp.Tools = []proto.ToolDef{
		{
			Name:           "weixinSendText",
			Description:    "Send plain text to current Weixin peer, or use tape_name weixin:p2p:<id> / peer_id for cron. Requires prior context_token (user messaged bot).",
			ParametersJSON: sendTextToolParamsJSON(),
		},
		{
			Name:           "weixinSendMedia",
			Description:    "Send image/video/file to current Weixin peer. Supports local files and remote URLs (http/https). Auto-detects media type from extension or use media_type parameter.",
			ParametersJSON: sendMediaToolParamsJSON(),
		},
	}
	log.Printf("weixin: ProvideTools -> weixinSendText, weixinSendMedia")
	return nil
}

func (p *WeixinPlugin) CallTool(req *proto.CallToolRequest, resp *proto.CallToolResponse) error {
	ctx := context.Background()
	p.runMu.Lock()
	if p.runCtx != nil {
		ctx = p.runCtx
	}
	p.runMu.Unlock()

	switch req.Name {
	case "weixinSendText":
		result, err := p.execSendText(ctx, req.ArgsJSON)
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
	case "weixinSendMedia":
		result, err := p.execSendMedia(ctx, req.ArgsJSON)
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

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
	if strings.TrimSpace(cfg.CDNBaseURL) == "" {
		return fmt.Errorf("weixin: cdn_base_url is required")
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
			Name:        "weixin.send_file",
			Description: "Deliver reports/files: upload local file to Weixin CDN and send to current p2p peer. Only during Weixin-triggered RunAgent. Path under send_file_root or cwd.",
			ParametersJSON: sendFileToolParamsJSON(),
		},
		{
			Name:        "weixin.send_text",
			Description: "Send plain text to current Weixin peer, or use tape_name weixin:p2p:<id> / peer_id for cron. Requires prior context_token (user messaged bot).",
			ParametersJSON: sendTextToolParamsJSON(),
		},
	}
	log.Printf("weixin: ProvideTools -> weixin.send_file, weixin.send_text")
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
	case "weixin.send_file":
		result, err := p.execSendFile(ctx, req.ArgsJSON)
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
	case "weixin.send_text":
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
	default:
		resp.Error = fmt.Sprintf("unknown tool: %s", req.Name)
		return nil
	}
}

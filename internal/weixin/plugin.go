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

	botsMu sync.RWMutex
	bots   []*weixinBot

	runMu    sync.Mutex
	runCtx   context.Context
	cancel   context.CancelFunc
	shutdown sync.Once

	dedup    *deduper
	approver *WeixinApprover
	queues   *queueManager

	extraRunPrompt string
}

func NewWeixinPlugin() *WeixinPlugin {
	p := &WeixinPlugin{
		cfg: defaultWeixinConfig(),
	}
	p.approver = newWeixinApprover(p)
	p.queues = newQueueManager(p)
	return p
}

func (p *WeixinPlugin) botForToolContext(toolCtx map[string]any, sessionTape string) (*weixinBot, error) {
	if toolCtx != nil {
		if s, ok := toolCtx["weixin_bot"].(string); ok && strings.TrimSpace(s) != "" {
			return p.botByTapeLabel(strings.TrimSpace(s))
		}
	}
	_, botLabel, ok := parseWeixinP2PTape(sessionTape)
	if !ok && strings.TrimSpace(sessionTape) != "" {
		return nil, fmt.Errorf("not a Weixin session tape")
	}
	return p.botByTapeLabel(botLabel)
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

	if strings.TrimSpace(cfg.GatewayBaseURL) == "" || strings.TrimSpace(cfg.Token) == "" {
		return fmt.Errorf("weixin: gateway_base_url and token are required (per bot)")
	}

	resolvedExtra, err := buildResolvedExtraPrompt(cfg)
	if err != nil {
		return fmt.Errorf("weixin: %w", err)
	}
	p.extraRunPrompt = resolvedExtra
	if resolvedExtra != "" {
		log.Printf("weixin: extra run prompt enabled (%d bytes)", len(resolvedExtra))
	}

	bots, err := newWeixinBots(p, cfg)
	if err != nil {
		return err
	}
	p.botsMu.Lock()
	p.bots = bots
	p.botsMu.Unlock()

	for i, b := range bots {
		if b.tapeLabel != "" {
			log.Printf("weixin: initialized bot #%d id=%q", i, b.tapeLabel)
		} else {
			log.Printf("weixin: initialized bot #%d (legacy tape weixin:p2p:<peer>)", i)
		}
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

	for _, b := range bots {
		go p.monitorLoop(ctx, b)
	}
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
	txt := sendTextToolParamsJSON()
	fl := sendFileToolParamsJSON()
	resp.Tools = []proto.ToolDef{
		{
			Name:           "weixinSendText",
			Description:    "Send plain text to current Weixin peer in this session; or use tape_name weixin:p2p:<peer> / weixin:<bot_id>:p2p:<peer> (multi-bot), or peer_id for cron-fired runs. Requires prior context_token (user messaged this bot).",
			ParametersJSON: txt,
			Group:          "extended",
			SearchHint:     "weixin, send, text, message, chat, im, 微信, 发送, 消息",
		},
		{
			Name:           "weixinSendFile",
			Description:    "Send files (image/video/file); same tape rules as weixinSendText for off-session peers.",
			ParametersJSON: fl,
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

	toolCtx := make(map[string]any)
	if req.ContextJSON != "" {
		if err := json.Unmarshal([]byte(req.ContextJSON), &toolCtx); err != nil {
			log.Printf("weixin: CallTool %s failed to parse context JSON: %v", req.Name, err)
		}
	}

	peerID, _ := toolCtx["peer_id"].(string)
	if peerID == "" {
		p2, _, ok := parseWeixinP2PTape(req.SessionTape)
		if ok {
			peerID = p2
		}
	}
	if peerID != "" {
		log.Printf("weixin: CallTool %s peer_id=%q", req.Name, peerID)
	} else {
		log.Printf("weixin: CallTool %s (no peer_id in context or tape %q)", req.Name, req.SessionTape)
	}

	switch req.Name {
	case "weixinSendText":
		result, err := p.execSendText(ctx, req.ArgsJSON, toolCtx, req.SessionTape)
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
		result, err := p.execSendFile(ctx, req.ArgsJSON, toolCtx, req.SessionTape)
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

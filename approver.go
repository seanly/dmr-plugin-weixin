package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/seanly/dmr/pkg/plugin/proto"
)

const (
	choiceDenied         int32 = 0
	choiceApprovedOnce   int32 = 1
	choiceApprovedSess   int32 = 2
	choiceApprovedAlways int32 = 3
)

const (
	maxApprovalContentRunesSingle = 8000
	maxApprovalContentRunesBatch  = 2500
	maxApprovalRestJSONRunes      = 6000
)

type approvalReply struct {
	choice  int32
	indices []int32
}

type approvalWait struct {
	ch     chan approvalReply
	batchN int
}

type WeixinApprover struct {
	plugin *WeixinPlugin
	mu     sync.Mutex
	wait   map[string]*approvalWait
}

func newWeixinApprover(p *WeixinPlugin) *WeixinApprover {
	return &WeixinApprover{
		plugin: p,
		wait:   make(map[string]*approvalWait),
	}
}

func parseApprovalChoice(content string) (int32, bool) {
	s := strings.TrimSpace(strings.ToLower(content))
	if s == "" {
		return choiceDenied, true
	}
	if utf8.RuneCountInString(s) == 1 {
		switch s[0] {
		case 'y':
			return choiceApprovedOnce, true
		case 's':
			return choiceApprovedSess, true
		case 'a':
			return choiceApprovedAlways, true
		case 'n':
			return choiceDenied, true
		default:
			return choiceDenied, true
		}
	}
	switch s {
	case "yes":
		return choiceApprovedOnce, true
	case "session":
		return choiceApprovedSess, true
	case "always":
		return choiceApprovedAlways, true
	case "no":
		return choiceDenied, true
	default:
		return choiceDenied, true
	}
}

func parseBatchApprovalChoice(content string, total int) (approvalReply, bool) {
	s := strings.TrimSpace(strings.ToLower(content))
	if s == "" {
		return approvalReply{choice: choiceDenied}, true
	}
	switch s {
	case "y", "yes":
		return approvalReply{choice: choiceApprovedOnce}, true
	case "s", "session":
		return approvalReply{choice: choiceApprovedSess}, true
	case "a", "always":
		return approvalReply{choice: choiceApprovedAlways}, true
	case "n", "no":
		return approvalReply{choice: choiceDenied}, true
	}
	if strings.Contains(s, ",") || isAllASCIIDigits(s) {
		indices, err := parseApprovalIndices(s, total)
		if err != nil {
			return approvalReply{choice: choiceDenied}, true
		}
		return approvalReply{choice: choiceApprovedOnce, indices: indices}, true
	}
	return approvalReply{choice: choiceDenied}, true
}

func isAllASCIIDigits(s string) bool {
	if s == "" {
		return false
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}

func parseApprovalIndices(input string, total int) ([]int32, error) {
	parts := strings.Split(input, ",")
	var indices []int32
	for _, p := range parts {
		p = strings.TrimSpace(p)
		n, err := strconv.Atoi(p)
		if err != nil || n < 1 || n > total {
			return nil, fmt.Errorf("invalid index: %s", p)
		}
		indices = append(indices, int32(n-1))
	}
	if len(indices) == 0 {
		return nil, fmt.Errorf("no indices")
	}
	return indices, nil
}

func (a *WeixinApprover) tryResolveP2P(peerID, content string) bool {
	a.mu.Lock()
	entry := a.wait[peerID]
	a.mu.Unlock()
	if entry == nil {
		return false
	}
	var reply approvalReply
	if entry.batchN == 0 {
		c, ok := parseApprovalChoice(content)
		if !ok {
			return false
		}
		reply = approvalReply{choice: c}
	} else {
		var ok bool
		reply, ok = parseBatchApprovalChoice(content, entry.batchN)
		if !ok {
			return false
		}
	}
	select {
	case entry.ch <- reply:
	default:
	}
	return true
}

func (a *WeixinApprover) resolveContextToken(peerID string) string {
	tok := a.plugin.tokens.get(peerID)
	if tok != "" {
		return tok
	}
	j := a.plugin.getActiveJob()
	if j != nil && j.PeerID == peerID {
		return strings.TrimSpace(j.ContextToken)
	}
	return ""
}

func (a *WeixinApprover) waitApproval(peerID, prompt string, batchN int) approvalReply {
	timeout := a.plugin.cfg.approvalTimeout()
	ch := make(chan approvalReply, 1)

	a.mu.Lock()
	if _, busy := a.wait[peerID]; busy {
		a.mu.Unlock()
		return approvalReply{choice: choiceDenied}
	}
	a.wait[peerID] = &approvalWait{ch: ch, batchN: batchN}
	a.mu.Unlock()

	defer func() {
		a.mu.Lock()
		delete(a.wait, peerID)
		a.mu.Unlock()
	}()

	ctx := a.plugin.runCtx
	if ctx == nil {
		ctx = context.Background()
	}
	tok := a.resolveContextToken(peerID)
	if tok == "" {
		log.Printf("weixin: approver no context_token for peer=%q", peerID)
		return approvalReply{choice: choiceDenied}
	}
	if err := a.plugin.sendApprovalText(ctx, peerID, tok, prompt); err != nil {
		log.Printf("weixin: approval send failed: %v", err)
		return approvalReply{choice: choiceDenied}
	}

	timer := time.NewTimer(timeout)
	defer timer.Stop()
	select {
	case v := <-ch:
		return v
	case <-timer.C:
		return approvalReply{choice: choiceDenied}
	case <-ctx.Done():
		return approvalReply{choice: choiceDenied}
	}
}

func formatApprovalArgsMarkdown(tool, argsJSON string, contentMaxRunes int) string {
	raw := strings.TrimSpace(argsJSON)
	if raw == "" {
		raw = "{}"
	}
	var args map[string]any
	if err := json.Unmarshal([]byte(raw), &args); err != nil {
		fallback := truncateRunes(raw, maxApprovalRestJSONRunes)
		return "### Arguments\n\n```json\n" + fallback + "\n```\n"
	}
	switch tool {
	case "shell":
		return formatShellArgsMarkdown(args, contentMaxRunes, maxApprovalRestJSONRunes)
	case "fsWrite", "fsEdit":
		return formatFsArgsMarkdown(args, contentMaxRunes, maxApprovalRestJSONRunes)
	default:
		return formatGenericArgsMarkdown(args, contentMaxRunes, maxApprovalRestJSONRunes)
	}
}

func formatShellArgsMarkdown(args map[string]any, cmdMaxRunes, restJSONMaxRunes int) string {
	cmd, _ := args["cmd"].(string)
	delete(args, "cmd")
	var b strings.Builder
	b.WriteString("### Command\n\n```\n")
	b.WriteString(truncateRunes(cmd, cmdMaxRunes))
	b.WriteString("\n```\n")
	if len(args) > 0 {
		b.WriteString("\n")
		b.WriteString(formatRemainingJSONMarkdown(args, restJSONMaxRunes))
	}
	return b.String()
}

func formatFsArgsMarkdown(args map[string]any, contentMaxRunes, restJSONMaxRunes int) string {
	path, _ := args["path"].(string)
	delete(args, "path")
	var b strings.Builder
	if path != "" {
		b.WriteString("### Path\n\n`")
		b.WriteString(path)
		b.WriteString("`\n\n")
	}
	if c, ok := args["content"].(string); ok && c != "" {
		b.WriteString("### File content\n\n")
		b.WriteString(truncateRunes(c, contentMaxRunes))
		b.WriteString("\n\n")
		delete(args, "content")
	}
	if len(args) > 0 {
		b.WriteString(formatRemainingJSONMarkdown(args, restJSONMaxRunes))
	} else if path == "" {
		b.WriteString("*(No path or content in args.)*\n")
	}
	return b.String()
}

func formatGenericArgsMarkdown(args map[string]any, contentMaxRunes, restJSONMaxRunes int) string {
	var b strings.Builder
	if c, ok := args["content"].(string); ok && c != "" {
		b.WriteString("### File content\n\n")
		b.WriteString(truncateRunes(c, contentMaxRunes))
		b.WriteString("\n\n")
		delete(args, "content")
	}
	b.WriteString(formatRemainingJSONMarkdown(args, restJSONMaxRunes))
	return b.String()
}

func formatRemainingJSONMarkdown(args map[string]any, restJSONMaxRunes int) string {
	if len(args) == 0 {
		return ""
	}
	rest, err := json.MarshalIndent(args, "", "  ")
	if err != nil {
		rest = []byte("{}")
	}
	rs := string(rest)
	if utf8.RuneCountInString(rs) > restJSONMaxRunes {
		rs = truncateRunes(rs, restJSONMaxRunes)
	}
	return "### Other arguments\n\n```json\n" + rs + "\n```\n"
}

func (a *WeixinApprover) handleSingle(req *proto.ApprovalRequest, resp *proto.ApprovalResult) {
	tape := strings.TrimSpace(req.Tape)
	log.Printf("weixin: approver single tape=%q tool=%q", tape, req.Tool)
	if !strings.HasPrefix(tape, "weixin:p2p:") {
		resp.Choice = choiceDenied
		resp.Comment = "approvals only supported for Weixin private chat (weixin:p2p:*)"
		return
	}
	peerID, ok := p2pPeerFromTape(tape)
	if !ok {
		resp.Choice = choiceDenied
		resp.Comment = "unknown tape routing for approval"
		return
	}
	argsStr := strings.TrimSpace(req.ArgsJSON)
	if argsStr == "" {
		argsStr = "{}"
	}
	reason := strings.TrimSpace(req.Decision.Reason)
	risk := strings.TrimSpace(req.Decision.Risk)
	var b strings.Builder
	b.WriteString("## DMR tool approval required\n\n")
	b.WriteString(fmt.Sprintf("- Tool: %s\n", req.Tool))
	if risk != "" {
		b.WriteString(fmt.Sprintf("- Risk: %s\n", risk))
	}
	if reason != "" {
		b.WriteString(fmt.Sprintf("- Reason: %s\n", reason))
	}
	b.WriteString("\n")
	b.WriteString(formatApprovalArgsMarkdown(req.Tool, argsStr, maxApprovalContentRunesSingle))
	b.WriteString("\n### Reply\n\n")
	b.WriteString("Reply with one letter:\n")
	b.WriteString("- y — approve once\n")
	b.WriteString("- s — approve session\n")
	b.WriteString("- a — approve always\n")
	b.WriteString("- n — deny\n")
	body := b.String()
	reply := a.waitApproval(peerID, body, 0)
	resp.Choice = reply.choice
	if resp.Choice == choiceDenied {
		resp.Comment = "denied or timeout"
	}
}

func (a *WeixinApprover) handleBatch(req *proto.BatchApprovalRequest, resp *proto.BatchApprovalResult) {
	if len(req.Requests) == 0 {
		resp.Choice = choiceDenied
		return
	}
	tape := strings.TrimSpace(req.Requests[0].Tape)
	for _, r := range req.Requests {
		if strings.TrimSpace(r.Tape) != tape {
			resp.Choice = choiceDenied
			return
		}
	}
	if !strings.HasPrefix(tape, "weixin:p2p:") {
		resp.Choice = choiceDenied
		return
	}
	peerID, ok := p2pPeerFromTape(tape)
	if !ok {
		resp.Choice = choiceDenied
		return
	}
	first := req.Requests[0]
	reason := strings.TrimSpace(first.Decision.Reason)
	risk := strings.TrimSpace(first.Decision.Risk)
	n := len(req.Requests)
	var b strings.Builder
	b.WriteString("## DMR batch approval\n\n")
	fmt.Fprintf(&b, "Approval required — %d command(s).\n\n", n)
	if reason != "" {
		b.WriteString(fmt.Sprintf("Reason: %s\n\n", reason))
	}
	if risk != "" {
		b.WriteString(fmt.Sprintf("Risk: %s\n\n", risk))
	}
	b.WriteString("### Commands\n\n")
	for i, r := range req.Requests {
		if i >= 8 {
			fmt.Fprintf(&b, "\n(Items after #8 omitted.)\n")
			break
		}
		argsStr := strings.TrimSpace(r.ArgsJSON)
		if argsStr == "" {
			argsStr = "{}"
		}
		b.WriteString(formatBatchCommandLine(i+1, r.Tool, argsStr, maxApprovalContentRunesBatch))
		b.WriteString("\n\n")
	}
	b.WriteString("### Reply\n\n")
	b.WriteString("- y / yes — approve all (once)\n")
	fmt.Fprintf(&b, "- 1–%d or 1,3,5 — approve listed (once)\n", n)
	b.WriteString("- s / session — session\n")
	b.WriteString("- a / always — always\n")
	b.WriteString("- n / no — deny all\n")
	reply := a.waitApproval(peerID, b.String(), n)
	resp.Choice = reply.choice
	if reply.indices != nil {
		resp.Approved = reply.indices
	} else {
		resp.Approved = nil
	}
}

func formatBatchCommandLine(index int, tool, argsJSON string, contentMaxRunes int) string {
	var b strings.Builder
	fmt.Fprintf(&b, "%d. ", index)
	raw := strings.TrimSpace(argsJSON)
	if raw == "" {
		raw = "{}"
	}
	var args map[string]any
	if err := json.Unmarshal([]byte(raw), &args); err != nil {
		fmt.Fprintf(&b, "%s — (invalid JSON)\n\n", tool)
		b.WriteString(truncateRunes(raw, 400))
		return b.String()
	}
	switch tool {
	case "shell":
		cmd, _ := args["cmd"].(string)
		fmt.Fprintf(&b, "%s\n", tool)
		b.WriteString(truncateRunes(cmd, contentMaxRunes))
	case "fsWrite", "fsEdit":
		path, _ := args["path"].(string)
		fmt.Fprintf(&b, "%s path: %s", tool, path)
		if c, ok := args["content"].(string); ok && c != "" {
			b.WriteString("\n")
			b.WriteString(truncateRunes(c, contentMaxRunes))
		}
	default:
		fmt.Fprintf(&b, "%s", tool)
		for k, v := range args {
			b.WriteString(fmt.Sprintf("\n- %s: %v", k, v))
		}
	}
	return b.String()
}

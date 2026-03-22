package main

import (
	"context"
	"crypto/subtle"
	"log"
	"strings"
)

func effectiveRestartTrigger(trigger string) string {
	if strings.TrimSpace(trigger) == "" {
		return ",dmr-restart"
	}
	return strings.TrimSpace(trigger)
}

func restartPayloadFromFirstLine(trigger, content string) (payload string, matchedPrefix bool) {
	line := strings.TrimSpace(strings.Split(content, "\n")[0])
	tr := effectiveRestartTrigger(trigger)
	if !strings.HasPrefix(line, tr) {
		return "", false
	}
	rest := strings.TrimSpace(strings.TrimPrefix(line, tr))
	return rest, true
}

func constantTimeStringEqual(a, b string) bool {
	if len(a) != len(b) {
		return false
	}
	if len(a) == 0 {
		return true
	}
	return subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1
}

func (p *WeixinPlugin) tryHandleDMRRestart(ctx context.Context, job *inboundJob, content string) bool {
	wantTok := strings.TrimSpace(p.cfg.DmrRestartToken)
	if wantTok == "" {
		return false
	}
	got, matched := restartPayloadFromFirstLine(p.cfg.DmrRestartTrigger, content)
	if !matched {
		return false
	}
	if got == "" || !constantTimeStringEqual(wantTok, got) {
		log.Printf("weixin: dmr restart rejected peerID=%q", job.PeerID)
		_ = p.replyAgentOutput(ctx, job, "dmr_restart：口令不匹配。首行应为："+effectiveRestartTrigger(p.cfg.DmrRestartTrigger)+" <token>")
		return true
	}
	hostErr, rpcErr := p.callRestartHost()
	if rpcErr != nil {
		_ = p.replyAgentOutput(ctx, job, "DMR 重启 RPC 失败: "+rpcErr.Error())
		return true
	}
	if hostErr != "" {
		_ = p.replyAgentOutput(ctx, job, "DMR 重启不可用: "+hostErr)
		return true
	}
	_ = p.replyAgentOutput(ctx, job, "已请求重启 DMR（等价于本机 `dmr serve service restart`）。")
	return true
}

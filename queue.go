package main

import (
	"context"
	"fmt"
	"log"
	"strings"
	"sync"

	"github.com/seanly/dmr/pkg/plugin/proto"
)

type inboundJob struct {
	QueueKey     string
	TapeName     string
	PeerID       string
	Content      string
	TriggerMsgID string
	ContextToken string
	// SessionID from the triggering inbound message (incl. loose JSON keys); used if peer map has no session.
	SessionID string
	// Attachments holds media files downloaded from direct (non-ref) message items.
	Attachments []InboundAttachment
	// RefAttachments holds media files downloaded from the referenced message.
	RefAttachments []InboundAttachment
	// RefTextContent is the text body of the referenced message (if it was text).
	RefTextContent string
}

type queueManager struct {
	plugin        *WeixinPlugin
	mu            sync.Mutex
	jobs          chan *inboundJob
	workerStarted bool
}

func newQueueManager(p *WeixinPlugin) *queueManager {
	return &queueManager{
		plugin: p,
		jobs:   make(chan *inboundJob, 64),
	}
}

func (qm *queueManager) enqueue(job *inboundJob) {
	if job == nil || job.TapeName == "" {
		return
	}
	qm.mu.Lock()
	if qm.jobs == nil {
		qm.mu.Unlock()
		return
	}
	if !qm.workerStarted {
		qm.workerStarted = true
		go qm.plugin.runWorker(qm.jobs)
	}
	ch := qm.jobs
	qm.mu.Unlock()
	ch <- job
}

func (qm *queueManager) shutdown() {
	qm.mu.Lock()
	defer qm.mu.Unlock()
	if qm.jobs != nil {
		close(qm.jobs)
		qm.jobs = nil
		qm.workerStarted = false
	}
}

func (p *WeixinPlugin) runWorker(jobs <-chan *inboundJob) {
	for job := range jobs {
		if job != nil {
			log.Printf("weixin: worker job tape=%q peer=%q msgID=%q", job.TapeName, job.PeerID, job.TriggerMsgID)
		}
		p.processJob(job)
	}
}

func (p *WeixinPlugin) processJob(job *inboundJob) {
	if job == nil {
		return
	}
	ctx := context.Background()
	p.setActiveJob(job)
	defer p.clearActiveJob()

	userTrim := strings.TrimSpace(job.Content)
	runPrompt := p.composeRunPrompt(job)
	if strings.HasPrefix(userTrim, ",") || strings.HasPrefix(userTrim, "，") {
		if strings.HasPrefix(userTrim, "，") {
			userTrim = "," + strings.TrimPrefix(userTrim, "，")
		}
		runPrompt = userTrim
	}
	resp, err := p.callRunAgent(job.TapeName, runPrompt, 0)
	if err != nil {
		log.Printf("weixin: RunAgent RPC error: %v", err)
		_ = p.replyAgentOutput(ctx, job, "DMR: RunAgent failed: "+err.Error())
		return
	}
	if resp == nil {
		return
	}
	if resp.Error != "" {
		_ = p.replyAgentOutput(ctx, job, "DMR error: "+resp.Error)
	} else {
		out := resp.Output
		if out == "" {
			out = weixinFallbackWhenNoText(job.TapeName, resp)
		}
		_ = p.replyAgentOutput(ctx, job, out)
	}
}

func weixinFallbackWhenNoText(tape string, resp *proto.RunAgentResponse) string {
	if resp == nil {
		return "(no output)"
	}
	if len(resp.ToolCalls) == 0 {
		if resp.Steps > 0 {
			return fmt.Sprintf(
				"助手未返回可见文字。本轮约 %d 步；请查 tape「%s」。长内容可先写入文件，再用 weixinSendText 发摘要或链接。",
				resp.Steps, strings.TrimSpace(tape),
			)
		}
		return "未产生助手回复（0 步）。"
	}
	var names []string
	for _, tc := range resp.ToolCalls {
		if n := strings.TrimSpace(tc.Name); n != "" {
			names = append(names, n)
		}
	}
	if len(names) == 0 {
		return "(no output)"
	}
	const maxShow = 12
	shown := names
	ellipsis := ""
	if len(names) > maxShow {
		shown = names[:maxShow]
		ellipsis = fmt.Sprintf(" …（共 %d 次）", len(names))
	} else {
		ellipsis = fmt.Sprintf("（共 %d 次）", len(names))
	}
	return fmt.Sprintf("本轮未输出文字，已执行：%s%s。", strings.Join(shown, ", "), ellipsis)
}

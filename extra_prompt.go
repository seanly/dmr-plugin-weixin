package main

import "strings"

const weixinInboundBuiltinSchedulingHint = `【微信·定时】由 dmr-plugin-cron 到点触发的 RunAgent 没有微信入站上下文，助手最终文本不会自动发到微信；若要让定时/提醒内容到达本单聊，须调用 weixinSendText 并指定 tape_name（与当前会话 tape 一致，例如 weixin:p2p:<user_id@im.wechat>）。一次性提醒可在 cronAdd 中使用 run_once=true。

[Cron/Weixin] Scheduled runs lack Weixin inbound context—assistant text is not auto-posted to IM. To deliver a message here, call weixinSendText with tape_name matching this chat. For one-shot jobs, use run_once=true on cronAdd.`

const weixinInboundBuiltinReportHint = `【微信·报告】长报告、多段落交付：可用 fsWrite 落盘（如 .md），再用 **weixinSendText** 发摘要、要点或文件路径说明（当前版本不提供 weixinSendFile）。

[Weixin reports] For long output: write to a file if needed, then summarize or reference via weixinSendText (file attachment tool not available in this build).`

func (p *WeixinPlugin) composeRunPrompt(job *inboundJob) string {
	configExtra := strings.TrimSpace(p.extraRunPrompt)
	var prefixParts []string
	prefixParts = append(prefixParts, strings.TrimSpace(weixinInboundBuiltinSchedulingHint))
	prefixParts = append(prefixParts, strings.TrimSpace(weixinInboundBuiltinReportHint))
	if configExtra != "" {
		prefixParts = append(prefixParts, configExtra)
	}
	prefix := strings.Join(prefixParts, "\n\n")

	userText := strings.TrimSpace(job.Content)
	hasRef := len(job.RefAttachments) > 0 || job.RefTextContent != ""

	if userText == "" && !hasRef {
		return prefix
	}

	var bodyParts []string

	// Build referenced content section if present.
	if hasRef {
		var refLines []string
		if job.RefTextContent != "" {
			refLines = append(refLines, job.RefTextContent)
		}
		if len(job.RefAttachments) > 0 {
			refLines = append(refLines, formatAttachments(job.RefAttachments))
		}
		bodyParts = append(bodyParts, "[引用消息]\n"+strings.Join(refLines, "\n"))
	}

	// User's own message.
	if userText != "" {
		bodyParts = append(bodyParts, "[用户消息]\n"+userText)
	}

	return prefix + "\n\n---\n\n" + strings.Join(bodyParts, "\n\n")
}

// formatAttachments formats attachment list for prompt injection.
func formatAttachments(atts []InboundAttachment) string {
	var lines []string
	for _, a := range atts {
		label := a.Type
		switch a.Type {
		case "image":
			label = "图片"
		case "voice":
			label = "语音"
		case "file":
			label = "文件"
		case "video":
			label = "视频"
		}
		if a.FileName != "" {
			lines = append(lines, "- "+label+": "+a.FilePath+" ("+a.FileName+")")
		} else {
			lines = append(lines, "- "+label+": "+a.FilePath)
		}
	}
	return strings.Join(lines, "\n")
}

package main

import "strings"

const weixinInboundBuiltinSchedulingHint = `【微信·定时】由 dmr-plugin-cron 到点触发的 RunAgent 没有微信入站上下文，助手最终文本不会自动发到微信；若要让定时/提醒内容到达本单聊，须调用 weixin.send_text 并指定 tape_name（与当前会话 tape 一致，例如 weixin:p2p:<user_id@im.wechat>）。一次性提醒可在 cron.add 中使用 run_once=true。

[Cron/Weixin] Scheduled runs lack Weixin inbound context—assistant text is not auto-posted to IM. To deliver a message here, call weixin.send_text with tape_name matching this chat. For one-shot jobs, use run_once=true on cron.add.`

const weixinInboundBuiltinReportHint = `【微信·报告】报告、分析、总结、评估、扫描结果等多段落交付：**先**用 fs.write 写成 UTF-8 文件（优先 .md），再 **只** 用 **weixin.send_file** 的 **path** 发送。**禁止**用 weixin.send_text 承载报告正文；send_text 仅用于短确认、链接等。

[Weixin reports] For report-style output: write full body to a file (prefer .md), deliver via weixin.send_file path only; do not put report body in weixin.send_text.`

func (p *WeixinPlugin) composeRunPrompt(userContent string) string {
	user := userContent
	configExtra := strings.TrimSpace(p.extraRunPrompt)
	var prefixParts []string
	prefixParts = append(prefixParts, strings.TrimSpace(weixinInboundBuiltinSchedulingHint))
	prefixParts = append(prefixParts, strings.TrimSpace(weixinInboundBuiltinReportHint))
	if configExtra != "" {
		prefixParts = append(prefixParts, configExtra)
	}
	prefix := strings.Join(prefixParts, "\n\n")
	if strings.TrimSpace(user) == "" {
		return prefix
	}
	return prefix + "\n\n---\n\nUser message:\n" + user
}

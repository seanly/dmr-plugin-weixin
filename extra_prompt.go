package main

import "strings"

const weixinInboundBuiltinSchedulingHint = `【微信·定时】由 dmr-plugin-cron 到点触发的 RunAgent 没有微信入站上下文，助手最终文本不会自动发到微信；若要让定时/提醒内容到达本单聊，须调用 weixinSendText 并指定 tape_name（与当前会话 tape 一致，例如 weixin:p2p:<user_id@im.wechat>）。一次性提醒可在 cronAdd 中使用 run_once=true。

[Cron/Weixin] Scheduled runs lack Weixin inbound context—assistant text is not auto-posted to IM. To deliver a message here, call weixinSendText with tape_name matching this chat. For one-shot jobs, use run_once=true on cronAdd.`

const weixinInboundBuiltinReportHint = `【微信·报告】长报告、多段落交付：可用 fsWrite 落盘（如 .md），再用 **weixinSendText** 发摘要、要点或文件路径说明（当前版本不提供 weixinSendFile）。

[Weixin reports] For long output: write to a file if needed, then summarize or reference via weixinSendText (file attachment tool not available in this build).`

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

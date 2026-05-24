# OPA Policy for Weixin plugin
# Rego V1 format

package dmr

# All Weixin send tools allowed without approval (human-in-the-loop off for these tools).

decision := {"action": "allow", "reason": "weixin: send allowed by policy", "risk": "low"} if {
	input.tool in [
		"weixinSendText",
		"weixinSendFile",
	]
}

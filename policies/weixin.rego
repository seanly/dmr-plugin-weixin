# OPA Policy for Weixin plugin
# Rego V1 format

package dmr

# All weixin operations are allowed by default

decision := {"action": "allow", "reason": "weixin operation", "risk": "low"} if {
	input.tool in [
		"weixinSendText",
		"weixinSendMedia"
	]
}

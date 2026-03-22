// dmr-plugin-weixin is an external DMR plugin: Weixin gateway (ilink/bot) long-poll -> agent -> Weixin reply.
package main

import (
	goplugin "github.com/hashicorp/go-plugin"
	"github.com/seanly/dmr/pkg/plugin/proto"
)

func main() {
	impl := NewWeixinPlugin()

	goplugin.Serve(&goplugin.ServeConfig{
		HandshakeConfig: proto.Handshake,
		Plugins: map[string]goplugin.Plugin{
			"dmr-plugin": &proto.DMRPlugin{Impl: impl},
		},
	})
}

package main

import (
	"encoding/json"
	"fmt"
	"net/rpc"

	"github.com/seanly/dmr/pkg/plugin/proto"
)

func (p *WeixinPlugin) hostRPC() (*rpc.Client, error) {
	p.hostMu.Lock()
	c := p.hostClient
	p.hostMu.Unlock()
	if c == nil {
		return nil, fmt.Errorf("host RPC client not ready")
	}
	return c, nil
}

func (p *WeixinPlugin) callRunAgent(tapeName, prompt string, historyAfter int32) (*proto.RunAgentResponse, error) {
	return p.callRunAgentWithContext(tapeName, prompt, historyAfter, nil)
}

func (p *WeixinPlugin) callRunAgentWithContext(tapeName, prompt string, historyAfter int32, ctx map[string]any) (*proto.RunAgentResponse, error) {
	c, err := p.hostRPC()
	if err != nil {
		return nil, err
	}
	req := &proto.RunAgentRequest{
		TapeName:            tapeName,
		Prompt:              prompt,
		HistoryAfterEntryID: historyAfter,
	}
	// Encode context if provided
	if ctx != nil && len(ctx) > 0 {
		ctxJSON, _ := json.Marshal(ctx)
		req.ContextJSON = string(ctxJSON)
	}
	var resp proto.RunAgentResponse
	if err := c.Call("Plugin.RunAgent", req, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

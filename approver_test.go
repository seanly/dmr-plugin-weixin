package main

import (
	"testing"
)

func TestParseBatchApprovalChoice(t *testing.T) {
	r, ok := parseBatchApprovalChoice("1,3", 3)
	if !ok || r.choice != choiceApprovedOnce || len(r.indices) != 2 || r.indices[0] != 0 || r.indices[1] != 2 {
		t.Fatalf("got %+v", r)
	}
	r2, _ := parseBatchApprovalChoice("y", 2)
	if r2.choice != choiceApprovedOnce || r2.indices != nil {
		t.Fatalf("got %+v", r2)
	}
}

func TestP2pPeerFromTape(t *testing.T) {
	id, ok := p2pPeerFromTape("weixin:p2p:abc@im.wechat")
	if !ok || id != "abc@im.wechat" {
		t.Fatalf("got %q %v", id, ok)
	}
}

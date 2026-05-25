package weixin

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

func TestParseWeixinP2PTape(t *testing.T) {
	pid, lbl, ok := parseWeixinP2PTape("weixin:p2p:abc@im.wechat")
	if !ok || lbl != "" || pid != "abc@im.wechat" {
		t.Fatalf("legacy got peer=%q label=%q ok=%v", pid, lbl, ok)
	}
	pid2, lbl2, ok2 := parseWeixinP2PTape("weixin:work:p2p:xyz@im.wechat")
	if !ok2 || lbl2 != "work" || pid2 != "xyz@im.wechat" {
		t.Fatalf("scoped got peer=%q label=%q ok=%v", pid2, lbl2, ok2)
	}
}

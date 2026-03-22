package main

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestSessionIDJSON_MarshalAlwaysQuotedString(t *testing.T) {
	b, err := json.Marshal(&weixinMessage{
		FromUserID:  "",
		ToUserID:    "u@x",
		MessageType: msgTypeBot,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), `"session_id"`) {
		t.Fatalf("expected session_id key in json: %s", b)
	}
}

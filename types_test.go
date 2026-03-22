package main

import (
	"encoding/json"
	"testing"
)

func TestWeixinMessage_InboundSessionJSONAliases(t *testing.T) {
	for _, tc := range []struct {
		raw    string
		wantID string
	}{
		{`{"session_id":"snake"}`, "snake"},
		{`{"sessionId":"camel"}`, "camel"},
		{`{"SessionId":"pascal"}`, "pascal"},
		{`{"session_id":"","sessionId":"c2"}`, "c2"},
		{`{"mlink_session_id":"mlink-1"}`, "mlink-1"},
		{`{"ilink_session_id":"il-2"}`, "il-2"},
		{`{"conversation_id":"conv-3"}`, "conv-3"},
		{`{"session_id":1844007122}`, "1844007122"},
	} {
		var m weixinMessage
		if err := json.Unmarshal([]byte(tc.raw), &m); err != nil {
			t.Fatalf("unmarshal %q: %v", tc.raw, err)
		}
		if got := m.inboundSessionID(); got != tc.wantID {
			t.Fatalf("inboundSessionID %q: got %q want %q", tc.raw, got, tc.wantID)
		}
	}
}

func TestWeixinMessage_InboundContextTokenJSONAliases(t *testing.T) {
	for _, tc := range []struct {
		raw     string
		wantTok string
	}{
		{`{"context_token":"snake"}`, "snake"},
		{`{"contextToken":"camel"}`, "camel"},
		{`{"ContextToken":"pascal"}`, "pascal"},
	} {
		var m weixinMessage
		if err := json.Unmarshal([]byte(tc.raw), &m); err != nil {
			t.Fatalf("unmarshal %q: %v", tc.raw, err)
		}
		if got := m.inboundContextToken(); got != tc.wantTok {
			t.Fatalf("inboundContextToken %q: got %q want %q", tc.raw, got, tc.wantTok)
		}
	}
}

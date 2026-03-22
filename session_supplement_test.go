package main

import (
	"encoding/json"
	"testing"
)

func TestSupplementWeixinSession_mlinkSessionID(t *testing.T) {
	raw := []byte(`{"from_user_id":"u@im.wechat","mlink_session_id":"sess-99"}`)
	var m weixinMessage
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatal(err)
	}
	supplementWeixinSession(&m, raw)
	if got := m.inboundSessionID(); got != "sess-99" {
		t.Fatalf("got %q want sess-99", got)
	}
}

func TestSupplementWeixinSession_nestedNumeric(t *testing.T) {
	raw := []byte(`{"from_user_id":"u@x","base":{"session_id":42}}`)
	var m weixinMessage
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatal(err)
	}
	supplementWeixinSession(&m, raw)
	if got := m.inboundSessionID(); got != "42" {
		t.Fatalf("got %q want 42", got)
	}
}

func TestSupplementWeixinSession_skipsWhenAlreadySet(t *testing.T) {
	raw := []byte(`{"session_id":"first","mlink_session_id":"second"}`)
	var m weixinMessage
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatal(err)
	}
	supplementWeixinSession(&m, raw)
	if got := m.inboundSessionID(); got != "first" {
		t.Fatalf("got %q want first", got)
	}
}

func TestSupplementWeixinSession_sessionInsideArray(t *testing.T) {
	raw := []byte(`{"from_user_id":"u@x","nested":[{"session_id":"arr-sess"}]}`)
	var m weixinMessage
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatal(err)
	}
	supplementWeixinSession(&m, raw)
	if got := m.inboundSessionID(); got != "arr-sess" {
		t.Fatalf("got %q want arr-sess", got)
	}
}

func TestSessionFromGetConfigRaw(t *testing.T) {
	raw := []byte(`{"ret":0,"data":{"mlink_session_id":"gc-7"}}`)
	if got := sessionFromGetConfigRaw(raw); got != "gc-7" {
		t.Fatalf("got %q want gc-7", got)
	}
}

func TestApplyLooseSessionFromGetUpdatesRaw(t *testing.T) {
	raw := []byte(`{"ret":0,"msgs":[{"from_user_id":"p@x","mlink_session":"s1"}]}`)
	var out getUpdatesResp
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatal(err)
	}
	applyLooseSessionFromGetUpdatesRaw(raw, out.Msgs)
	if len(out.Msgs) != 1 {
		t.Fatalf("msgs len %d", len(out.Msgs))
	}
	if got := out.Msgs[0].inboundSessionID(); got != "s1" {
		t.Fatalf("got %q want s1", got)
	}
}

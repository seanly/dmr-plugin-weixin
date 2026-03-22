package main

import (
	"bytes"
	"encoding/json"
	"log"
	"os"
	"sort"
	"strconv"
	"strings"
)

// applyLooseSessionFromGetUpdatesRaw re-parses msgs from the raw getupdates body so we can
// pick up session identifiers under non-standard keys or nesting (some gateways omit session_id).
func applyLooseSessionFromGetUpdatesRaw(raw []byte, msgs []weixinMessage) {
	if len(msgs) == 0 || len(raw) == 0 {
		return
	}
	var env struct {
		Msgs []json.RawMessage `json:"msgs"`
	}
	if err := json.Unmarshal(raw, &env); err != nil {
		return
	}
	for i := range msgs {
		if i >= len(env.Msgs) {
			break
		}
		supplementWeixinSession(&msgs[i], env.Msgs[i])
		if os.Getenv("DMR_WEIXIN_DEBUG_SEND") == "1" && msgs[i].inboundSessionID() == "" {
			log.Printf("weixin debug: getupdates msg[%d] no session after supplement; top-level json keys: %s; session_id_json=%s",
				i, jsonTopLevelKeys(env.Msgs[i]), describeSessionIDRawJSON(env.Msgs[i]))
		}
	}
}

func supplementWeixinSession(m *weixinMessage, msgRaw []byte) {
	if m == nil || len(msgRaw) == 0 {
		return
	}
	if m.inboundSessionID() != "" {
		return
	}
	var obj map[string]json.RawMessage
	if err := json.Unmarshal(msgRaw, &obj); err != nil {
		return
	}
	if s := findSessionInRawJSONMap(obj); s != "" {
		m.SessionID = sessionIDJSON(s)
	}
}

// describeSessionIDRawJSON classifies the raw session_id value for debug (no secret token contents).
func describeSessionIDRawJSON(msgRaw []byte) string {
	var obj map[string]json.RawMessage
	if err := json.Unmarshal(msgRaw, &obj); err != nil {
		return "parse_err"
	}
	v, ok := obj["session_id"]
	if !ok {
		return "absent"
	}
	v = bytes.TrimSpace(v)
	if len(v) == 0 || bytes.Equal(v, []byte("null")) {
		return "null"
	}
	switch v[0] {
	case '"':
		var s string
		if json.Unmarshal(v, &s) != nil {
			return "string(parse_err)"
		}
		if strings.TrimSpace(s) == "" {
			return `string("")`
		}
		return "string(non-empty)"
	case '0', '1', '2', '3', '4', '5', '6', '7', '8', '9', '-':
		return "number"
	case '{':
		return "object"
	case '[':
		return "array"
	default:
		return "other"
	}
}

func findSessionInRawJSONMap(obj map[string]json.RawMessage) string {
	for k, v := range obj {
		if sessionLikeJSONKey(k) {
			if s := jsonRawToSessionString(v); s != "" {
				return s
			}
		}
	}
	for _, v := range obj {
		v = bytes.TrimSpace(v)
		if len(v) == 0 {
			continue
		}
		switch v[0] {
		case '{':
			var inner map[string]json.RawMessage
			if json.Unmarshal(v, &inner) != nil {
				continue
			}
			if s := findSessionInRawJSONMap(inner); s != "" {
				return s
			}
		case '[':
			var arr []json.RawMessage
			if json.Unmarshal(v, &arr) != nil {
				continue
			}
			for _, elem := range arr {
				elem = bytes.TrimSpace(elem)
				if len(elem) == 0 || elem[0] != '{' {
					continue
				}
				var inner map[string]json.RawMessage
				if json.Unmarshal(elem, &inner) != nil {
					continue
				}
				if s := findSessionInRawJSONMap(inner); s != "" {
					return s
				}
			}
		}
	}
	return ""
}

func jsonTopLevelKeys(msgRaw []byte) string {
	var obj map[string]json.RawMessage
	if err := json.Unmarshal(msgRaw, &obj); err != nil {
		return "(unmarshal err)"
	}
	keys := make([]string, 0, len(obj))
	for k := range obj {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	if len(keys) > 24 {
		keys = keys[:24]
	}
	return strings.Join(keys, ",")
}

func sessionLikeJSONKey(k string) bool {
	lk := strings.ToLower(strings.ReplaceAll(k, "-", "_"))
	if strings.Contains(lk, "session_id") {
		return true
	}
	if strings.HasSuffix(lk, "sessionid") {
		return true
	}
	switch lk {
	case "session_key", "sessionkey", "mlink_session", "chatsessionid", "ilink_session",
		"conversation_id", "conv_id", "chat_session_id":
		return true
	default:
		return false
	}
}

// sessionFromGetConfigRaw extracts session-like fields from getconfig JSON when they are missing
// from the top-level struct (e.g. nested under "data").
func sessionFromGetConfigRaw(raw []byte) string {
	var obj map[string]json.RawMessage
	if err := json.Unmarshal(raw, &obj); err != nil {
		return ""
	}
	if s := findSessionInRawJSONMap(obj); s != "" {
		return s
	}
	v, ok := obj["data"]
	if !ok {
		return ""
	}
	v = bytes.TrimSpace(v)
	if len(v) == 0 || v[0] != '{' {
		return ""
	}
	var inner map[string]json.RawMessage
	if err := json.Unmarshal(v, &inner); err != nil {
		return ""
	}
	return findSessionInRawJSONMap(inner)
}

func jsonRawToSessionString(raw json.RawMessage) string {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 || bytes.Equal(raw, []byte("null")) {
		return ""
	}
	if raw[0] == '"' {
		var s string
		if json.Unmarshal(raw, &s) != nil {
			return ""
		}
		return strings.TrimSpace(s)
	}
	var n int64
	if json.Unmarshal(raw, &n) == nil {
		return strconv.FormatInt(n, 10)
	}
	var f float64
	if json.Unmarshal(raw, &f) == nil {
		return strconv.FormatInt(int64(f), 10)
	}
	return ""
}

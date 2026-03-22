package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

// sessionIDJSON unmarshals ilink session_id as JSON string or number. Some gateways send numeric ids;
// encoding/json cannot put a number into a Go string field without this.
type sessionIDJSON string

func (s *sessionIDJSON) UnmarshalJSON(b []byte) error {
	b = bytes.TrimSpace(b)
	if len(b) == 0 || bytes.Equal(b, []byte("null")) {
		*s = ""
		return nil
	}
	if b[0] == '"' {
		var str string
		if err := json.Unmarshal(b, &str); err != nil {
			return err
		}
		*s = sessionIDJSON(strings.TrimSpace(str))
		return nil
	}
	var n int64
	if err := json.Unmarshal(b, &n); err == nil {
		*s = sessionIDJSON(strconv.FormatInt(n, 10))
		return nil
	}
	var f float64
	if err := json.Unmarshal(b, &f); err == nil {
		*s = sessionIDJSON(strconv.FormatInt(int64(f), 10))
		return nil
	}
	return fmt.Errorf("session_id: unsupported JSON %s", string(b))
}

func (s sessionIDJSON) MarshalJSON() ([]byte, error) {
	return json.Marshal(strings.TrimSpace(string(s)))
}

func (s sessionIDJSON) String() string {
	return strings.TrimSpace(string(s))
}

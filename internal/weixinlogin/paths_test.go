package weixinlogin

import (
	"strings"
	"testing"
)

func TestSanitizeLoginID(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		in    string
		want  string
		valid bool
	}{
		{"", "", true},
		{"  ", "", true},
		{"work", "work", true},
		{"a", "a", true},
		{"bot_2", "bot_2", true},
		{"a.b-c", "a.b-c", true},
		{"../x", "", false},
		{"/bad", "", false},
		{"café", "", false},
	} {
		tc := tc
		t.Run(strings.ReplaceAll(tc.in, "/", "_"), func(t *testing.T) {
			t.Parallel()
			got, err := SanitizeLoginID(tc.in)
			if tc.valid {
				if err != nil || got != tc.want {
					t.Fatalf("SanitizeLoginID(%q) = %q, %v; want %q, nil", tc.in, got, err, tc.want)
				}
				return
			}
			if err == nil {
				t.Fatalf("SanitizeLoginID(%q) = %q, nil; want error", tc.in, got)
			}
		})
	}
}

func TestDefaultCredentialsPath(t *testing.T) {
	t.Parallel()
	p := DefaultCredentialsPath("")
	if !strings.Contains(p, "credentials.json") {
		t.Fatalf("unexpected path %q", p)
	}
	p2 := DefaultCredentialsPath("acct1")
	if !strings.Contains(p2, "acct1") || !strings.HasSuffix(p2, "credentials.json") {
		t.Fatalf("unexpected path %q", p2)
	}
}

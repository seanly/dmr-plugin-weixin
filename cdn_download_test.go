package main

import (
	"bytes"
	"encoding/base64"
	"testing"
)

func TestDecryptAesEcb_RoundTrip(t *testing.T) {
	key := []byte("0123456789abcdef") // 16 bytes
	plaintext := []byte("Hello, World! This is a test.")

	ciphertext, err := encryptAesEcb(plaintext, key)
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}

	got, err := decryptAesEcb(ciphertext, key)
	if err != nil {
		t.Fatalf("decrypt: %v", err)
	}

	if !bytes.Equal(got, plaintext) {
		t.Errorf("round-trip mismatch: got %q, want %q", got, plaintext)
	}
}

func TestDecryptAesEcb_BlockAligned(t *testing.T) {
	key := []byte("abcdefghijklmnop")
	// 16 bytes exactly — PKCS7 adds a full block of padding
	plaintext := []byte("exactly16bytes!!")

	ciphertext, err := encryptAesEcb(plaintext, key)
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}

	got, err := decryptAesEcb(ciphertext, key)
	if err != nil {
		t.Fatalf("decrypt: %v", err)
	}

	if !bytes.Equal(got, plaintext) {
		t.Errorf("got %q, want %q", got, plaintext)
	}
}

func TestPkcs7Unpad_Invalid(t *testing.T) {
	// Empty
	if _, err := pkcs7Unpad(nil); err == nil {
		t.Error("expected error for nil")
	}
	// Padding byte 0
	if _, err := pkcs7Unpad([]byte{0}); err == nil {
		t.Error("expected error for padding 0")
	}
	// Padding byte > block size
	if _, err := pkcs7Unpad([]byte{0x11}); err == nil {
		t.Error("expected error for padding > 16")
	}
}

func TestDecodeAesKey_Hex(t *testing.T) {
	// 32-char hex → 16 bytes
	key, err := decodeAesKey("30313233343536373839616263646566")
	if err != nil {
		t.Fatal(err)
	}
	if string(key) != "0123456789abcdef" {
		t.Errorf("got %q", key)
	}
}

func TestDecodeAesKey_Base64OfHex(t *testing.T) {
	// base64( hex_string ) — this is what inbound messages carry (44 chars)
	hexStr := "30313233343536373839616263646566" // hex of "0123456789abcdef"
	b64 := base64.StdEncoding.EncodeToString([]byte(hexStr))
	key, err := decodeAesKey(b64)
	if err != nil {
		t.Fatal(err)
	}
	if string(key) != "0123456789abcdef" {
		t.Errorf("got %q", key)
	}
}

func TestDecodeAesKey_Invalid(t *testing.T) {
	if _, err := decodeAesKey("tooshort"); err == nil {
		t.Error("expected error for short key")
	}
}

func TestBuildCdnDownloadUrl(t *testing.T) {
	u, err := buildCdnDownloadUrl("https://cdn.example.com/c2c", "test_param")
	if err != nil {
		t.Fatal(err)
	}
	if u == "" {
		t.Error("empty url")
	}
	if !contains(u, "encrypted_query_param=test_param") {
		t.Errorf("missing param in %s", u)
	}
	if !contains(u, "/download") {
		t.Errorf("missing /download in %s", u)
	}
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(s) > 0 && containsStr(s, sub))
}

func containsStr(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

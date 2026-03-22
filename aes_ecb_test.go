package main

import (
	"bytes"
	"crypto/aes"
	"testing"
)

func TestAesEcbPaddedSize(t *testing.T) {
	cases := []struct {
		in, want int
	}{
		{0, 16},
		{1, 16},
		{15, 16},
		{16, 32},
		{17, 32},
	}
	for _, c := range cases {
		if g := aesEcbPaddedSize(c.in); g != c.want {
			t.Fatalf("aesEcbPaddedSize(%d)=%d want %d", c.in, g, c.want)
		}
	}
}

func TestEncryptAESECBPKCS7RoundTrip(t *testing.T) {
	key := bytes.Repeat([]byte{0xab}, 16)
	plaintext := []byte("hello weixin cdn")
	ct, err := encryptAESECBPKCS7(plaintext, key)
	if err != nil {
		t.Fatal(err)
	}
	if len(ct) != aesEcbPaddedSize(len(plaintext)) {
		t.Fatalf("ciphertext len %d want %d", len(ct), aesEcbPaddedSize(len(plaintext)))
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		t.Fatal(err)
	}
	pt := make([]byte, len(ct))
	for i := 0; i < len(ct); i += aes.BlockSize {
		block.Decrypt(pt[i:i+aes.BlockSize], ct[i:i+aes.BlockSize])
	}
	pad := int(pt[len(pt)-1])
	if pad < 1 || pad > 16 {
		t.Fatalf("bad pad %d", pad)
	}
	got := pt[:len(pt)-pad]
	if !bytes.Equal(got, plaintext) {
		t.Fatalf("decrypt got %q want %q", got, plaintext)
	}
}

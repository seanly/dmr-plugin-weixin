package main

import (
	"crypto/aes"
	"errors"
)

// aesEcbPaddedSize matches openclaw-weixin aesEcbPaddedSize (PKCS7 ciphertext length).
func aesEcbPaddedSize(plaintextSize int) int {
	if plaintextSize < 0 {
		return 0
	}
	return ((plaintextSize + 16) / 16) * 16
}

func encryptAESECBPKCS7(plaintext, key []byte) ([]byte, error) {
	if len(key) != 16 {
		return nil, errors.New("aes key must be 16 bytes")
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	padLen := 16 - len(plaintext)%16
	if padLen == 0 {
		padLen = 16
	}
	padded := make([]byte, len(plaintext)+padLen)
	copy(padded, plaintext)
	for i := len(plaintext); i < len(padded); i++ {
		padded[i] = byte(padLen)
	}
	out := make([]byte, len(padded))
	for i := 0; i < len(padded); i += aes.BlockSize {
		block.Encrypt(out[i:i+aes.BlockSize], padded[i:i+aes.BlockSize])
	}
	return out, nil
}

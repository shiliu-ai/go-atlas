package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"io"
)

// AES provides AES-GCM encryption and decryption.
// Key must be 16, 24, or 32 bytes (AES-128, AES-192, AES-256).
type AES struct {
	gcm cipher.AEAD
}

// NewAES creates an AES-GCM cipher with the given key.
func NewAES(key []byte) (*AES, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("crypto: new aes cipher: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("crypto: new gcm: %w", err)
	}
	return &AES{gcm: gcm}, nil
}

// Encrypt encrypts plaintext and returns base64-encoded ciphertext.
func (a *AES) Encrypt(plaintext []byte) (string, error) {
	nonce := make([]byte, a.gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", fmt.Errorf("crypto: generate nonce: %w", err)
	}
	ciphertext := a.gcm.Seal(nonce, nonce, plaintext, nil)
	return base64.StdEncoding.EncodeToString(ciphertext), nil
}

// Decrypt decodes a base64-encoded ciphertext and returns the plaintext.
func (a *AES) Decrypt(encoded string) ([]byte, error) {
	data, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return nil, fmt.Errorf("crypto: base64 decode: %w", err)
	}

	nonceSize := a.gcm.NonceSize()
	if len(data) < nonceSize {
		return nil, fmt.Errorf("crypto: ciphertext too short")
	}

	nonce, ciphertext := data[:nonceSize], data[nonceSize:]
	plaintext, err := a.gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return nil, fmt.Errorf("crypto: decrypt: %w", err)
	}
	return plaintext, nil
}

// EncryptString encrypts a string and returns base64-encoded ciphertext.
func (a *AES) EncryptString(plaintext string) (string, error) {
	return a.Encrypt([]byte(plaintext))
}

// DecryptString decodes and decrypts to a string.
func (a *AES) DecryptString(encoded string) (string, error) {
	b, err := a.Decrypt(encoded)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

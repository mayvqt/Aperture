package security

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"io"
	"strings"
)

const encryptedPrefix = "enc:v1:"

type Encryptor struct {
	aead cipher.AEAD
}

func NewEncryptor(secret string) (*Encryptor, error) {
	if len(secret) < 32 {
		return nil, errors.New("encryption key must be at least 32 characters")
	}
	key := sha256.Sum256([]byte(secret))
	block, err := aes.NewCipher(key[:])
	if err != nil {
		return nil, err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	return &Encryptor{aead: aead}, nil
}

func (e *Encryptor) EncryptString(value string) (string, error) {
	if value == "" {
		return "", nil
	}
	nonce := make([]byte, e.aead.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}
	sealed := e.aead.Seal(nonce, nonce, []byte(value), nil)
	return encryptedPrefix + base64.RawURLEncoding.EncodeToString(sealed), nil
}

func (e *Encryptor) DecryptString(value string) (string, error) {
	if value == "" {
		return "", nil
	}
	if !strings.HasPrefix(value, encryptedPrefix) {
		return value, nil
	}
	raw, err := base64.RawURLEncoding.DecodeString(strings.TrimPrefix(value, encryptedPrefix))
	if err != nil {
		return "", err
	}
	if len(raw) < e.aead.NonceSize() {
		return "", errors.New("encrypted value is too short")
	}
	nonce := raw[:e.aead.NonceSize()]
	ciphertext := raw[e.aead.NonceSize():]
	plain, err := e.aead.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return "", err
	}
	return string(plain), nil
}

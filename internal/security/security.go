package security

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"strings"
)

func RandomToken(bytes int) (string, error) {
	if bytes < 16 {
		return "", errors.New("token size must be at least 16 bytes")
	}
	buf := make([]byte, bytes)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}

func HashToken(secret, token string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(token))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

func ConstantEqual(a, b string) bool {
	return hmac.Equal([]byte(a), []byte(b))
}

func Prefix(token string) string {
	if len(token) <= 8 {
		return token
	}
	return token[:8]
}

func Redact(value string) string {
	if value == "" {
		return ""
	}
	return strings.Repeat("*", 8)
}

package security

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"regexp"
	"strings"
)

var (
	secretAssignmentPattern = regexp.MustCompile(`(?i)["']?\b(api[_ -]?key|access[_ -]?token|authorization|token|password|passwd|pw|secret)\b["']?\s*[:=]\s*(?:"[^"]*"|'[^']*'|[^\s,;&}]+)`)
	bearerTokenPattern      = regexp.MustCompile(`(?i)\bbearer\s+[^\s,;&]+`)
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

// RedactText removes common credential forms and any exact secret values from
// diagnostic text before it is logged, persisted, or displayed.
func RedactText(value string, secrets ...string) string {
	value = bearerTokenPattern.ReplaceAllString(value, "Bearer [redacted]")
	value = secretAssignmentPattern.ReplaceAllStringFunc(value, func(match string) string {
		separator := strings.IndexAny(match, ":=")
		if separator < 0 {
			return "[redacted]"
		}
		return strings.TrimSpace(match[:separator]) + "=[redacted]"
	})
	for _, secret := range secrets {
		if secret != "" {
			value = strings.ReplaceAll(value, secret, "[redacted]")
		}
	}
	return value
}

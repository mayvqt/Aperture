package security

import (
	"strings"
	"testing"
)

func TestHashTokenUsesSecret(t *testing.T) {
	token := "invite-token"
	first := HashToken("secret-one", token)
	second := HashToken("secret-two", token)
	if first == second {
		t.Fatal("hash should change when the secret changes")
	}
	if !ConstantEqual(first, HashToken("secret-one", token)) {
		t.Fatal("same secret and token should compare equal")
	}
}

func TestRedactTextCoversStructuredAndKnownSecrets(t *testing.T) {
	known := "bare-known-api-key"
	input := `api_key=one "access_token":"two" Authorization: MediaBrowser Token="three" Bearer four password='five' ` + known
	got := RedactText(input, known)
	for _, secret := range []string{"one", "two", "three", "four", "five", known} {
		if strings.Contains(got, secret) {
			t.Fatalf("RedactText leaked %q in %q", secret, got)
		}
	}
}

func TestRandomToken(t *testing.T) {
	token, err := RandomToken(32)
	if err != nil {
		t.Fatal(err)
	}
	if len(token) < 32 {
		t.Fatalf("token is unexpectedly short: %d", len(token))
	}
	other, err := RandomToken(32)
	if err != nil {
		t.Fatal(err)
	}
	if token == other {
		t.Fatal("two random tokens should not match")
	}
}

func TestRandomTokenRejectsSmallSizes(t *testing.T) {
	if _, err := RandomToken(8); err == nil {
		t.Fatal("expected small token size to fail")
	}
}

func TestPrefixAndRedact(t *testing.T) {
	if got := Prefix("1234567890"); got != "12345678" {
		t.Fatalf("Prefix() = %q, want first 8 characters", got)
	}
	if got := Prefix("short"); got != "short" {
		t.Fatalf("Prefix(short) = %q, want unchanged", got)
	}
	if got := Redact("secret"); got != "********" {
		t.Fatalf("Redact() = %q, want fixed mask", got)
	}
	if got := Redact(""); got != "" {
		t.Fatalf("Redact(empty) = %q, want empty", got)
	}
}

func TestEncryptorRoundTrip(t *testing.T) {
	encryptor, err := NewEncryptor("test-encryption-key-32-characters")
	if err != nil {
		t.Fatal(err)
	}
	encrypted, err := encryptor.EncryptString("secret")
	if err != nil {
		t.Fatal(err)
	}
	if encrypted == "secret" {
		t.Fatal("secret was not encrypted")
	}
	decrypted, err := encryptor.DecryptString(encrypted)
	if err != nil {
		t.Fatal(err)
	}
	if decrypted != "secret" {
		t.Fatalf("decrypted value = %q, want secret", decrypted)
	}
}

func TestEncryptorRejectsShortKey(t *testing.T) {
	if _, err := NewEncryptor("short"); err == nil {
		t.Fatal("expected short encryption key to fail")
	}
}

func TestDecryptStringCompatibilityAndTamperFailure(t *testing.T) {
	encryptor, err := NewEncryptor("test-encryption-key-32-characters")
	if err != nil {
		t.Fatal(err)
	}
	if got, err := encryptor.DecryptString("plain"); err != nil || got != "plain" {
		t.Fatalf("DecryptString(plain) = %q, %v; want plaintext passthrough", got, err)
	}
	encrypted, err := encryptor.EncryptString("secret")
	if err != nil {
		t.Fatal(err)
	}
	replacement := "A"
	tamperAt := len(encryptedPrefix)
	if encrypted[tamperAt:tamperAt+1] == replacement {
		replacement = "B"
	}
	tampered := encrypted[:tamperAt] + replacement + encrypted[tamperAt+1:]
	if _, err := encryptor.DecryptString(tampered); err == nil {
		t.Fatal("expected tampered ciphertext to fail")
	}
}

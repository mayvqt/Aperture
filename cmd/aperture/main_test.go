package main

import (
	"bytes"
	"errors"
	"os"
	"strings"
	"testing"
)

func TestDropCommand(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want int
	}{
		{name: "explicit serve", args: []string{"aperture", "serve", "--http-addr", ":8099"}, want: 2},
		{name: "implicit serve flags", args: []string{"aperture", "--http-addr", ":8099"}, want: 1},
		{name: "no args", args: []string{"aperture"}, want: 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := dropCommand(tt.args); got != tt.want {
				t.Fatalf("dropCommand() = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestRunRejectsUnknownCommand(t *testing.T) {
	oldArgs := os.Args
	defer func() { os.Args = oldArgs }()
	os.Args = []string{"aperture", "nope"}
	if err := run(); err == nil {
		t.Fatal("expected unknown command to fail")
	}
}

func TestLoggerRedactsStructuredAndKnownSecrets(t *testing.T) {
	var output bytes.Buffer
	logger := newLogger(&output, "info", "bare-environment-api-key")
	logger.Error("request failed",
		"error", errors.New(`Authorization: Bearer header-token password="form-password" bare-environment-api-key`),
		"detail", `{"access_token":"json-token"}`,
	)
	for _, secret := range []string{"header-token", "form-password", "bare-environment-api-key", "json-token"} {
		if strings.Contains(output.String(), secret) {
			t.Fatalf("log exposed %q in %q", secret, output.String())
		}
	}
	if !strings.Contains(output.String(), "[redacted]") {
		t.Fatalf("log did not contain redaction marker: %q", output.String())
	}
}

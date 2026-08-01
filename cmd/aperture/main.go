package main

import (
	"flag"
	"log/slog"
	"os"

	"github.com/mayvqt/aperture/internal/security"
)

var version = "dev"

func main() {
	if err := run(); err != nil {
		slog.Error("aperture stopped", "error", security.RedactText(err.Error(), environmentSecrets()...))
		os.Exit(1)
	}
}

func environmentSecrets() []string {
	return []string{
		os.Getenv("APERTURE_API_KEY"),
		os.Getenv("APERTURE_ENCRYPTION_KEY"),
		os.Getenv("APERTURE_SESSION_SECRET"),
		os.Getenv("APERTURE_INVITE_SECRET"),
	}
}

func init() {
	flag.CommandLine.SetOutput(os.Stdout)
}

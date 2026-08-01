package main

import (
	"flag"
	"log/slog"
	"os"
)

var version = "dev"

func main() {
	setupLogger("info",
		os.Getenv("APERTURE_API_KEY"),
		os.Getenv("APERTURE_ENCRYPTION_KEY"),
		os.Getenv("APERTURE_SESSION_SECRET"),
		os.Getenv("APERTURE_INVITE_SECRET"),
	)
	if err := run(); err != nil {
		slog.Error("aperture stopped", "error", err)
		os.Exit(1)
	}
}

func init() {
	flag.CommandLine.SetOutput(os.Stdout)
}

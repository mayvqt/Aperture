package main

import (
	"flag"
	"log/slog"
	"os"
)

var version = "dev"

func main() {
	if err := run(); err != nil {
		slog.Error("aperture stopped", "error", err)
		os.Exit(1)
	}
}

func init() {
	flag.CommandLine.SetOutput(os.Stdout)
}

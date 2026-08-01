package main

import (
	"io"
	"log/slog"
	"os"

	"github.com/mayvqt/aperture/internal/security"
)

func setupLogger(level string, secrets ...string) {
	slog.SetDefault(newLogger(os.Stdout, level, secrets...))
}

func newLogger(output io.Writer, level string, secrets ...string) *slog.Logger {
	lvl := slog.LevelInfo
	switch level {
	case "debug":
		lvl = slog.LevelDebug
	case "warn":
		lvl = slog.LevelWarn
	case "error":
		lvl = slog.LevelError
	}
	handler := slog.NewTextHandler(output, &slog.HandlerOptions{
		Level: lvl,
		ReplaceAttr: func(_ []string, attr slog.Attr) slog.Attr {
			switch attr.Value.Kind() {
			case slog.KindString:
				attr.Value = slog.StringValue(security.RedactText(attr.Value.String(), secrets...))
			case slog.KindAny:
				if err, ok := attr.Value.Any().(error); ok {
					attr.Value = slog.StringValue(security.RedactText(err.Error(), secrets...))
				}
			}
			return attr
		},
	})
	return slog.New(handler)
}

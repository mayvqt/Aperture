package main

import (
	"os"
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

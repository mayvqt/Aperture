package main

import (
	"errors"
	"fmt"
	"os"

	"github.com/mayvqt/aperture/internal/config"
)

func run() error {
	command := "serve"
	if len(os.Args) > 1 && os.Args[1][0] != '-' {
		command = os.Args[1]
	}

	switch command {
	case "serve":
		return serve(os.Args[dropCommand(os.Args):])
	case "version":
		fmt.Println(version)
		return nil
	case "config":
		if len(os.Args) > 2 && os.Args[2] == "check" {
			_, err := config.Load(os.Args[3:])
			return err
		}
		return errors.New("usage: aperture config check")
	default:
		return fmt.Errorf("unknown command %q", command)
	}
}

func dropCommand(args []string) int {
	if len(args) > 1 && args[1] == "serve" {
		return 2
	}
	return 1
}

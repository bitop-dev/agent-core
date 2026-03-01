package main

import (
	"fmt"
	"os"

	"github.com/bitop-dev/agent-core/internal/config"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	// TODO: Replace with cobra CLI setup
	fmt.Println("agent-core", config.Version)
	return nil
}

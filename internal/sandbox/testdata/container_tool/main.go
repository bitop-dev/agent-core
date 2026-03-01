// A simple tool that runs inside a Docker container.
// Reads JSON from stdin, performs a task, writes JSON to stdout.
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"runtime"
	"strings"
)

func main() {
	var input struct {
		Name      string          `json:"name"`
		Arguments json.RawMessage `json:"arguments"`
	}
	if err := json.NewDecoder(os.Stdin).Decode(&input); err != nil {
		fmt.Fprintf(os.Stderr, "decode: %v\n", err)
		os.Exit(1)
	}

	var args struct {
		Text string `json:"text"`
	}
	json.Unmarshal(input.Arguments, &args)

	if args.Text == "" {
		args.Text = "hello from container"
	}

	// Demonstrate container isolation: report hostname, OS, etc.
	hostname, _ := os.Hostname()
	output := map[string]any{
		"content":  strings.ToUpper(args.Text),
		"runtime":  "container",
		"os":       runtime.GOOS,
		"arch":     runtime.GOARCH,
		"hostname": hostname,
	}

	json.NewEncoder(os.Stdout).Encode(output)
}

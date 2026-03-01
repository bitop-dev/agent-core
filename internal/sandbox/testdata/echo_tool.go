// echo_tool is a minimal WASM tool for testing.
// It reads JSON from stdin and echoes it back with metadata.
//
// Build: GOOS=wasip1 GOARCH=wasm go build -o echo_tool.wasm ./echo_tool.go
package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
)

func main() {
	input, err := io.ReadAll(os.Stdin)
	if err != nil {
		fmt.Fprintf(os.Stderr, "read stdin: %v", err)
		os.Exit(1)
	}

	// Parse the input
	var req map[string]any
	if err := json.Unmarshal(input, &req); err != nil {
		// Not JSON — just echo raw
		result := map[string]any{
			"content":  string(input),
			"is_error": false,
		}
		json.NewEncoder(os.Stdout).Encode(result)
		return
	}

	// Extract arguments
	args, _ := req["arguments"].(map[string]any)
	message := "hello from wasm"
	if m, ok := args["message"]; ok {
		message = fmt.Sprintf("%v", m)
	}

	result := map[string]any{
		"content":  message,
		"is_error": false,
		"runtime":  "wasm",
		"tool":     req["name"],
	}
	json.NewEncoder(os.Stdout).Encode(result)
}

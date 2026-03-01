// read_file_tool is a WASM tool that reads a file.
// It demonstrates WASI filesystem capability sandboxing.
//
// Build: GOOS=wasip1 GOARCH=wasm go build -o read_file_tool.wasm ./read_file_tool.go
package main

import (
	"encoding/json"
	"io"
	"os"
)

type request struct {
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments"`
}

type readArgs struct {
	Path string `json:"path"`
}

type result struct {
	Content string `json:"content"`
	IsError bool   `json:"is_error"`
}

func main() {
	input, _ := io.ReadAll(os.Stdin)

	var req request
	if err := json.Unmarshal(input, &req); err != nil {
		writeResult(result{Content: "invalid input: " + err.Error(), IsError: true})
		return
	}

	var args readArgs
	if err := json.Unmarshal(req.Arguments, &args); err != nil {
		writeResult(result{Content: "invalid arguments: " + err.Error(), IsError: true})
		return
	}

	if args.Path == "" {
		writeResult(result{Content: "path is required", IsError: true})
		return
	}

	data, err := os.ReadFile(args.Path)
	if err != nil {
		writeResult(result{Content: "read error: " + err.Error(), IsError: true})
		return
	}

	// Truncate large files
	content := string(data)
	if len(content) > 65536 {
		content = content[:65536] + "\n...(truncated)"
	}

	writeResult(result{Content: content})
}

func writeResult(r result) {
	json.NewEncoder(os.Stdout).Encode(r)
}

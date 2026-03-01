package builtin

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/bitop-dev/agent-core/internal/tool"
)

type writeFileArgs struct {
	Path    string `json:"path"`
	Content string `json:"content"`
}

type writeFileTool struct {
	sandbox *tool.SandboxPolicy
}

// NewWriteFile creates the write_file tool. Pass nil for no sandbox.
func NewWriteFile() tool.Tool { return &writeFileTool{} }

// NewWriteFileWithSandbox creates the write_file tool with path checking.
func NewWriteFileWithSandbox(p *tool.SandboxPolicy) tool.Tool {
	return &writeFileTool{sandbox: p}
}

func (t *writeFileTool) Definition() tool.Definition {
	return tool.Definition{
		Name:        "write_file",
		Description: "Write content to a file. Creates the file if it doesn't exist, overwrites if it does. Automatically creates parent directories.",
		InputSchema: json.RawMessage(`{
			"type": "object",
			"properties": {
				"path": {
					"type": "string",
					"description": "Path to the file to write"
				},
				"content": {
					"type": "string",
					"description": "Content to write to the file"
				}
			},
			"required": ["path", "content"]
		}`),
	}
}

func (t *writeFileTool) Execute(ctx context.Context, input json.RawMessage) (tool.Result, error) {
	var args writeFileArgs
	if err := json.Unmarshal(input, &args); err != nil {
		return tool.Result{Content: fmt.Sprintf("invalid arguments: %v", err), IsError: true}, nil
	}
	if args.Path == "" {
		return tool.Result{Content: "path is required", IsError: true}, nil
	}

	// Check sandbox path restrictions
	if t.sandbox != nil {
		if err := t.sandbox.CheckPath(args.Path); err != nil {
			return tool.Result{Content: err.Error(), IsError: true}, nil
		}
	}

	// Create parent directories
	dir := filepath.Dir(args.Path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return tool.Result{Content: fmt.Sprintf("cannot create directory: %v", err), IsError: true}, nil
	}

	if err := os.WriteFile(args.Path, []byte(args.Content), 0644); err != nil {
		return tool.Result{Content: fmt.Sprintf("cannot write file: %v", err), IsError: true}, nil
	}

	return tool.Result{Content: fmt.Sprintf("wrote %d bytes to %s", len(args.Content), args.Path)}, nil
}

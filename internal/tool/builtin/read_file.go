package builtin

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/bitop-dev/agent-core/internal/tool"
)

type readFileArgs struct {
	Path   string `json:"path"`
	Offset int    `json:"offset,omitempty"`
	Limit  int    `json:"limit,omitempty"`
}

type readFileTool struct{}

func NewReadFile() tool.Tool { return &readFileTool{} }

func (t *readFileTool) Definition() tool.Definition {
	return tool.Definition{
		Name:        "read_file",
		Description: "Read the contents of a file. Supports offset and limit for reading portions of large files.",
		InputSchema: json.RawMessage(`{
			"type": "object",
			"properties": {
				"path": {
					"type": "string",
					"description": "Path to the file to read"
				},
				"offset": {
					"type": "integer",
					"description": "Line number to start reading from (1-indexed, default: 1)"
				},
				"limit": {
					"type": "integer",
					"description": "Maximum number of lines to read (default: all)"
				}
			},
			"required": ["path"]
		}`),
	}
}

func (t *readFileTool) Execute(ctx context.Context, input json.RawMessage) (tool.Result, error) {
	var args readFileArgs
	if err := json.Unmarshal(input, &args); err != nil {
		return tool.Result{Content: fmt.Sprintf("invalid arguments: %v", err), IsError: true}, nil
	}
	if args.Path == "" {
		return tool.Result{Content: "path is required", IsError: true}, nil
	}

	f, err := os.Open(args.Path)
	if err != nil {
		return tool.Result{Content: fmt.Sprintf("cannot open file: %v", err), IsError: true}, nil
	}
	defer f.Close()

	offset := args.Offset
	if offset <= 0 {
		offset = 1
	}

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 1024*1024), 1024*1024)

	var lines []string
	lineNum := 0
	for scanner.Scan() {
		lineNum++
		if lineNum < offset {
			continue
		}
		if args.Limit > 0 && len(lines) >= args.Limit {
			break
		}
		lines = append(lines, scanner.Text())
	}

	if err := scanner.Err(); err != nil {
		return tool.Result{Content: fmt.Sprintf("read error: %v", err), IsError: true}, nil
	}

	if len(lines) == 0 {
		return tool.Result{Content: "(empty file or offset past end)"}, nil
	}

	content := strings.Join(lines, "\n")

	// Truncate if very large
	const maxBytes = 512 * 1024
	if len(content) > maxBytes {
		content = content[:maxBytes] + "\n... (truncated)"
	}

	return tool.Result{Content: content}, nil
}

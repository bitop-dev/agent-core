package builtin

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/bitop-dev/agent-core/internal/tool"
)

type listDirArgs struct {
	Path string `json:"path"`
}

type listDirTool struct{}

func NewListDir() tool.Tool { return &listDirTool{} }

func (t *listDirTool) Definition() tool.Definition {
	return tool.Definition{
		Name:        "list_dir",
		Description: "List directory contents with file type, size, and modification time.",
		InputSchema: json.RawMessage(`{
			"type": "object",
			"properties": {
				"path": {
					"type": "string",
					"description": "Directory path to list"
				}
			},
			"required": ["path"]
		}`),
	}
}

func (t *listDirTool) Execute(ctx context.Context, input json.RawMessage) (tool.Result, error) {
	var args listDirArgs
	if err := json.Unmarshal(input, &args); err != nil {
		return tool.Result{Content: fmt.Sprintf("invalid arguments: %v", err), IsError: true}, nil
	}
	if args.Path == "" {
		args.Path = "."
	}

	entries, err := os.ReadDir(args.Path)
	if err != nil {
		return tool.Result{Content: fmt.Sprintf("cannot read directory: %v", err), IsError: true}, nil
	}

	var lines []string
	for _, entry := range entries {
		info, err := entry.Info()
		if err != nil {
			continue
		}
		kind := "file"
		if entry.IsDir() {
			kind = "dir "
		}
		lines = append(lines, fmt.Sprintf("%s  %8d  %s  %s",
			kind,
			info.Size(),
			info.ModTime().Format("2006-01-02 15:04"),
			entry.Name(),
		))
	}

	if len(lines) == 0 {
		return tool.Result{Content: "(empty directory)"}, nil
	}
	return tool.Result{Content: strings.Join(lines, "\n")}, nil
}

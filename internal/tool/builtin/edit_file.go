package builtin

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/bitop-dev/agent-core/internal/tool"
)

type editFileArgs struct {
	Path    string `json:"path"`
	OldText string `json:"old_text"`
	NewText string `json:"new_text"`
}

type editFileTool struct {
	sandbox *tool.SandboxPolicy
}

func NewEditFile() tool.Tool { return &editFileTool{} }

func NewEditFileWithSandbox(p *tool.SandboxPolicy) tool.Tool {
	return &editFileTool{sandbox: p}
}

func (t *editFileTool) Definition() tool.Definition {
	return tool.Definition{
		Name:        "edit_file",
		Description: "Edit a file by replacing exact text. The old_text must match exactly (including whitespace). Use for precise, surgical edits.",
		InputSchema: json.RawMessage(`{
			"type": "object",
			"properties": {
				"path": {
					"type": "string",
					"description": "Path to the file to edit"
				},
				"old_text": {
					"type": "string",
					"description": "Exact text to find and replace"
				},
				"new_text": {
					"type": "string",
					"description": "New text to replace old_text with"
				}
			},
			"required": ["path", "old_text", "new_text"]
		}`),
	}
}

func (t *editFileTool) Execute(ctx context.Context, input json.RawMessage) (tool.Result, error) {
	var args editFileArgs
	if err := json.Unmarshal(input, &args); err != nil {
		return tool.Result{Content: fmt.Sprintf("invalid arguments: %v", err), IsError: true}, nil
	}
	if args.Path == "" {
		return tool.Result{Content: "path is required", IsError: true}, nil
	}
	if args.OldText == "" {
		return tool.Result{Content: "old_text is required", IsError: true}, nil
	}

	if t.sandbox != nil {
		if err := t.sandbox.CheckPath(args.Path); err != nil {
			return tool.Result{Content: err.Error(), IsError: true}, nil
		}
	}

	data, err := os.ReadFile(args.Path)
	if err != nil {
		return tool.Result{Content: fmt.Sprintf("cannot read file: %v", err), IsError: true}, nil
	}

	content := string(data)
	count := strings.Count(content, args.OldText)

	if count == 0 {
		return tool.Result{Content: "old_text not found in file", IsError: true}, nil
	}
	if count > 1 {
		return tool.Result{Content: fmt.Sprintf("old_text found %d times — must be unique", count), IsError: true}, nil
	}

	newContent := strings.Replace(content, args.OldText, args.NewText, 1)
	if err := os.WriteFile(args.Path, []byte(newContent), 0644); err != nil {
		return tool.Result{Content: fmt.Sprintf("cannot write file: %v", err), IsError: true}, nil
	}

	return tool.Result{Content: fmt.Sprintf("edited %s", args.Path)}, nil
}

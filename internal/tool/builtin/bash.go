package builtin

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"time"

	"github.com/bitop-dev/agent-core/internal/tool"
)

type bashArgs struct {
	Command string `json:"command"`
	Timeout int    `json:"timeout,omitempty"`
}

type bashTool struct {
	sandbox    *tool.SandboxPolicy
	workingDir string
}

// NewBash creates the bash tool with default settings.
func NewBash() tool.Tool { return &bashTool{} }

// NewBashWithSandbox creates the bash tool with sandbox policy.
// workingDir sets the subprocess working directory (empty = inherit).
func NewBashWithSandbox(p *tool.SandboxPolicy, workingDir string) tool.Tool {
	return &bashTool{sandbox: p, workingDir: workingDir}
}

func (t *bashTool) Definition() tool.Definition {
	return tool.Definition{
		Name:        "bash",
		Description: "Execute a bash command. Returns stdout and stderr. Use for running programs, installing packages, or any shell operation.",
		InputSchema: json.RawMessage(`{
			"type": "object",
			"properties": {
				"command": {
					"type": "string",
					"description": "The bash command to execute"
				},
				"timeout": {
					"type": "integer",
					"description": "Timeout in seconds (default: 60)"
				}
			},
			"required": ["command"]
		}`),
	}
}

func (t *bashTool) Execute(ctx context.Context, input json.RawMessage) (tool.Result, error) {
	var args bashArgs
	if err := json.Unmarshal(input, &args); err != nil {
		return tool.Result{Content: fmt.Sprintf("invalid arguments: %v", err), IsError: true}, nil
	}
	if args.Command == "" {
		return tool.Result{Content: "command is required", IsError: true}, nil
	}

	timeout := time.Duration(args.Timeout) * time.Second
	if timeout <= 0 {
		if t.sandbox != nil {
			timeout = time.Duration(t.sandbox.Timeout()) * time.Second
		} else {
			timeout = 60 * time.Second
		}
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "bash", "-c", args.Command)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	// Apply sandbox: working directory and filtered environment
	if t.workingDir != "" {
		cmd.Dir = t.workingDir
	}
	if t.sandbox != nil {
		if env := t.sandbox.FilteredEnv(); env != nil {
			cmd.Env = env
		}
	}

	err := cmd.Run()

	var result string
	if stdout.Len() > 0 {
		result = stdout.String()
	}
	if stderr.Len() > 0 {
		if result != "" {
			result += "\n"
		}
		result += stderr.String()
	}

	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return tool.Result{Content: fmt.Sprintf("command timed out after %ds\n%s", int(timeout.Seconds()), result), IsError: true}, nil
		}
		if result == "" {
			result = err.Error()
		}
		return tool.Result{Content: result, IsError: true}, nil
	}

	if result == "" {
		result = "(no output)"
	}
	return tool.Result{Content: result}, nil
}

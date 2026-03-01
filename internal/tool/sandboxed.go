package tool

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/bitop-dev/agent-core/internal/sandbox"
)

// SandboxedTool wraps an external tool to execute through a sandbox runtime
// (WASM, container, or subprocess). It implements the Tool interface so it
// can be registered in the Engine like any other tool.
type SandboxedTool struct {
	def     Definition
	runtime sandbox.RuntimeType
	module  string // path to .wasm file, container image, or executable
	workDir string
	caps    sandbox.Capabilities
	reg     *sandbox.Registry
	config  map[string]any // per-skill config from agent YAML
}

// SandboxedToolConfig configures a sandboxed tool.
type SandboxedToolConfig struct {
	// Def is the tool definition (name, description, schema).
	Def Definition

	// Runtime is which sandbox to use (wasm, container, subprocess).
	Runtime sandbox.RuntimeType

	// Module is the path to the WASM module, container image, or executable.
	Module string

	// WorkDir is the working directory for execution.
	WorkDir string

	// Caps is the capability grant for this tool.
	Caps sandbox.Capabilities

	// Registry is the sandbox runtime registry.
	Registry *sandbox.Registry

	// SkillConfig is per-skill config from the agent YAML.
	SkillConfig map[string]any
}

// NewSandboxedTool creates a tool that executes through the sandbox system.
func NewSandboxedTool(cfg SandboxedToolConfig) *SandboxedTool {
	return &SandboxedTool{
		def:     cfg.Def,
		runtime: cfg.Runtime,
		module:  cfg.Module,
		workDir: cfg.WorkDir,
		caps:    cfg.Caps,
		reg:     cfg.Registry,
		config:  cfg.SkillConfig,
	}
}

func (t *SandboxedTool) Definition() Definition {
	return t.def
}

func (t *SandboxedTool) Execute(ctx context.Context, input json.RawMessage) (Result, error) {
	// Build the stdin payload (standard tool protocol: name + arguments + config)
	payload := map[string]any{
		"name":      t.def.Name,
		"arguments": json.RawMessage(input),
		"config":    t.config,
	}
	inputBytes, err := json.Marshal(payload)
	if err != nil {
		return Result{Content: fmt.Sprintf("marshal input: %v", err), IsError: true}, nil
	}

	inv := sandbox.ToolInvocation{
		Name:    t.def.Name,
		Input:   inputBytes,
		Module:  t.module,
		WorkDir: t.workDir,
	}

	out, err := t.reg.Execute(ctx, t.runtime, inv, t.caps)
	if err != nil {
		return Result{Content: fmt.Sprintf("sandbox error: %v", err), IsError: true}, nil
	}

	// Non-zero exit code → error
	if out.ExitCode != 0 {
		content := string(out.Stdout)
		if content == "" {
			content = string(out.Stderr)
		}
		if content == "" {
			content = fmt.Sprintf("tool exited with code %d", out.ExitCode)
		}
		return Result{Content: content, IsError: true}, nil
	}

	// Try to parse as tool Result JSON
	var result Result
	if err := json.Unmarshal(out.Stdout, &result); err != nil {
		// Not JSON — return raw stdout as content
		return Result{Content: string(out.Stdout)}, nil
	}
	return result, nil
}

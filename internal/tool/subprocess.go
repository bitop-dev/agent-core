package tool

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"time"
)

// SubprocessConfig defines constraints for running an external tool process.
type SubprocessConfig struct {
	Command        string            // path to the executable
	TimeoutSeconds int               // kill after N seconds (default: 30)
	WorkDir        string            // locked working directory
	EnvAllowlist   []string          // only these env vars pass through
	MaxOutputBytes int64             // stdout cap (default: 1MB)
	SkillConfig    map[string]any    // per-skill config from agent YAML
}

// SubprocessTool wraps an external tool executable that communicates via stdin/stdout JSON.
type SubprocessTool struct {
	def    Definition
	config SubprocessConfig
}

// NewSubprocessTool creates a tool backed by an external process.
func NewSubprocessTool(def Definition, cfg SubprocessConfig) *SubprocessTool {
	if cfg.TimeoutSeconds <= 0 {
		cfg.TimeoutSeconds = 30
	}
	if cfg.MaxOutputBytes <= 0 {
		cfg.MaxOutputBytes = 1 << 20 // 1MB
	}
	return &SubprocessTool{def: def, config: cfg}
}

func (t *SubprocessTool) Definition() Definition {
	return t.def
}

func (t *SubprocessTool) Execute(ctx context.Context, input json.RawMessage) (Result, error) {
	timeout := time.Duration(t.config.TimeoutSeconds) * time.Second
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	// Build the stdin payload with config
	payload := map[string]any{
		"name":      t.def.Name,
		"arguments": json.RawMessage(input),
		"config":    t.config.SkillConfig,
	}
	stdinBytes, err := json.Marshal(payload)
	if err != nil {
		return Result{}, fmt.Errorf("marshal stdin: %w", err)
	}

	cmd := exec.CommandContext(ctx, t.config.Command)
	cmd.Stdin = bytes.NewReader(stdinBytes)
	cmd.Dir = t.config.WorkDir

	// TODO: Filter env vars to allowlist only
	// TODO: Capture stderr separately for logging

	stdout, err := cmd.Output()
	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return Result{Content: fmt.Sprintf("tool timed out after %ds", t.config.TimeoutSeconds), IsError: true}, nil
		}
		return Result{Content: fmt.Sprintf("tool process error: %v", err), IsError: true}, nil
	}

	// Truncate if over limit
	if int64(len(stdout)) > t.config.MaxOutputBytes {
		stdout = stdout[:t.config.MaxOutputBytes]
	}

	var result Result
	if err := json.Unmarshal(stdout, &result); err != nil {
		return Result{Content: fmt.Sprintf("invalid tool output JSON: %v", err), IsError: true}, nil
	}
	return result, nil
}

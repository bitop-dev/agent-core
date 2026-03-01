package sandbox

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"time"
)

// SubprocessRuntime executes tools as raw OS processes.
// This is the legacy/development path — no real sandboxing,
// but compatible with any executable (Python, shell, Go binary, etc.).
type SubprocessRuntime struct{}

// NewSubprocessRuntime creates a subprocess runtime.
func NewSubprocessRuntime() *SubprocessRuntime {
	return &SubprocessRuntime{}
}

func (s *SubprocessRuntime) Type() RuntimeType {
	return RuntimeSubprocess
}

func (s *SubprocessRuntime) Execute(ctx context.Context, inv ToolInvocation, caps Capabilities) (*ToolOutput, error) {
	timeout := time.Duration(caps.MaxTimeoutSec) * time.Second
	if timeout <= 0 {
		timeout = 60 * time.Second
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, inv.Module)
	cmd.Stdin = bytes.NewReader(inv.Input)
	cmd.Dir = inv.WorkDir

	// Build filtered environment
	env := []string{
		"PATH=" + os.Getenv("PATH"),
		"HOME=" + os.Getenv("HOME"),
		"TMPDIR=" + os.Getenv("TMPDIR"),
	}
	for k, v := range caps.EnvVars {
		env = append(env, fmt.Sprintf("%s=%s", k, v))
	}
	cmd.Env = env

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()

	exitCode := 0
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		} else if ctx.Err() != nil {
			return &ToolOutput{
				Stderr:   []byte(fmt.Sprintf("process timed out after %s", timeout)),
				ExitCode: 124,
			}, nil
		} else {
			return nil, fmt.Errorf("exec %s: %w", inv.Module, err)
		}
	}

	out := stdout.Bytes()
	if caps.MaxOutputBytes > 0 && len(out) > caps.MaxOutputBytes {
		out = out[:caps.MaxOutputBytes]
	}

	return &ToolOutput{
		Stdout:   out,
		Stderr:   stderr.Bytes(),
		ExitCode: exitCode,
	}, nil
}

func (s *SubprocessRuntime) Close() error {
	return nil
}

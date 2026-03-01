package sandbox

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

// ContainerRuntime executes tools inside OCI containers (Docker/Podman).
// Each invocation gets its own ephemeral container.
type ContainerRuntime struct {
	// engine is "docker" or "podman" — auto-detected.
	engine string
}

// NewContainerRuntime creates a container runtime.
// It auto-detects whether Docker or Podman is available.
func NewContainerRuntime() (*ContainerRuntime, error) {
	// Prefer podman (daemonless), fall back to docker
	for _, eng := range []string{"podman", "docker"} {
		if path, err := exec.LookPath(eng); err == nil {
			_ = path
			return &ContainerRuntime{engine: eng}, nil
		}
	}
	return nil, fmt.Errorf("neither docker nor podman found in PATH")
}

func (c *ContainerRuntime) Type() RuntimeType {
	return RuntimeContainer
}

func (c *ContainerRuntime) Execute(ctx context.Context, inv ToolInvocation, caps Capabilities) (*ToolOutput, error) {
	// Apply timeout
	timeout := time.Duration(caps.MaxTimeoutSec) * time.Second
	if timeout <= 0 {
		timeout = 60 * time.Second
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	// Build docker/podman run command
	args := []string{
		"run",
		"--rm",          // remove container after exit
		"-i",            // attach stdin
		"--network=none", // no network by default
	}

	// Memory limit
	memMB := caps.MaxMemoryMB
	if memMB <= 0 {
		memMB = 256
	}
	args = append(args, fmt.Sprintf("--memory=%dm", memMB))

	// CPU limit — 1 core
	args = append(args, "--cpus=1")

	// No new privileges
	args = append(args, "--security-opt=no-new-privileges")

	// Read-only root filesystem
	args = append(args, "--read-only")

	// Tmpfs for /tmp
	args = append(args, "--tmpfs=/tmp:rw,noexec,nosuid,size=64m")

	// Mount allowed paths
	for _, p := range caps.AllowedPaths {
		args = append(args, "-v", fmt.Sprintf("%s:%s:rw", p, p))
	}
	for _, p := range caps.ReadOnlyPaths {
		args = append(args, "-v", fmt.Sprintf("%s:%s:ro", p, p))
	}

	// Network access — if hosts are specified, enable networking
	if len(caps.AllowedHosts) > 0 {
		// Remove --network=none, add default networking
		// Note: fine-grained host filtering would require iptables/network policies
		for i, a := range args {
			if a == "--network=none" {
				args = append(args[:i], args[i+1:]...)
				break
			}
		}
	}

	// Environment variables
	for k, v := range caps.EnvVars {
		args = append(args, "-e", fmt.Sprintf("%s=%s", k, v))
	}

	// Working directory
	if inv.WorkDir != "" {
		args = append(args, "-w", inv.WorkDir)
	}

	// Image — inv.Module is the container image reference
	args = append(args, inv.Module)

	// Build the command
	cmd := exec.CommandContext(ctx, c.engine, args...)
	cmd.Stdin = bytes.NewReader(inv.Input)

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
				Stderr:   []byte(fmt.Sprintf("container timed out after %s", timeout)),
				ExitCode: 124,
			}, nil
		} else {
			return nil, fmt.Errorf("%s run failed: %w\nstderr: %s", c.engine, err, stderr.String())
		}
	}

	// Truncate output
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

// PullImage pulls a container image if not already present.
func (c *ContainerRuntime) PullImage(ctx context.Context, image string) error {
	// Check if image exists locally
	check := exec.CommandContext(ctx, c.engine, "image", "inspect", image)
	if check.Run() == nil {
		return nil // already present
	}

	cmd := exec.CommandContext(ctx, c.engine, "pull", image)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("pull %s: %w: %s", image, err, strings.TrimSpace(stderr.String()))
	}
	return nil
}

// Engine returns "docker" or "podman".
func (c *ContainerRuntime) Engine() string {
	return c.engine
}

func (c *ContainerRuntime) Close() error {
	return nil // no persistent state
}

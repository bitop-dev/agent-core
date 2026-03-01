package sandbox

import (
	"context"
	"encoding/json"
	"os/exec"
	"testing"
	"time"
)

// TestContainerRuntime_E2E tests the full container runtime pipeline:
// create runtime → execute tool in Docker → verify JSON output.
//
// Requires: Docker running, image "agent-core-test-tool:latest" built.
// Build with: docker build -t agent-core-test-tool:latest internal/sandbox/testdata/container_tool/
func TestContainerRuntime_E2E(t *testing.T) {
	// Skip if Docker not available
	if _, err := exec.LookPath("docker"); err != nil {
		t.Skip("docker not found, skipping container E2E test")
	}

	// Skip if test image not built
	check := exec.Command("docker", "image", "inspect", "agent-core-test-tool:latest")
	if err := check.Run(); err != nil {
		t.Skip("agent-core-test-tool:latest not built, skipping (build with: docker build -t agent-core-test-tool:latest internal/sandbox/testdata/container_tool/)")
	}

	ctx := context.Background()

	rt, err := NewContainerRuntime()
	if err != nil {
		t.Fatalf("NewContainerRuntime: %v", err)
	}
	defer rt.Close()

	t.Logf("Container engine: %s", rt.Engine())

	if rt.Type() != RuntimeContainer {
		t.Errorf("expected type %q, got %q", RuntimeContainer, rt.Type())
	}

	t.Run("BasicExecution", func(t *testing.T) {
		input := map[string]any{
			"name":      "uppercase",
			"arguments": map[string]any{"text": "container sandbox test"},
		}
		inputBytes, _ := json.Marshal(input)

		caps := DefaultCapabilities()
		caps.MaxTimeoutSec = 30

		start := time.Now()
		out, err := rt.Execute(ctx, ToolInvocation{
			Name:   "uppercase",
			Input:  inputBytes,
			Module: "agent-core-test-tool:latest", // Module = image name for containers
		}, caps)
		dur := time.Since(start)

		if err != nil {
			t.Fatalf("Execute: %v", err)
		}

		t.Logf("Container execution: %v", dur)
		t.Logf("Stdout: %s", out.Stdout)
		t.Logf("Stderr: %s", out.Stderr)
		t.Logf("Exit code: %d", out.ExitCode)

		if out.ExitCode != 0 {
			t.Fatalf("exit code %d, stderr: %s", out.ExitCode, out.Stderr)
		}

		var result map[string]any
		if err := json.Unmarshal(out.Stdout, &result); err != nil {
			t.Fatalf("unmarshal output: %v\nraw: %s", err, out.Stdout)
		}

		if result["content"] != "CONTAINER SANDBOX TEST" {
			t.Errorf("expected 'CONTAINER SANDBOX TEST', got %q", result["content"])
		}
		if result["runtime"] != "container" {
			t.Errorf("expected runtime 'container', got %q", result["runtime"])
		}
		if result["os"] != "linux" {
			t.Errorf("expected os 'linux', got %q", result["os"])
		}

		// Verify it ran in a container (hostname should be a container ID, not the host)
		hostname, _ := result["hostname"].(string)
		if hostname == "" {
			t.Error("expected hostname from container")
		}
		t.Logf("Container hostname: %s", hostname)
	})

	t.Run("DefaultMessage", func(t *testing.T) {
		input := map[string]any{
			"name":      "uppercase",
			"arguments": map[string]any{},
		}
		inputBytes, _ := json.Marshal(input)

		out, err := rt.Execute(ctx, ToolInvocation{
			Name:   "uppercase",
			Input:  inputBytes,
			Module: "agent-core-test-tool:latest",
		}, DefaultCapabilities())
		if err != nil {
			t.Fatalf("Execute: %v", err)
		}

		var result map[string]any
		json.Unmarshal(out.Stdout, &result)

		if result["content"] != "HELLO FROM CONTAINER" {
			t.Errorf("expected default message, got %q", result["content"])
		}
	})

	t.Run("SecurityConstraints", func(t *testing.T) {
		// Container runs with --read-only, --no-new-privileges, --network=none
		// We can't easily verify these from outside, but we can verify the tool still works
		caps := DefaultCapabilities()
		caps.MaxMemoryMB = 64 // tight memory limit

		input := map[string]any{
			"name":      "uppercase",
			"arguments": map[string]any{"text": "security test"},
		}
		inputBytes, _ := json.Marshal(input)

		out, err := rt.Execute(ctx, ToolInvocation{
			Name: "uppercase", Input: inputBytes, Module: "agent-core-test-tool:latest",
		}, caps)
		if err != nil {
			t.Fatalf("Execute: %v", err)
		}
		if out.ExitCode != 0 {
			t.Errorf("exit code %d under memory constraint", out.ExitCode)
		}
	})

	t.Run("RegistryDispatch", func(t *testing.T) {
		// Test through the registry dispatch layer
		reg := NewRegistry()
		reg.Register(rt)

		input := map[string]any{
			"name":      "uppercase",
			"arguments": map[string]any{"text": "registry dispatch"},
		}
		inputBytes, _ := json.Marshal(input)

		out, err := reg.Execute(ctx, RuntimeContainer, ToolInvocation{
			Name: "uppercase", Input: inputBytes, Module: "agent-core-test-tool:latest",
		}, DefaultCapabilities())
		if err != nil {
			t.Fatalf("Registry Execute: %v", err)
		}

		var result map[string]any
		json.Unmarshal(out.Stdout, &result)

		if result["content"] != "REGISTRY DISPATCH" {
			t.Errorf("expected 'REGISTRY DISPATCH', got %q", result["content"])
		}
		t.Logf("✅ Container dispatched through registry")
	})
}

// TestContainerRuntime_PullImage tests image pull functionality.
func TestContainerRuntime_PullImage(t *testing.T) {
	if _, err := exec.LookPath("docker"); err != nil {
		t.Skip("docker not found")
	}

	rt, err := NewContainerRuntime()
	if err != nil {
		t.Fatal(err)
	}

	// Pull a tiny image
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	err = rt.PullImage(ctx, "alpine:3.21")
	if err != nil {
		t.Fatalf("PullImage: %v", err)
	}
	t.Log("✅ PullImage succeeded (alpine:3.21)")

	// Pull same image again (should be instant — already cached)
	start := time.Now()
	err = rt.PullImage(ctx, "alpine:3.21")
	if err != nil {
		t.Fatalf("PullImage cached: %v", err)
	}
	t.Logf("✅ PullImage cached: %v", time.Since(start))
}

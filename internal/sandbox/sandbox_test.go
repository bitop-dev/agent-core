package sandbox

import (
	"context"
	"encoding/json"
	"path/filepath"
	"runtime"
	"testing"
)

func testdataDir() string {
	_, file, _, _ := runtime.Caller(0)
	return filepath.Join(filepath.Dir(file), "testdata")
}

// ─── WASM Runtime Tests ─────────────────────────────────────────────────────

func TestWASMRuntime_Type(t *testing.T) {
	ctx := context.Background()
	rt, err := NewWASMRuntime(ctx)
	if err != nil {
		t.Fatalf("NewWASMRuntime: %v", err)
	}
	defer rt.Close()

	if rt.Type() != RuntimeWASM {
		t.Errorf("expected type %q, got %q", RuntimeWASM, rt.Type())
	}
}

func TestWASMRuntime_EchoTool(t *testing.T) {
	ctx := context.Background()
	rt, err := NewWASMRuntime(ctx)
	if err != nil {
		t.Fatalf("NewWASMRuntime: %v", err)
	}
	defer rt.Close()

	wasmPath := filepath.Join(testdataDir(), "echo_tool.wasm")

	input := map[string]any{
		"name":      "echo",
		"arguments": map[string]any{"message": "sandboxed hello"},
	}
	inputBytes, _ := json.Marshal(input)

	inv := ToolInvocation{
		Name:   "echo",
		Input:  inputBytes,
		Module: wasmPath,
	}

	out, err := rt.Execute(ctx, inv, DefaultCapabilities())
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}

	if out.ExitCode != 0 {
		t.Fatalf("exit code %d, stderr: %s", out.ExitCode, out.Stderr)
	}

	var result map[string]any
	if err := json.Unmarshal(out.Stdout, &result); err != nil {
		t.Fatalf("unmarshal output: %v\nraw: %s", err, out.Stdout)
	}

	if result["content"] != "sandboxed hello" {
		t.Errorf("expected content 'sandboxed hello', got %q", result["content"])
	}
	if result["runtime"] != "wasm" {
		t.Errorf("expected runtime 'wasm', got %q", result["runtime"])
	}

	t.Logf("WASM output: %s", out.Stdout)
}

func TestWASMRuntime_DefaultMessage(t *testing.T) {
	ctx := context.Background()
	rt, err := NewWASMRuntime(ctx)
	if err != nil {
		t.Fatalf("NewWASMRuntime: %v", err)
	}
	defer rt.Close()

	wasmPath := filepath.Join(testdataDir(), "echo_tool.wasm")

	// No message argument — should use default
	input := map[string]any{
		"name":      "echo",
		"arguments": map[string]any{},
	}
	inputBytes, _ := json.Marshal(input)

	out, err := rt.Execute(ctx, ToolInvocation{
		Name: "echo", Input: inputBytes, Module: wasmPath,
	}, DefaultCapabilities())
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}

	var result map[string]any
	json.Unmarshal(out.Stdout, &result)

	if result["content"] != "hello from wasm" {
		t.Errorf("expected default message, got %q", result["content"])
	}
}

func TestWASMRuntime_Timeout(t *testing.T) {
	ctx := context.Background()
	rt, err := NewWASMRuntime(ctx)
	if err != nil {
		t.Fatalf("NewWASMRuntime: %v", err)
	}
	defer rt.Close()

	wasmPath := filepath.Join(testdataDir(), "echo_tool.wasm")

	caps := DefaultCapabilities()
	caps.MaxTimeoutSec = 1 // 1 second — should be plenty for echo

	out, err := rt.Execute(ctx, ToolInvocation{
		Name: "echo", Input: []byte(`{"name":"echo","arguments":{}}`), Module: wasmPath,
	}, caps)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if out.ExitCode != 0 {
		t.Errorf("expected success, got exit %d", out.ExitCode)
	}
}

func TestWASMRuntime_BadModule(t *testing.T) {
	ctx := context.Background()
	rt, err := NewWASMRuntime(ctx)
	if err != nil {
		t.Fatalf("NewWASMRuntime: %v", err)
	}
	defer rt.Close()

	_, err = rt.Execute(ctx, ToolInvocation{
		Name: "bad", Input: []byte(`{}`), Module: "/nonexistent.wasm",
	}, DefaultCapabilities())
	if err == nil {
		t.Error("expected error for missing module")
	}
}

// ─── Subprocess Runtime Tests ────────────────────────────────────────────────

func TestSubprocessRuntime_Type(t *testing.T) {
	rt := NewSubprocessRuntime()
	if rt.Type() != RuntimeSubprocess {
		t.Errorf("expected type %q, got %q", RuntimeSubprocess, rt.Type())
	}
}

func TestSubprocessRuntime_Echo(t *testing.T) {
	rt := NewSubprocessRuntime()

	out, err := rt.Execute(context.Background(), ToolInvocation{
		Name:   "echo",
		Input:  []byte(`hello subprocess`),
		Module: "/bin/cat", // cat reads stdin → stdout
	}, DefaultCapabilities())
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if out.ExitCode != 0 {
		t.Errorf("exit code %d", out.ExitCode)
	}
	if string(out.Stdout) != "hello subprocess" {
		t.Errorf("expected 'hello subprocess', got %q", out.Stdout)
	}
}

// ─── Registry Tests ──────────────────────────────────────────────────────────

func TestRegistry_Dispatch(t *testing.T) {
	ctx := context.Background()

	reg := NewRegistry()

	// Register subprocess runtime
	reg.Register(NewSubprocessRuntime())

	// Register WASM runtime
	wrt, err := NewWASMRuntime(ctx)
	if err != nil {
		t.Fatalf("NewWASMRuntime: %v", err)
	}
	defer wrt.Close()
	reg.Register(wrt)

	// Dispatch to subprocess
	out, err := reg.Execute(ctx, RuntimeSubprocess, ToolInvocation{
		Name: "cat", Input: []byte("from registry"), Module: "/bin/cat",
	}, DefaultCapabilities())
	if err != nil {
		t.Fatalf("subprocess dispatch: %v", err)
	}
	if string(out.Stdout) != "from registry" {
		t.Errorf("subprocess: got %q", out.Stdout)
	}

	// Dispatch to WASM
	wasmPath := filepath.Join(testdataDir(), "echo_tool.wasm")
	out, err = reg.Execute(ctx, RuntimeWASM, ToolInvocation{
		Name: "echo", Input: []byte(`{"name":"echo","arguments":{"message":"from registry"}}`), Module: wasmPath,
	}, DefaultCapabilities())
	if err != nil {
		t.Fatalf("wasm dispatch: %v", err)
	}
	var result map[string]any
	json.Unmarshal(out.Stdout, &result)
	if result["content"] != "from registry" {
		t.Errorf("wasm: got %q", result["content"])
	}

	// Dispatch to unregistered runtime
	_, err = reg.Execute(ctx, RuntimeContainer, ToolInvocation{}, DefaultCapabilities())
	if err == nil {
		t.Error("expected error for unregistered runtime")
	}
}

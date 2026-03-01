package sandbox

import (
	"context"
	"encoding/json"
	"path/filepath"
	"runtime"
	"testing"
	"time"
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
	caps.MaxTimeoutSec = 1

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

// ─── Module Cache Tests ──────────────────────────────────────────────────────

func TestWASMRuntime_ModuleCache(t *testing.T) {
	ctx := context.Background()
	rt, err := NewWASMRuntime(ctx)
	if err != nil {
		t.Fatalf("NewWASMRuntime: %v", err)
	}
	defer rt.Close()

	wasmPath := filepath.Join(testdataDir(), "echo_tool.wasm")
	input := []byte(`{"name":"echo","arguments":{"message":"cache test"}}`)
	caps := DefaultCapabilities()

	// First call — compiles the module
	start1 := time.Now()
	out1, err := rt.Execute(ctx, ToolInvocation{Name: "echo", Input: input, Module: wasmPath}, caps)
	dur1 := time.Since(start1)
	if err != nil {
		t.Fatalf("first Execute: %v", err)
	}
	if out1.ExitCode != 0 {
		t.Fatalf("first call failed: %s", out1.Stderr)
	}

	// Cache should have 1 entry
	if rt.CacheSize() != 1 {
		t.Errorf("expected cache size 1, got %d", rt.CacheSize())
	}

	// Second call — should use cached compiled module (faster)
	start2 := time.Now()
	out2, err := rt.Execute(ctx, ToolInvocation{Name: "echo", Input: input, Module: wasmPath}, caps)
	dur2 := time.Since(start2)
	if err != nil {
		t.Fatalf("second Execute: %v", err)
	}
	if out2.ExitCode != 0 {
		t.Fatalf("second call failed: %s", out2.Stderr)
	}

	// Cache should still have 1 entry (same module)
	if rt.CacheSize() != 1 {
		t.Errorf("expected cache size 1, got %d", rt.CacheSize())
	}

	t.Logf("First call:  %v", dur1)
	t.Logf("Second call: %v (cached)", dur2)
	t.Logf("Speedup:     %.1fx", float64(dur1)/float64(dur2))

	// Cached call should be meaningfully faster
	if dur2 >= dur1 {
		t.Logf("⚠ Cached call was not faster (may be noise on fast hardware)")
	} else {
		t.Logf("✅ Cache working — %v → %v", dur1, dur2)
	}
}

// ─── Registry Tests ──────────────────────────────────────────────────────────

func TestRegistry_Dispatch(t *testing.T) {
	ctx := context.Background()

	reg := NewRegistry()

	// Register WASM runtime
	wrt, err := NewWASMRuntime(ctx)
	if err != nil {
		t.Fatalf("NewWASMRuntime: %v", err)
	}
	defer wrt.Close()
	reg.Register(wrt)

	// Dispatch to WASM
	wasmPath := filepath.Join(testdataDir(), "echo_tool.wasm")
	out, err := reg.Execute(ctx, RuntimeWASM, ToolInvocation{
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

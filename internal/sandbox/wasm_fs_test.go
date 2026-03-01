package sandbox

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestWASM_FileRead_Allowed(t *testing.T) {
	ctx := context.Background()
	rt, err := NewWASMRuntime(ctx)
	if err != nil {
		t.Fatalf("NewWASMRuntime: %v", err)
	}
	defer rt.Close()

	// Create a temp directory with a test file
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "hello.txt")
	os.WriteFile(testFile, []byte("hello from sandboxed file"), 0o644)

	wasmPath := filepath.Join(testdataDir(), "read_file_tool.wasm")

	input, _ := json.Marshal(map[string]any{
		"name":      "read_file",
		"arguments": map[string]any{"path": testFile},
	})

	caps := DefaultCapabilities()
	caps.AllowedPaths = []string{tmpDir} // grant access to tmpDir

	out, err := rt.Execute(ctx, ToolInvocation{
		Name: "read_file", Input: input, Module: wasmPath,
	}, caps)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}

	var result map[string]any
	if err := json.Unmarshal(out.Stdout, &result); err != nil {
		t.Fatalf("unmarshal: %v\nstdout: %s\nstderr: %s", err, out.Stdout, out.Stderr)
	}

	if result["content"] != "hello from sandboxed file" {
		t.Errorf("expected file content, got %q", result["content"])
	}
	if result["is_error"] == true {
		t.Errorf("unexpected error: %v", result["content"])
	}

	t.Logf("✅ WASM read allowed file: %s", result["content"])
}

func TestWASM_FileRead_Denied(t *testing.T) {
	ctx := context.Background()
	rt, err := NewWASMRuntime(ctx)
	if err != nil {
		t.Fatalf("NewWASMRuntime: %v", err)
	}
	defer rt.Close()

	// Create two directories — only grant access to one
	allowedDir := t.TempDir()
	deniedDir := t.TempDir()

	// Put the secret file in the denied directory
	secretFile := filepath.Join(deniedDir, "secret.txt")
	os.WriteFile(secretFile, []byte("TOP SECRET DATA"), 0o644)

	wasmPath := filepath.Join(testdataDir(), "read_file_tool.wasm")

	input, _ := json.Marshal(map[string]any{
		"name":      "read_file",
		"arguments": map[string]any{"path": secretFile},
	})

	caps := DefaultCapabilities()
	caps.AllowedPaths = []string{allowedDir} // only grant access to allowedDir, NOT deniedDir

	out, err := rt.Execute(ctx, ToolInvocation{
		Name: "read_file", Input: input, Module: wasmPath,
	}, caps)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}

	var result map[string]any
	if err := json.Unmarshal(out.Stdout, &result); err != nil {
		t.Fatalf("unmarshal: %v\nstdout: %s\nstderr: %s", err, out.Stdout, out.Stderr)
	}

	// The WASM module should NOT be able to read the secret file
	if result["is_error"] != true {
		t.Errorf("expected error when reading denied path, got content: %q", result["content"])
	} else {
		t.Logf("✅ WASM correctly denied access: %s", result["content"])
	}
}

func TestWASM_FileRead_NoCapabilities(t *testing.T) {
	ctx := context.Background()
	rt, err := NewWASMRuntime(ctx)
	if err != nil {
		t.Fatalf("NewWASMRuntime: %v", err)
	}
	defer rt.Close()

	// Create a file but don't grant any filesystem access
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "data.txt")
	os.WriteFile(testFile, []byte("should not be readable"), 0o644)

	wasmPath := filepath.Join(testdataDir(), "read_file_tool.wasm")

	input, _ := json.Marshal(map[string]any{
		"name":      "read_file",
		"arguments": map[string]any{"path": testFile},
	})

	// No AllowedPaths — zero filesystem access
	caps := DefaultCapabilities()

	out, err := rt.Execute(ctx, ToolInvocation{
		Name: "read_file", Input: input, Module: wasmPath,
	}, caps)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}

	var result map[string]any
	json.Unmarshal(out.Stdout, &result)

	if result["is_error"] != true {
		t.Errorf("expected error with no FS capabilities, got: %q", result["content"])
	} else {
		t.Logf("✅ WASM correctly denied (no FS caps): %s", result["content"])
	}
}

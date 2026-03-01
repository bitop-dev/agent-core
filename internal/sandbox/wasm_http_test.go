package sandbox

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"
)

func TestWASM_HTTPRequest_WebSearch(t *testing.T) {
	ctx := context.Background()
	rt, err := NewWASMRuntime(ctx)
	if err != nil {
		t.Fatalf("NewWASMRuntime: %v", err)
	}
	defer rt.Close()

	wasmPath := filepath.Join(testdataDir(), "web_search_tool.wasm")

	input, _ := json.Marshal(map[string]any{
		"name": "web_search",
		"arguments": map[string]any{
			"query":       "golang wazero wasm runtime",
			"max_results": 3,
		},
	})

	caps := DefaultCapabilities()
	caps.AllowedHosts = []string{"html.duckduckgo.com"} // grant DDG access
	caps.MaxTimeoutSec = 30

	out, err := rt.Execute(ctx, ToolInvocation{
		Name: "web_search", Input: input, Module: wasmPath,
	}, caps)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}

	t.Logf("Exit code: %d", out.ExitCode)
	t.Logf("Stderr: %s", out.Stderr)
	t.Logf("Stdout: %s", out.Stdout)

	if out.ExitCode != 0 {
		t.Fatalf("exit code %d, stderr: %s", out.ExitCode, out.Stderr)
	}

	var result map[string]any
	if err := json.Unmarshal(out.Stdout, &result); err != nil {
		t.Fatalf("unmarshal output: %v\nraw: %s", err, out.Stdout)
	}

	if result["is_error"] == true {
		t.Fatalf("tool returned error: %s", result["content"])
	}

	content, _ := result["content"].(string)
	if content == "" {
		t.Fatal("empty content")
	}

	t.Logf("✅ WASM web_search results:\n%s", content)
}

func TestWASM_HTTPRequest_HostDenied(t *testing.T) {
	ctx := context.Background()
	rt, err := NewWASMRuntime(ctx)
	if err != nil {
		t.Fatalf("NewWASMRuntime: %v", err)
	}
	defer rt.Close()

	wasmPath := filepath.Join(testdataDir(), "web_search_tool.wasm")

	input, _ := json.Marshal(map[string]any{
		"name": "web_search",
		"arguments": map[string]any{
			"query": "test query",
		},
	})

	caps := DefaultCapabilities()
	// NO AllowedHosts — network should be denied
	caps.AllowedHosts = nil
	caps.MaxTimeoutSec = 10

	out, err := rt.Execute(ctx, ToolInvocation{
		Name: "web_search", Input: input, Module: wasmPath,
	}, caps)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}

	var result map[string]any
	if err := json.Unmarshal(out.Stdout, &result); err != nil {
		t.Fatalf("unmarshal: %v\nstdout: %s\nstderr: %s", err, out.Stdout, out.Stderr)
	}

	// Should fail because no hosts are allowed
	if result["is_error"] != true {
		t.Errorf("expected error when no hosts allowed, got: %v", result["content"])
	} else {
		t.Logf("✅ Correctly denied: %s", result["content"])
	}
}

func TestWASM_HTTPRequest_WrongHostDenied(t *testing.T) {
	ctx := context.Background()
	rt, err := NewWASMRuntime(ctx)
	if err != nil {
		t.Fatalf("NewWASMRuntime: %v", err)
	}
	defer rt.Close()

	wasmPath := filepath.Join(testdataDir(), "web_search_tool.wasm")

	input, _ := json.Marshal(map[string]any{
		"name": "web_search",
		"arguments": map[string]any{
			"query": "test query",
		},
	})

	caps := DefaultCapabilities()
	// Allow google but NOT duckduckgo
	caps.AllowedHosts = []string{"www.google.com"}
	caps.MaxTimeoutSec = 10

	out, err := rt.Execute(ctx, ToolInvocation{
		Name: "web_search", Input: input, Module: wasmPath,
	}, caps)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}

	var result map[string]any
	json.Unmarshal(out.Stdout, &result)

	if result["is_error"] != true {
		t.Errorf("expected error when DDG host not allowed, got: %v", result["content"])
	} else {
		t.Logf("✅ Correctly denied wrong host: %s", result["content"])
	}
}

func TestWASM_HTTPRequest_WildcardHost(t *testing.T) {
	ctx := context.Background()
	rt, err := NewWASMRuntime(ctx)
	if err != nil {
		t.Fatalf("NewWASMRuntime: %v", err)
	}
	defer rt.Close()

	wasmPath := filepath.Join(testdataDir(), "web_search_tool.wasm")

	input, _ := json.Marshal(map[string]any{
		"name": "web_search",
		"arguments": map[string]any{
			"query":       "golang",
			"max_results": 2,
		},
	})

	caps := DefaultCapabilities()
	caps.AllowedHosts = []string{"*"} // wildcard = allow all
	caps.MaxTimeoutSec = 30

	out, err := rt.Execute(ctx, ToolInvocation{
		Name: "web_search", Input: input, Module: wasmPath,
	}, caps)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}

	var result map[string]any
	json.Unmarshal(out.Stdout, &result)

	if result["is_error"] == true {
		t.Errorf("wildcard should allow all hosts: %s", result["content"])
	} else {
		t.Logf("✅ Wildcard host works: %.100s...", result["content"])
	}
}

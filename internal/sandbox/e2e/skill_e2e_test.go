package e2e

import (
	"context"
	"encoding/json"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/bitop-dev/agent-core/internal/sandbox"
	"github.com/bitop-dev/agent-core/internal/skill"
	"github.com/bitop-dev/agent-core/internal/tool"
)

func testdataDir() string {
	_, file, _, _ := runtime.Caller(0)
	return filepath.Join(filepath.Dir(file), "..", "testdata")
}

// TestSkillE2E_LoadAndExecuteWASM tests the full flow:
//  1. Load a WASM skill from disk (SKILL.md + tools/*.json + tools/*.wasm)
//  2. Detect runtime as "wasm"
//  3. Register tool through sandbox system
//  4. Execute a web search through the sandboxed WASM module
//  5. Verify results
func TestSkillE2E_LoadAndExecuteWASM(t *testing.T) {
	ctx := context.Background()

	// ── Step 1: Load the skill ──────────────────────────────────────────
	skillDir := filepath.Join(testdataDir(), "skills", "web_search_wasm")
	loader := skill.NewLoader(filepath.Join(testdataDir(), "skills"))
	sk, err := loader.Load(skillDir)
	if err != nil {
		t.Fatalf("load skill: %v", err)
	}

	t.Logf("Loaded skill: %s (v%s) runtime=%s", sk.Name, sk.Version, sk.Runtime)
	t.Logf("  Description: %s", sk.Description)
	t.Logf("  Tools: %d", len(sk.Tools))

	if sk.Name != "web_search_wasm" {
		t.Errorf("expected name 'web_search_wasm', got %q", sk.Name)
	}

	// ── Step 2: Verify runtime detection ────────────────────────────────
	if sk.Runtime != "wasm" {
		t.Errorf("expected runtime 'wasm', got %q", sk.Runtime)
	}

	if len(sk.Tools) != 1 {
		t.Fatalf("expected 1 tool, got %d", len(sk.Tools))
	}

	// ── Step 3: Set up sandbox and register tool ────────────────────────
	wasmRT, err := sandbox.NewWASMRuntime(ctx)
	if err != nil {
		t.Fatalf("NewWASMRuntime: %v", err)
	}
	defer wasmRT.Close()

	reg := sandbox.NewRegistry()
	reg.Register(wasmRT)

	// Find the WASM executable
	td := sk.Tools[0]
	execPath, execType := skill.FindToolExec(sk.Dir, td.Name)
	if execPath == "" {
		t.Fatalf("tool executable not found for %q", td.Name)
	}
	t.Logf("  Tool exec: %s (type: %s)", execPath, execType)

	if execType != "wasm" {
		t.Errorf("expected execType 'wasm', got %q", execType)
	}

	// Create capabilities
	caps := sandbox.DefaultCapabilities()
	caps.AllowedHosts = []string{"html.duckduckgo.com"}
	caps.MaxTimeoutSec = 30

	// Create the sandboxed tool
	sandboxedTool := tool.NewSandboxedTool(tool.SandboxedToolConfig{
		Def: tool.Definition{
			Name:        td.Name,
			Description: td.Description,
			InputSchema: json.RawMessage(td.Parameters),
		},
		Runtime:  sandbox.RuntimeWASM,
		Module:   execPath,
		WorkDir:  ".",
		Caps:     caps,
		Registry: reg,
	})

	// Register in tool engine
	engine := tool.NewEngine()
	engine.Register(sandboxedTool)

	// Verify the tool is registered
	defs := engine.Definitions()
	if len(defs) != 1 {
		t.Fatalf("expected 1 definition, got %d", len(defs))
	}
	t.Logf("  Registered tool: %s", defs[0].Name)

	// ── Step 4: Execute through the tool engine ─────────────────────────
	results := engine.Dispatch(ctx, []tool.Call{
		{
			ID:        "call_1",
			Name:      "web_search_wasm",
			Arguments: json.RawMessage(`{"query":"Go programming language","max_results":3}`),
		},
	})

	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}

	result := results[0]
	if result.IsError {
		t.Fatalf("tool returned error: %s", result.Content)
	}

	t.Logf("\n✅ End-to-end WASM skill execution succeeded!\n")
	t.Logf("Search results:\n%s", result.Content)

	// ── Step 5: Verify results contain expected data ────────────────────
	if len(result.Content) < 50 {
		t.Errorf("result content too short: %d bytes", len(result.Content))
	}
}

// TestSkillE2E_RuntimeDetection verifies runtime auto-detection from file types.
func TestSkillE2E_RuntimeDetection(t *testing.T) {
	// The web_search_wasm skill should detect as "wasm"
	dir := filepath.Join(testdataDir(), "skills", "web_search_wasm")
	detected := skill.DetectRuntime(dir)
	if detected != "wasm" {
		t.Errorf("expected detected runtime 'wasm', got %q", detected)
	}
	t.Logf("✅ DetectRuntime(web_search_wasm) = %q", detected)

	// A directory with no tools/ should detect as ""
	detected = skill.DetectRuntime(t.TempDir())
	if detected != "" {
		t.Errorf("expected empty runtime for dir with no tools, got %q", detected)
	}
	t.Logf("✅ DetectRuntime(empty dir) = %q", detected)
}

// TestSkillE2E_SandboxDeniesNetwork verifies that WASM tools can't reach
// hosts that aren't in AllowedHosts.
func TestSkillE2E_SandboxDeniesNetwork(t *testing.T) {
	ctx := context.Background()

	skillDir := filepath.Join(testdataDir(), "skills", "web_search_wasm")
	loader := skill.NewLoader(filepath.Join(testdataDir(), "skills"))
	sk, err := loader.Load(skillDir)
	if err != nil {
		t.Fatalf("load skill: %v", err)
	}

	wasmRT, err := sandbox.NewWASMRuntime(ctx)
	if err != nil {
		t.Fatalf("NewWASMRuntime: %v", err)
	}
	defer wasmRT.Close()

	reg := sandbox.NewRegistry()
	reg.Register(wasmRT)

	td := sk.Tools[0]
	execPath, _ := skill.FindToolExec(sk.Dir, td.Name)

	// NO AllowedHosts — should deny all network access
	caps := sandbox.DefaultCapabilities()
	caps.AllowedHosts = nil
	caps.MaxTimeoutSec = 10

	sandboxedTool := tool.NewSandboxedTool(tool.SandboxedToolConfig{
		Def: tool.Definition{
			Name:        td.Name,
			Description: td.Description,
			InputSchema: json.RawMessage(td.Parameters),
		},
		Runtime:  sandbox.RuntimeWASM,
		Module:   execPath,
		WorkDir:  ".",
		Caps:     caps,
		Registry: reg,
	})

	engine := tool.NewEngine()
	engine.Register(sandboxedTool)

	results := engine.Dispatch(ctx, []tool.Call{
		{
			ID:        "call_1",
			Name:      "web_search_wasm",
			Arguments: json.RawMessage(`{"query":"test"}`),
		},
	})

	result := results[0]
	if !result.IsError {
		t.Errorf("expected error when no hosts allowed, got: %s", result.Content)
	} else {
		t.Logf("✅ Correctly denied network: %s", result.Content)
	}
}

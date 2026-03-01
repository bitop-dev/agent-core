package sandbox

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"time"

	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/imports/wasi_snapshot_preview1"
)

// WASMRuntime executes tool modules as WebAssembly via Wazero.
// Each invocation gets its own module instance with sandboxed capabilities.
type WASMRuntime struct {
	engine wazero.Runtime
}

// NewWASMRuntime creates a WASM runtime backed by Wazero.
// The runtime is safe for concurrent use.
func NewWASMRuntime(ctx context.Context) (*WASMRuntime, error) {
	cfg := wazero.NewRuntimeConfig().
		WithCloseOnContextDone(true)

	rt := wazero.NewRuntimeWithConfig(ctx, cfg)

	// Instantiate WASI — gives modules stdin/stdout/stderr, args, env, clock.
	if _, err := wasi_snapshot_preview1.Instantiate(ctx, rt); err != nil {
		rt.Close(ctx)
		return nil, fmt.Errorf("wasi init: %w", err)
	}

	return &WASMRuntime{engine: rt}, nil
}

func (w *WASMRuntime) Type() RuntimeType {
	return RuntimeWASM
}

func (w *WASMRuntime) Execute(ctx context.Context, inv ToolInvocation, caps Capabilities) (*ToolOutput, error) {
	// Apply timeout
	timeout := time.Duration(caps.MaxTimeoutSec) * time.Second
	if timeout <= 0 {
		timeout = 60 * time.Second
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	// Read the WASM module bytes
	wasmBytes, err := os.ReadFile(inv.Module)
	if err != nil {
		return nil, fmt.Errorf("read wasm module %q: %w", inv.Module, err)
	}

	// Compile the module
	compiled, err := w.engine.CompileModule(ctx, wasmBytes)
	if err != nil {
		return nil, fmt.Errorf("compile wasm: %w", err)
	}
	defer compiled.Close(ctx)

	// Build WASI configuration with sandboxed I/O
	var stdout, stderr bytes.Buffer
	stdin := bytes.NewReader(inv.Input)

	wasiConfig := wazero.NewModuleConfig().
		WithStdin(stdin).
		WithStdout(&stdout).
		WithStderr(&stderr).
		WithArgs(inv.Name). // argv[0] = tool name
		WithSysWalltime().
		WithSysNanotime().
		WithRandSource(nil) // deterministic by default

	// Pass environment variables
	for k, v := range caps.EnvVars {
		wasiConfig = wasiConfig.WithEnv(k, v)
	}

	// Mount allowed filesystem paths
	for _, p := range caps.AllowedPaths {
		if stat, err := os.Stat(p); err == nil && stat.IsDir() {
			wasiConfig = wasiConfig.WithFSConfig(
				wazero.NewFSConfig().WithDirMount(p, p),
			)
		}
	}
	for _, p := range caps.ReadOnlyPaths {
		if stat, err := os.Stat(p); err == nil && stat.IsDir() {
			wasiConfig = wasiConfig.WithFSConfig(
				wazero.NewFSConfig().WithReadOnlyDirMount(p, p),
			)
		}
	}

	// Instantiate and run the module
	mod, err := w.engine.InstantiateModule(ctx, compiled, wasiConfig)
	if err != nil {
		// If context was cancelled, it's a timeout
		if ctx.Err() != nil {
			return &ToolOutput{
				Stderr:   []byte(fmt.Sprintf("wasm execution timed out after %s", timeout)),
				ExitCode: 124,
			}, nil
		}
		// Wazero wraps exit codes in errors — extract if possible
		return &ToolOutput{
			Stdout:   stdout.Bytes(),
			Stderr:   append(stderr.Bytes(), []byte(fmt.Sprintf("\nwasm error: %v", err))...),
			ExitCode: 1,
		}, nil
	}
	defer mod.Close(ctx)

	// Truncate output if over limit
	out := stdout.Bytes()
	if caps.MaxOutputBytes > 0 && len(out) > caps.MaxOutputBytes {
		out = out[:caps.MaxOutputBytes]
	}

	return &ToolOutput{
		Stdout:   out,
		Stderr:   stderr.Bytes(),
		ExitCode: 0,
	}, nil
}

func (w *WASMRuntime) Close() error {
	return w.engine.Close(context.Background())
}

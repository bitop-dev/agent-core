// Package sandbox defines the runtime execution abstraction for tools.
//
// Tools can run in different sandboxed environments:
//   - native: direct Go execution (built-in tools, no sandbox)
//   - wasm: WebAssembly modules via Wazero (lightweight, capability-based)
//   - container: OCI containers via Docker/Podman (full isolation)
//   - native: built-in Go tools (compiled into the binary)
//
// The Runtime interface abstracts these so tools declare what they need
// and the engine dispatches to the right executor.
package sandbox

import (
	"context"
	"fmt"
)

// RuntimeType identifies which sandbox backend to use.
type RuntimeType string

const (
	RuntimeNative    RuntimeType = "native"    // compiled-in Go code
	RuntimeWASM      RuntimeType = "wasm"      // WebAssembly via Wazero
	RuntimeContainer RuntimeType = "container" // Docker/Podman OCI container
)

// Capabilities defines what a sandboxed runtime is allowed to access.
type Capabilities struct {
	// AllowedPaths is the list of host directories the tool can read/write.
	AllowedPaths []string

	// ReadOnlyPaths is the list of host directories mounted read-only.
	ReadOnlyPaths []string

	// AllowedHosts is the list of network hosts the tool can reach.
	// Empty means no network access. ["*"] means unrestricted.
	AllowedHosts []string

	// EnvVars is the map of environment variables passed to the runtime.
	EnvVars map[string]string

	// MaxMemoryMB caps memory usage (WASM). 0 = default (256MB).
	MaxMemoryMB int

	// MaxTimeoutSec caps execution time. 0 = default (60s).
	MaxTimeoutSec int

	// MaxOutputBytes caps stdout/result size. 0 = default (1MB).
	MaxOutputBytes int
}

// DefaultCapabilities returns a restrictive default.
func DefaultCapabilities() Capabilities {
	return Capabilities{
		MaxMemoryMB:    256,
		MaxTimeoutSec:  60,
		MaxOutputBytes: 1 << 20, // 1MB
	}
}

// ToolInvocation is the input to a sandboxed tool execution.
type ToolInvocation struct {
	// Name of the tool being called.
	Name string

	// Input is the JSON arguments from the LLM.
	Input []byte

	// Module is the path to the WASM module or container image.
	// For native tools, this may be empty. For WASM, it's the .wasm file path.
	Module string

	// WorkDir is the working directory for the execution.
	WorkDir string
}

// ToolOutput is the result of a sandboxed tool execution.
type ToolOutput struct {
	// Stdout is the raw standard output.
	Stdout []byte

	// Stderr is the raw standard error (for debugging).
	Stderr []byte

	// ExitCode is the process exit code (0 = success).
	ExitCode int
}

// Runtime executes tool invocations in a sandboxed environment.
type Runtime interface {
	// Type returns the runtime type identifier.
	Type() RuntimeType

	// Execute runs a tool invocation with the given capabilities.
	Execute(ctx context.Context, inv ToolInvocation, caps Capabilities) (*ToolOutput, error)

	// Close releases any resources held by the runtime.
	Close() error
}

// Registry holds available runtimes and dispatches to the right one.
type Registry struct {
	runtimes map[RuntimeType]Runtime
}

// NewRegistry creates an empty runtime registry.
func NewRegistry() *Registry {
	return &Registry{runtimes: make(map[RuntimeType]Runtime)}
}

// Register adds a runtime backend.
func (r *Registry) Register(rt Runtime) {
	r.runtimes[rt.Type()] = rt
}

// Get returns a runtime by type.
func (r *Registry) Get(t RuntimeType) (Runtime, error) {
	rt, ok := r.runtimes[t]
	if !ok {
		return nil, fmt.Errorf("runtime %q not registered", t)
	}
	return rt, nil
}

// Execute dispatches to the appropriate runtime.
func (r *Registry) Execute(ctx context.Context, rt RuntimeType, inv ToolInvocation, caps Capabilities) (*ToolOutput, error) {
	runtime, err := r.Get(rt)
	if err != nil {
		return nil, err
	}
	return runtime.Execute(ctx, inv, caps)
}

// Close shuts down all registered runtimes.
func (r *Registry) Close() error {
	var last error
	for _, rt := range r.runtimes {
		if err := rt.Close(); err != nil {
			last = err
		}
	}
	return last
}

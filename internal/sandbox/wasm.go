package sandbox

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/api"
	"github.com/tetratelabs/wazero/imports/wasi_snapshot_preview1"
)

// WASMRuntime executes tool modules as WebAssembly via Wazero.
// Each invocation gets its own module instance with sandboxed capabilities.
// Host functions are injected for HTTP access, gated by AllowedHosts.
// Compiled modules are cached by content hash for fast repeat invocations.
type WASMRuntime struct {
	engine wazero.Runtime

	cacheMu sync.RWMutex
	cache   map[string]wazero.CompiledModule // content hash → compiled module
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

	w := &WASMRuntime{
		engine: rt,
		cache:  make(map[string]wazero.CompiledModule),
	}

	// Register host functions once at init (not per-execution).
	if err := w.registerHostFunctions(ctx); err != nil {
		rt.Close(ctx)
		return nil, fmt.Errorf("register host functions: %w", err)
	}

	return w, nil
}

func (w *WASMRuntime) Type() RuntimeType {
	return RuntimeWASM
}

// capsKey is the context key for passing capabilities to host functions.
type capsKey struct{}

func (w *WASMRuntime) Execute(ctx context.Context, inv ToolInvocation, caps Capabilities) (*ToolOutput, error) {
	// Apply timeout
	timeout := time.Duration(caps.MaxTimeoutSec) * time.Second
	if timeout <= 0 {
		timeout = 60 * time.Second
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	// Store capabilities in context for host functions
	ctx = context.WithValue(ctx, capsKey{}, &caps)

	// Get or compile the module (cached by content hash)
	compiled, err := w.getOrCompile(ctx, inv.Module)
	if err != nil {
		return nil, err
	}
	// Don't close compiled — it's cached for reuse

	// Build WASI configuration with sandboxed I/O
	var stdout, stderr bytes.Buffer
	stdin := bytes.NewReader(inv.Input)

	modConfig := wazero.NewModuleConfig().
		WithStdin(stdin).
		WithStdout(&stdout).
		WithStderr(&stderr).
		WithArgs(inv.Name). // argv[0] = tool name
		WithSysWalltime().
		WithSysNanotime().
		WithStartFunctions("_start").
		WithName("")

	// Pass environment variables
	for k, v := range caps.EnvVars {
		modConfig = modConfig.WithEnv(k, v)
	}

	// Mount allowed filesystem paths
	fsConfig := wazero.NewFSConfig()
	hasMounts := false
	for _, p := range caps.AllowedPaths {
		if stat, err := os.Stat(p); err == nil && stat.IsDir() {
			fsConfig = fsConfig.WithDirMount(p, p)
			hasMounts = true
		}
	}
	for _, p := range caps.ReadOnlyPaths {
		if stat, err := os.Stat(p); err == nil && stat.IsDir() {
			fsConfig = fsConfig.WithReadOnlyDirMount(p, p)
			hasMounts = true
		}
	}
	if hasMounts {
		modConfig = modConfig.WithFSConfig(fsConfig)
	}

	// Instantiate and run the module
	mod, err := w.engine.InstantiateModule(ctx, compiled, modConfig)
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

// registerHostFunctions creates the "agent_host" module with HTTP host functions.
// WASM tools import these to make network requests, gated by AllowedHosts.
//
// Exported functions:
//   - http_request(method_ptr, method_len, url_ptr, url_len, body_ptr, body_len,
//     resp_buf_ptr, resp_buf_len) -> bytes_written i32
//     Returns -1 on error (denied host, network failure, etc.)
//     Returns -2 if response exceeds buffer size.
func (w *WASMRuntime) registerHostFunctions(ctx context.Context) error {
	_, err := w.engine.NewHostModuleBuilder("agent_host").
		NewFunctionBuilder().
		WithGoModuleFunction(api.GoModuleFunc(httpRequestFn), 
			[]api.ValueType{
				api.ValueTypeI32, api.ValueTypeI32, // method_ptr, method_len
				api.ValueTypeI32, api.ValueTypeI32, // url_ptr, url_len
				api.ValueTypeI32, api.ValueTypeI32, // body_ptr, body_len
				api.ValueTypeI32, api.ValueTypeI32, // resp_buf_ptr, resp_buf_len
			},
			[]api.ValueType{api.ValueTypeI32}).    // -> bytes_written
		WithParameterNames("method_ptr", "method_len", "url_ptr", "url_len",
			"body_ptr", "body_len", "resp_buf_ptr", "resp_buf_len").
		Export("http_request").
		Instantiate(ctx)
	return err
}

// httpRequestFn is the host implementation of agent_host.http_request.
func httpRequestFn(ctx context.Context, mod api.Module, stack []uint64) {
	methodPtr := uint32(stack[0])
	methodLen := uint32(stack[1])
	urlPtr := uint32(stack[2])
	urlLen := uint32(stack[3])
	bodyPtr := uint32(stack[4])
	bodyLen := uint32(stack[5])
	respBufPtr := uint32(stack[6])
	respBufLen := uint32(stack[7])

	mem := mod.Memory()

	// Read method from guest memory
	methodBytes, ok := mem.Read(methodPtr, methodLen)
	if !ok {
		stack[0] = uint64(encodeI32(-1))
		return
	}
	method := string(methodBytes)

	// Read URL from guest memory
	urlBytes, ok := mem.Read(urlPtr, urlLen)
	if !ok {
		stack[0] = uint64(encodeI32(-1))
		return
	}
	rawURL := string(urlBytes)

	// Enforce AllowedHosts from capabilities
	caps, _ := ctx.Value(capsKey{}).(*Capabilities)
	if caps != nil && !isHostAllowed(rawURL, caps.AllowedHosts) {
		// Write error message to response buffer
		errMsg := fmt.Sprintf(`{"error":"host not allowed","url":"%s"}`, rawURL)
		if uint32(len(errMsg)) <= respBufLen {
			mem.Write(respBufPtr, []byte(errMsg))
		}
		stack[0] = uint64(encodeI32(-3)) // -3 = host denied
		return
	}

	// Read body from guest memory (may be empty)
	var bodyReader io.Reader
	if bodyLen > 0 {
		bodyBytes, ok := mem.Read(bodyPtr, bodyLen)
		if !ok {
			stack[0] = uint64(encodeI32(-1))
			return
		}
		bodyReader = bytes.NewReader(bodyBytes)
	}

	// Make the HTTP request
	client := &http.Client{Timeout: 30 * time.Second}
	req, err := http.NewRequestWithContext(ctx, method, rawURL, bodyReader)
	if err != nil {
		stack[0] = uint64(encodeI32(-1))
		return
	}
	req.Header.Set("User-Agent", "agent-core/1.0")

	resp, err := client.Do(req)
	if err != nil {
		// Write error to response buffer so guest can see what went wrong
		errMsg := fmt.Sprintf(`{"error":"%s"}`, strings.ReplaceAll(err.Error(), `"`, `\"`))
		if uint32(len(errMsg)) <= respBufLen {
			mem.Write(respBufPtr, []byte(errMsg))
			stack[0] = uint64(encodeI32(int32(len(errMsg))))
		} else {
			stack[0] = uint64(encodeI32(-1))
		}
		return
	}
	defer resp.Body.Close()

	// Read response body (capped at buffer size)
	limited := io.LimitReader(resp.Body, int64(respBufLen))
	respBody, err := io.ReadAll(limited)
	if err != nil {
		stack[0] = uint64(encodeI32(-1))
		return
	}

	// Write response to guest memory
	if !mem.Write(respBufPtr, respBody) {
		stack[0] = uint64(encodeI32(-1))
		return
	}

	stack[0] = uint64(encodeI32(int32(len(respBody))))
}

// isHostAllowed checks if a URL's host is in the allowed list.
func isHostAllowed(rawURL string, allowedHosts []string) bool {
	if len(allowedHosts) == 0 {
		return false // no hosts allowed = no network
	}
	for _, h := range allowedHosts {
		if h == "*" {
			return true // wildcard = allow all
		}
	}

	parsed, err := url.Parse(rawURL)
	if err != nil {
		return false
	}
	host := parsed.Hostname()

	for _, allowed := range allowedHosts {
		if host == allowed {
			return true
		}
		// Support wildcard subdomains: *.example.com
		if strings.HasPrefix(allowed, "*.") {
			domain := allowed[2:]
			if host == domain || strings.HasSuffix(host, "."+domain) {
				return true
			}
		}
	}
	return false
}

// encodeI32 ensures proper sign extension for negative i32 values in uint64.
func encodeI32(v int32) uint32 {
	return uint32(v)
}

// getOrCompile returns a cached compiled module or compiles and caches a new one.
// Modules are cached by SHA-256 hash of their contents, so updated .wasm files
// are automatically recompiled.
func (w *WASMRuntime) getOrCompile(ctx context.Context, modulePath string) (wazero.CompiledModule, error) {
	wasmBytes, err := os.ReadFile(modulePath)
	if err != nil {
		return nil, fmt.Errorf("read wasm module %q: %w", modulePath, err)
	}

	// Hash the module contents
	hash := sha256.Sum256(wasmBytes)
	key := hex.EncodeToString(hash[:])

	// Check cache
	w.cacheMu.RLock()
	if compiled, ok := w.cache[key]; ok {
		w.cacheMu.RUnlock()
		return compiled, nil
	}
	w.cacheMu.RUnlock()

	// Compile and cache
	compiled, err := w.engine.CompileModule(ctx, wasmBytes)
	if err != nil {
		return nil, fmt.Errorf("compile wasm: %w", err)
	}

	w.cacheMu.Lock()
	// Double-check in case another goroutine compiled while we were compiling
	if existing, ok := w.cache[key]; ok {
		w.cacheMu.Unlock()
		compiled.Close(ctx) // discard our duplicate
		return existing, nil
	}
	w.cache[key] = compiled
	w.cacheMu.Unlock()

	return compiled, nil
}

// CacheSize returns the number of compiled modules in the cache.
func (w *WASMRuntime) CacheSize() int {
	w.cacheMu.RLock()
	defer w.cacheMu.RUnlock()
	return len(w.cache)
}

func (w *WASMRuntime) Close() error {
	// Close all cached compiled modules
	ctx := context.Background()
	w.cacheMu.Lock()
	for _, compiled := range w.cache {
		compiled.Close(ctx)
	}
	w.cache = nil
	w.cacheMu.Unlock()

	return w.engine.Close(ctx)
}

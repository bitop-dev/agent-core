// Package tool defines the Tool interface, ToolEngine for dispatch,
// and the sandboxed runner for external tools.
package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
)

// Tool is implemented by all tool types: core built-ins, WASM skill tools, and MCP tools.
type Tool interface {
	Definition() Definition
	Execute(ctx context.Context, input json.RawMessage) (Result, error)
}

// Definition describes a tool for the LLM.
type Definition struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	InputSchema json.RawMessage `json:"parameters"`
}

// Result is what a tool returns after execution.
type Result struct {
	Content string `json:"content"`
	IsError bool   `json:"is_error"`
}

// Call represents a single tool invocation from the LLM.
type Call struct {
	ID        string
	Name      string
	Arguments json.RawMessage
}

// Engine manages tool registration, allowlisting, and parallel dispatch.
type Engine struct {
	mu      sync.RWMutex
	tools   map[string]Tool
	allowed map[string]bool // nil = all allowed
	Sandbox SandboxPolicy
}

// NewEngine creates an empty ToolEngine with default sandbox policy.
func NewEngine() *Engine {
	return &Engine{
		tools:   make(map[string]Tool),
		Sandbox: DefaultSandboxPolicy(),
	}
}

// Register adds a tool to the engine.
func (e *Engine) Register(t Tool) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.tools[t.Definition().Name] = t
}

// SetAllowed restricts which tools can be dispatched.
// Pass nil to allow all registered tools.
func (e *Engine) SetAllowed(names []string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if names == nil {
		e.allowed = nil
		return
	}
	e.allowed = make(map[string]bool, len(names))
	for _, n := range names {
		e.allowed[n] = true
	}
}

// Definitions returns all registered tool definitions (respecting allowlist).
func (e *Engine) Definitions() []Definition {
	e.mu.RLock()
	defer e.mu.RUnlock()
	var defs []Definition
	for name, t := range e.tools {
		if e.allowed != nil && !e.allowed[name] {
			continue
		}
		defs = append(defs, t.Definition())
	}
	return defs
}

// Dispatch executes one or more tool calls in parallel and returns results
// in the same order as the input calls.
func (e *Engine) Dispatch(ctx context.Context, calls []Call) []Result {
	results := make([]Result, len(calls))
	var wg sync.WaitGroup
	for i, call := range calls {
		wg.Add(1)
		go func(idx int, c Call) {
			defer wg.Done()
			results[idx] = e.execute(ctx, c)
		}(i, call)
	}
	wg.Wait()
	return results
}

func (e *Engine) execute(ctx context.Context, c Call) Result {
	e.mu.RLock()
	t, ok := e.tools[c.Name]
	allowed := e.allowed == nil || e.allowed[c.Name]
	e.mu.RUnlock()

	if !ok {
		return Result{Content: fmt.Sprintf("unknown tool: %s", c.Name), IsError: true}
	}
	if !allowed {
		return Result{Content: fmt.Sprintf("tool not allowed: %s", c.Name), IsError: true}
	}

	result, err := t.Execute(ctx, c.Arguments)
	if err != nil {
		return Result{Content: fmt.Sprintf("tool error: %v", err), IsError: true}
	}

	// Apply sandbox output truncation
	if truncated, wasTruncated := e.Sandbox.TruncateOutput(result.Content); wasTruncated {
		result.Content = truncated
	}

	return result
}

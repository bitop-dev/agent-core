// Package builtin provides the 7 core tools compiled into agent-core.
package builtin

import "github.com/bitop-dev/agent-core/internal/tool"

// All returns all built-in tools.
func All() []tool.Tool {
	return []tool.Tool{
		NewBash(),
		NewReadFile(),
		NewWriteFile(),
		NewEditFile(),
		NewListDir(),
		NewGrep(),
		NewHTTPFetch(),
	}
}

// ByName returns built-in tools as a map for selective registration.
func ByName() map[string]tool.Tool {
	m := make(map[string]tool.Tool)
	for _, t := range All() {
		m[t.Definition().Name] = t
	}
	return m
}

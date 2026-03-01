// Package builtin provides the 8 core tools compiled into agent-core.
package builtin

import "github.com/bitop-dev/agent-core/internal/tool"

// All returns all built-in tools (tasks uses a default store).
func All() []tool.Tool {
	return AllWithTaskStore(NewTaskStore())
}

// AllWithTaskStore returns all built-in tools using the given task store.
// This allows the CLI to share the task store with session persistence.
func AllWithTaskStore(ts *TaskStore) []tool.Tool {
	return []tool.Tool{
		NewBash(),
		NewReadFile(),
		NewWriteFile(),
		NewEditFile(),
		NewListDir(),
		NewGrep(),
		NewHTTPFetch(),
		NewTasks(ts),
	}
}

// ByName returns built-in tools as a map for selective registration.
func ByName() map[string]tool.Tool {
	return ByNameWithTaskStore(NewTaskStore())
}

// ByNameWithTaskStore returns built-in tools as a map using the given task store.
func ByNameWithTaskStore(ts *TaskStore) map[string]tool.Tool {
	m := make(map[string]tool.Tool)
	for _, t := range AllWithTaskStore(ts) {
		m[t.Definition().Name] = t
	}
	return m
}

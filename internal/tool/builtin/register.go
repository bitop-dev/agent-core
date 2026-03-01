// Package builtin provides the 8 core tools compiled into agent-core.
package builtin

import "github.com/bitop-dev/agent-core/internal/tool"

// BuiltinOptions configures the built-in tool set.
type BuiltinOptions struct {
	TaskStore  *TaskStore
	Sandbox    *tool.SandboxPolicy
	WorkingDir string // for bash tool
}

// All returns all built-in tools with default settings.
func All() []tool.Tool {
	return AllWithOptions(BuiltinOptions{TaskStore: NewTaskStore()})
}

// AllWithTaskStore returns all built-in tools using the given task store.
// Kept for backward compatibility.
func AllWithTaskStore(ts *TaskStore) []tool.Tool {
	return AllWithOptions(BuiltinOptions{TaskStore: ts})
}

// AllWithOptions returns all built-in tools with full configuration.
func AllWithOptions(opts BuiltinOptions) []tool.Tool {
	if opts.TaskStore == nil {
		opts.TaskStore = NewTaskStore()
	}

	var bash tool.Tool
	if opts.Sandbox != nil {
		bash = NewBashWithSandbox(opts.Sandbox, opts.WorkingDir)
	} else {
		bash = NewBash()
	}

	var readFile, writeFile, editFile tool.Tool
	if opts.Sandbox != nil {
		readFile = NewReadFileWithSandbox(opts.Sandbox)
		writeFile = NewWriteFileWithSandbox(opts.Sandbox)
		editFile = NewEditFileWithSandbox(opts.Sandbox)
	} else {
		readFile = NewReadFile()
		writeFile = NewWriteFile()
		editFile = NewEditFile()
	}

	return []tool.Tool{
		bash,
		readFile,
		writeFile,
		editFile,
		NewListDir(),
		NewGrep(),
		NewHTTPFetch(),
		NewTasks(opts.TaskStore),
		newAgentSpawn(),
	}
}

// ByName returns built-in tools as a map for selective registration.
func ByName() map[string]tool.Tool {
	return ByNameWithOptions(BuiltinOptions{TaskStore: NewTaskStore()})
}

// ByNameWithTaskStore returns built-in tools as a map using the given task store.
// Kept for backward compatibility.
func ByNameWithTaskStore(ts *TaskStore) map[string]tool.Tool {
	return ByNameWithOptions(BuiltinOptions{TaskStore: ts})
}

// ByNameWithOptions returns built-in tools as a map with full configuration.
func ByNameWithOptions(opts BuiltinOptions) map[string]tool.Tool {
	m := make(map[string]tool.Tool)
	for _, t := range AllWithOptions(opts) {
		m[t.Definition().Name] = t
	}
	return m
}

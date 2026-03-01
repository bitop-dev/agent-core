package builtin

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"

	"github.com/bitop-dev/agent-core/internal/tool"
)

// TaskStatus represents the state of a task.
type TaskStatus string

const (
	TaskPending    TaskStatus = "pending"
	TaskInProgress TaskStatus = "in_progress"
	TaskCompleted  TaskStatus = "completed"
)

// TaskItem is a single task in the checklist.
type TaskItem struct {
	ID     int        `json:"id"`
	Title  string     `json:"title"`
	Status TaskStatus `json:"status"`
}

// TaskStore holds the session-scoped task list.
type TaskStore struct {
	mu     sync.RWMutex
	items  []TaskItem
	nextID int
}

// NewTaskStore creates an empty task store.
func NewTaskStore() *TaskStore {
	return &TaskStore{nextID: 1}
}

// Snapshot returns a copy of all tasks (for session persistence).
func (ts *TaskStore) Snapshot() []TaskItem {
	ts.mu.RLock()
	defer ts.mu.RUnlock()
	out := make([]TaskItem, len(ts.items))
	copy(out, ts.items)
	return out
}

// Restore loads tasks from a snapshot (for session resume).
func (ts *TaskStore) Restore(items []TaskItem) {
	ts.mu.Lock()
	defer ts.mu.Unlock()
	ts.items = make([]TaskItem, len(items))
	copy(ts.items, items)
	ts.nextID = 1
	for _, item := range ts.items {
		if item.ID >= ts.nextID {
			ts.nextID = item.ID + 1
		}
	}
}

type tasksArgs struct {
	Action string          `json:"action"`
	Tasks  json.RawMessage `json:"tasks,omitempty"`
	Title  string          `json:"title,omitempty"`
	ID     int             `json:"id,omitempty"`
	Status string          `json:"status,omitempty"`
}

type taskEntry struct {
	Title  string `json:"title"`
	Status string `json:"status,omitempty"`
}

type tasksTool struct {
	store *TaskStore
}

// NewTasks creates the tasks tool backed by the given store.
func NewTasks(store *TaskStore) tool.Tool {
	return &tasksTool{store: store}
}

func (t *tasksTool) Definition() tool.Definition {
	return tool.Definition{
		Name: "tasks",
		Description: "Manage a task checklist for the current session. Use to break complex work into steps and track progress. " +
			"Actions: create (batch replace), add (single), update (change status), list (view all), delete (clear all).",
		InputSchema: json.RawMessage(`{
			"type": "object",
			"properties": {
				"action": {
					"type": "string",
					"enum": ["create", "add", "update", "list", "delete"],
					"description": "Operation to perform"
				},
				"tasks": {
					"type": "array",
					"items": {
						"type": "object",
						"properties": {
							"title": { "type": "string" },
							"status": { "type": "string", "enum": ["pending", "in_progress", "completed"] }
						},
						"required": ["title"]
					},
					"description": "For 'create': list of tasks (replaces existing list)"
				},
				"title": {
					"type": "string",
					"description": "For 'add': title of the new task"
				},
				"id": {
					"type": "integer",
					"description": "For 'update': ID of the task to update"
				},
				"status": {
					"type": "string",
					"enum": ["pending", "in_progress", "completed"],
					"description": "For 'update': new status"
				}
			},
			"required": ["action"]
		}`),
	}
}

func (t *tasksTool) Execute(ctx context.Context, input json.RawMessage) (tool.Result, error) {
	var args tasksArgs
	if err := json.Unmarshal(input, &args); err != nil {
		return tool.Result{Content: fmt.Sprintf("invalid arguments: %v", err), IsError: true}, nil
	}

	switch args.Action {
	case "create":
		return t.handleCreate(args.Tasks), nil
	case "add":
		return t.handleAdd(args.Title), nil
	case "update":
		return t.handleUpdate(args.ID, args.Status), nil
	case "list":
		return t.handleList(), nil
	case "delete":
		return t.handleDelete(), nil
	default:
		return tool.Result{
			Content: fmt.Sprintf("unknown action %q — valid: create, add, update, list, delete", args.Action),
			IsError: true,
		}, nil
	}
}

func (t *tasksTool) handleCreate(tasksJSON json.RawMessage) tool.Result {
	var entries []taskEntry
	if err := json.Unmarshal(tasksJSON, &entries); err != nil || len(entries) == 0 {
		return tool.Result{Content: "'tasks' must be a non-empty array of {title, status?}", IsError: true}
	}

	t.store.mu.Lock()
	defer t.store.mu.Unlock()

	t.store.items = nil
	t.store.nextID = 1

	for _, e := range entries {
		if e.Title == "" {
			return tool.Result{Content: "each task must have a non-empty 'title'", IsError: true}
		}
		status := TaskPending
		if e.Status != "" {
			s, ok := parseStatus(e.Status)
			if !ok {
				return tool.Result{Content: fmt.Sprintf("invalid status %q", e.Status), IsError: true}
			}
			status = s
		}
		t.store.items = append(t.store.items, TaskItem{
			ID:     t.store.nextID,
			Title:  e.Title,
			Status: status,
		})
		t.store.nextID++
	}

	return tool.Result{Content: fmt.Sprintf("Created %d task(s).", len(entries))}
}

func (t *tasksTool) handleAdd(title string) tool.Result {
	if title == "" {
		return tool.Result{Content: "'title' is required for add", IsError: true}
	}

	t.store.mu.Lock()
	defer t.store.mu.Unlock()

	id := t.store.nextID
	t.store.nextID++
	t.store.items = append(t.store.items, TaskItem{
		ID:     id,
		Title:  title,
		Status: TaskPending,
	})

	return tool.Result{Content: fmt.Sprintf("Added task [%d] %q.", id, title)}
}

func (t *tasksTool) handleUpdate(id int, statusStr string) tool.Result {
	if id <= 0 {
		return tool.Result{Content: "'id' is required for update", IsError: true}
	}
	if statusStr == "" {
		return tool.Result{Content: "'status' is required for update", IsError: true}
	}
	status, ok := parseStatus(statusStr)
	if !ok {
		return tool.Result{Content: fmt.Sprintf("invalid status %q — valid: pending, in_progress, completed", statusStr), IsError: true}
	}

	t.store.mu.Lock()
	defer t.store.mu.Unlock()

	for i := range t.store.items {
		if t.store.items[i].ID == id {
			t.store.items[i].Status = status
			return tool.Result{Content: fmt.Sprintf("Task [%d] updated to %s.", id, status)}
		}
	}

	return tool.Result{Content: fmt.Sprintf("task with id %d not found", id), IsError: true}
}

func (t *tasksTool) handleList() tool.Result {
	t.store.mu.RLock()
	defer t.store.mu.RUnlock()

	if len(t.store.items) == 0 {
		return tool.Result{Content: "No tasks."}
	}

	completed := 0
	for _, item := range t.store.items {
		if item.Status == TaskCompleted {
			completed++
		}
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "Tasks (%d/%d completed):\n", completed, len(t.store.items))
	for _, item := range t.store.items {
		marker := "○"
		switch item.Status {
		case TaskInProgress:
			marker = "◐"
		case TaskCompleted:
			marker = "●"
		}
		fmt.Fprintf(&sb, "  %s [%d] %s\n", marker, item.ID, item.Title)
	}

	return tool.Result{Content: sb.String()}
}

func (t *tasksTool) handleDelete() tool.Result {
	t.store.mu.Lock()
	defer t.store.mu.Unlock()

	t.store.items = nil
	t.store.nextID = 1

	return tool.Result{Content: "Task list cleared."}
}

func parseStatus(s string) (TaskStatus, bool) {
	switch s {
	case "pending":
		return TaskPending, true
	case "in_progress":
		return TaskInProgress, true
	case "completed":
		return TaskCompleted, true
	default:
		return "", false
	}
}

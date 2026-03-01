package builtin

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/bitop-dev/agent-core/internal/tool"
)

func execTasks(t *testing.T, tt tool.Tool, args string) tool.Result {
	t.Helper()
	r, err := tt.Execute(context.Background(), json.RawMessage(args))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	return r
}

func TestTasks_CreateAndList(t *testing.T) {
	store := NewTaskStore()
	tt := NewTasks(store)

	r := execTasks(t, tt, `{"action":"create","tasks":[{"title":"step one"},{"title":"step two"},{"title":"step three","status":"completed"}]}`)
	if r.IsError {
		t.Fatalf("create failed: %s", r.Content)
	}
	if !strings.Contains(r.Content, "3 task(s)") {
		t.Errorf("expected 3 tasks, got: %s", r.Content)
	}

	r = execTasks(t, tt, `{"action":"list"}`)
	if r.IsError {
		t.Fatalf("list failed: %s", r.Content)
	}
	if !strings.Contains(r.Content, "1/3 completed") {
		t.Errorf("expected 1/3 completed, got: %s", r.Content)
	}
	if !strings.Contains(r.Content, "[1] step one") {
		t.Errorf("expected task 1, got: %s", r.Content)
	}
	if !strings.Contains(r.Content, "[3] step three") {
		t.Errorf("expected task 3, got: %s", r.Content)
	}
}

func TestTasks_Add(t *testing.T) {
	store := NewTaskStore()
	tt := NewTasks(store)

	execTasks(t, tt, `{"action":"create","tasks":[{"title":"first"}]}`)

	r := execTasks(t, tt, `{"action":"add","title":"second"}`)
	if r.IsError {
		t.Fatalf("add failed: %s", r.Content)
	}
	if !strings.Contains(r.Content, "[2]") {
		t.Errorf("expected id 2, got: %s", r.Content)
	}

	r = execTasks(t, tt, `{"action":"list"}`)
	if !strings.Contains(r.Content, "first") || !strings.Contains(r.Content, "second") {
		t.Errorf("expected both tasks, got: %s", r.Content)
	}
}

func TestTasks_Update(t *testing.T) {
	store := NewTaskStore()
	tt := NewTasks(store)

	execTasks(t, tt, `{"action":"create","tasks":[{"title":"do thing"}]}`)

	r := execTasks(t, tt, `{"action":"update","id":1,"status":"in_progress"}`)
	if r.IsError {
		t.Fatalf("update failed: %s", r.Content)
	}
	if !strings.Contains(r.Content, "in_progress") {
		t.Errorf("expected in_progress, got: %s", r.Content)
	}

	r = execTasks(t, tt, `{"action":"list"}`)
	if !strings.Contains(r.Content, "◐") {
		t.Errorf("expected in_progress marker, got: %s", r.Content)
	}
}

func TestTasks_UpdateNonexistent(t *testing.T) {
	store := NewTaskStore()
	tt := NewTasks(store)

	r := execTasks(t, tt, `{"action":"update","id":999,"status":"completed"}`)
	if !r.IsError {
		t.Error("expected error for nonexistent task")
	}
}

func TestTasks_Delete(t *testing.T) {
	store := NewTaskStore()
	tt := NewTasks(store)

	execTasks(t, tt, `{"action":"create","tasks":[{"title":"will be deleted"}]}`)

	r := execTasks(t, tt, `{"action":"delete"}`)
	if r.IsError {
		t.Fatalf("delete failed: %s", r.Content)
	}

	r = execTasks(t, tt, `{"action":"list"}`)
	if !strings.Contains(r.Content, "No tasks") {
		t.Errorf("expected empty, got: %s", r.Content)
	}
}

func TestTasks_CreateReplaces(t *testing.T) {
	store := NewTaskStore()
	tt := NewTasks(store)

	execTasks(t, tt, `{"action":"create","tasks":[{"title":"old"}]}`)
	execTasks(t, tt, `{"action":"create","tasks":[{"title":"new"}]}`)

	r := execTasks(t, tt, `{"action":"list"}`)
	if strings.Contains(r.Content, "old") {
		t.Error("old task should be replaced")
	}
	if !strings.Contains(r.Content, "new") {
		t.Error("new task should be present")
	}
}

func TestTasks_EmptyCreateFails(t *testing.T) {
	store := NewTaskStore()
	tt := NewTasks(store)

	r := execTasks(t, tt, `{"action":"create","tasks":[]}`)
	if !r.IsError {
		t.Error("expected error for empty tasks")
	}
}

func TestTasks_AddEmptyTitleFails(t *testing.T) {
	store := NewTaskStore()
	tt := NewTasks(store)

	r := execTasks(t, tt, `{"action":"add","title":""}`)
	if !r.IsError {
		t.Error("expected error for empty title")
	}
}

func TestTasks_InvalidStatusFails(t *testing.T) {
	store := NewTaskStore()
	tt := NewTasks(store)

	execTasks(t, tt, `{"action":"create","tasks":[{"title":"t"}]}`)
	r := execTasks(t, tt, `{"action":"update","id":1,"status":"invalid"}`)
	if !r.IsError {
		t.Error("expected error for invalid status")
	}
}

func TestTasks_UnknownAction(t *testing.T) {
	store := NewTaskStore()
	tt := NewTasks(store)

	r := execTasks(t, tt, `{"action":"nope"}`)
	if !r.IsError {
		t.Error("expected error for unknown action")
	}
}

func TestTasks_SnapshotAndRestore(t *testing.T) {
	store := NewTaskStore()
	tt := NewTasks(store)

	execTasks(t, tt, `{"action":"create","tasks":[{"title":"one"},{"title":"two","status":"completed"}]}`)

	// Snapshot
	snap := store.Snapshot()
	if len(snap) != 2 {
		t.Fatalf("expected 2 tasks in snapshot, got %d", len(snap))
	}

	// Restore into a new store
	store2 := NewTaskStore()
	store2.Restore(snap)
	tt2 := NewTasks(store2)

	r := execTasks(t, tt2, `{"action":"list"}`)
	if !strings.Contains(r.Content, "1/2 completed") {
		t.Errorf("restored list wrong: %s", r.Content)
	}

	// Add should use next ID after restored ones
	r = execTasks(t, tt2, `{"action":"add","title":"three"}`)
	if !strings.Contains(r.Content, "[3]") {
		t.Errorf("expected id 3 after restore, got: %s", r.Content)
	}
}

func TestTasks_ListEmpty(t *testing.T) {
	store := NewTaskStore()
	tt := NewTasks(store)

	r := execTasks(t, tt, `{"action":"list"}`)
	if r.IsError {
		t.Fatalf("list failed: %s", r.Content)
	}
	if !strings.Contains(r.Content, "No tasks") {
		t.Errorf("expected 'No tasks', got: %s", r.Content)
	}
}

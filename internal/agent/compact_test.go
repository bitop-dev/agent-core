package agent

import (
	"testing"

	"github.com/bitop-dev/agent-core/internal/provider"
)

func TestAdjustSplitForToolBoundary_NoToolResults(t *testing.T) {
	history := []provider.Message{
		{Role: provider.RoleUser},
		{Role: provider.RoleAssistant},
		{Role: provider.RoleUser},
		{Role: provider.RoleAssistant},
	}
	// Split at index 2 — no tool results, should stay at 2
	got := adjustSplitForToolBoundary(history, 2)
	if got != 2 {
		t.Errorf("got %d, want 2", got)
	}
}

func TestAdjustSplitForToolBoundary_SplitOnToolResult(t *testing.T) {
	history := []provider.Message{
		{Role: provider.RoleUser},
		{Role: provider.RoleAssistant, Content: []provider.ContentBlock{
			{Type: provider.ContentToolCall, ToolName: "bash"},
		}},
		{Role: provider.RoleToolResult}, // split would land here
		{Role: provider.RoleUser},
		{Role: provider.RoleAssistant},
	}
	// Split at index 2 (tool_result) — should advance to 3
	got := adjustSplitForToolBoundary(history, 2)
	if got != 3 {
		t.Errorf("got %d, want 3 (should skip past tool_result)", got)
	}
}

func TestAdjustSplitForToolBoundary_MultipleToolResults(t *testing.T) {
	history := []provider.Message{
		{Role: provider.RoleUser},
		{Role: provider.RoleAssistant, Content: []provider.ContentBlock{
			{Type: provider.ContentToolCall, ToolName: "bash"},
			{Type: provider.ContentToolCall, ToolName: "list_dir"},
		}},
		{Role: provider.RoleToolResult}, // split would land here
		{Role: provider.RoleToolResult}, // parallel tool result
		{Role: provider.RoleUser},
	}
	// Split at index 2 — should advance past both tool_results to 4
	got := adjustSplitForToolBoundary(history, 2)
	if got != 4 {
		t.Errorf("got %d, want 4 (should skip past all tool_results)", got)
	}
}

func TestAdjustSplitForToolBoundary_AssistantWithToolCallsAtBoundary(t *testing.T) {
	history := []provider.Message{
		{Role: provider.RoleUser},
		{Role: provider.RoleAssistant, Content: []provider.ContentBlock{
			{Type: provider.ContentText, Text: "Let me check"},
			{Type: provider.ContentToolCall, ToolName: "read_file"},
		}},
		{Role: provider.RoleToolResult}, // belongs to the assistant above
		{Role: provider.RoleUser},
	}
	// Split at index 2 (tool_result) — should advance to 3
	got := adjustSplitForToolBoundary(history, 2)
	if got != 3 {
		t.Errorf("got %d, want 3", got)
	}
}

func TestBuildTranscript(t *testing.T) {
	messages := []provider.Message{
		{Role: provider.RoleUser, Content: []provider.ContentBlock{
			{Type: provider.ContentText, Text: "Hello world"},
		}},
		{Role: provider.RoleAssistant, Content: []provider.ContentBlock{
			{Type: provider.ContentText, Text: "Hi there"},
			{Type: provider.ContentToolCall, ToolName: "bash"},
		}},
		{Role: provider.RoleToolResult, Content: []provider.ContentBlock{
			{Type: provider.ContentToolResult, Text: "command output"},
		}},
	}

	transcript := buildTranscript(messages, 10000)

	if transcript == "" {
		t.Fatal("transcript should not be empty")
	}
	if !containsStr(transcript, "user: Hello world") {
		t.Error("should contain user message")
	}
	if !containsStr(transcript, "[called tool bash]") {
		t.Error("should contain tool call")
	}
	if !containsStr(transcript, "tool_result: command output") {
		t.Error("should contain tool result")
	}
}

func TestBuildTranscript_Truncation(t *testing.T) {
	messages := []provider.Message{
		{Role: provider.RoleUser, Content: []provider.ContentBlock{
			{Type: provider.ContentText, Text: "This is a very long message that should be truncated"},
		}},
	}

	transcript := buildTranscript(messages, 20)
	if len(transcript) > 30 { // some overhead for role prefix
		t.Errorf("transcript should be truncated, got %d chars", len(transcript))
	}
}

func TestShouldCompact(t *testing.T) {
	// 80% of 100K = 80K tokens
	if !shouldCompact(85000, 100000, 0.8) {
		t.Error("should compact at 85% usage")
	}
	if shouldCompact(70000, 100000, 0.8) {
		t.Error("should not compact at 70% usage")
	}
	if shouldCompact(0, 100000, 0.8) {
		t.Error("should not compact with 0 tokens")
	}
	if shouldCompact(50000, 0, 0.8) {
		t.Error("should not compact with 0 context window")
	}
}

func containsStr(s, sub string) bool {
	return len(s) >= len(sub) && searchStr(s, sub)
}

func searchStr(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

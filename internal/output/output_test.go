package output

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/bitop-dev/agent-core/internal/agent"
)

func TestTextRenderer_TextDelta(t *testing.T) {
	out := &bytes.Buffer{}
	err := &bytes.Buffer{}
	r := NewTextRenderer(out, err)

	r.Render(agent.RunEvent{Type: agent.EventTextDelta, Data: agent.TextDeltaData{Text: "hello"}})
	r.Render(agent.RunEvent{Type: agent.EventTextDelta, Data: agent.TextDeltaData{Text: " world"}})
	r.Flush()

	if out.String() != "hello world" {
		t.Errorf("expected 'hello world', got %q", out.String())
	}
}

func TestTextRenderer_ToolCalls(t *testing.T) {
	out := &bytes.Buffer{}
	errBuf := &bytes.Buffer{}
	r := NewTextRenderer(out, errBuf)

	r.Render(agent.RunEvent{Type: agent.EventToolCallStart, Data: agent.ToolCallStartData{
		ToolName: "bash", Arguments: `{"command":"ls"}`,
	}})
	r.Render(agent.RunEvent{Type: agent.EventToolCallEnd, Data: agent.ToolCallEndData{
		ToolName: "bash", Content: "file1.txt\nfile2.txt",
	}})

	if !strings.Contains(errBuf.String(), "bash") {
		t.Errorf("expected tool name in stderr, got: %s", errBuf.String())
	}
}

func TestTextRenderer_AgentEnd(t *testing.T) {
	out := &bytes.Buffer{}
	errBuf := &bytes.Buffer{}
	r := NewTextRenderer(out, errBuf)

	r.Render(agent.RunEvent{Type: agent.EventAgentEnd, Data: agent.AgentEndData{
		TotalTurns: 5, StopReason: "complete", DurationMs: 1234,
	}})

	if !strings.Contains(errBuf.String(), "complete") {
		t.Errorf("expected stop reason, got: %s", errBuf.String())
	}
	if !strings.Contains(errBuf.String(), "1234ms") {
		t.Errorf("expected duration, got: %s", errBuf.String())
	}
}

func TestJSONRenderer_Output(t *testing.T) {
	out := &bytes.Buffer{}
	r := NewJSONRenderer(out)

	r.Render(agent.RunEvent{Type: agent.EventTextDelta, Data: agent.TextDeltaData{Text: "hi"}})
	r.Render(agent.RunEvent{Type: agent.EventAgentEnd, Data: agent.AgentEndData{
		TotalTurns: 2, StopReason: "complete", DurationMs: 500,
	}})
	r.Flush()

	var events []JSONEvent
	if err := json.Unmarshal(out.Bytes(), &events); err != nil {
		t.Fatalf("invalid JSON: %v\nraw: %s", err, out.String())
	}
	if len(events) != 2 {
		t.Fatalf("expected 2 events, got %d", len(events))
	}
	if events[0].Type != "text_delta" {
		t.Errorf("expected text_delta, got %s", events[0].Type)
	}
	if events[1].Type != "agent_end" {
		t.Errorf("expected agent_end, got %s", events[1].Type)
	}
}

func TestJSONLRenderer_Output(t *testing.T) {
	out := &bytes.Buffer{}
	r := NewJSONLRenderer(out)

	r.Render(agent.RunEvent{Type: agent.EventTextDelta, Data: agent.TextDeltaData{Text: "hi"}})
	r.Render(agent.RunEvent{Type: agent.EventToolCallStart, Data: agent.ToolCallStartData{
		ToolCallID: "tc1", ToolName: "bash", Arguments: `{"cmd":"ls"}`,
	}})
	r.Render(agent.RunEvent{Type: agent.EventAgentEnd, Data: agent.AgentEndData{
		TotalTurns: 3, StopReason: "complete", DurationMs: 100,
	}})
	r.Flush()

	lines := strings.Split(strings.TrimSpace(out.String()), "\n")
	if len(lines) != 3 {
		t.Fatalf("expected 3 lines, got %d: %s", len(lines), out.String())
	}

	for i, line := range lines {
		var evt JSONEvent
		if err := json.Unmarshal([]byte(line), &evt); err != nil {
			t.Fatalf("line %d invalid JSON: %v", i, err)
		}
	}

	// Verify first line
	var first JSONEvent
	json.Unmarshal([]byte(lines[0]), &first)
	if first.Type != "text_delta" {
		t.Errorf("expected text_delta, got %s", first.Type)
	}
}

func TestJSONLRenderer_ToolCallEnd(t *testing.T) {
	out := &bytes.Buffer{}
	r := NewJSONLRenderer(out)

	r.Render(agent.RunEvent{Type: agent.EventToolCallEnd, Data: agent.ToolCallEndData{
		ToolCallID: "tc1", ToolName: "bash", Content: "output", IsError: true,
	}})

	var evt JSONEvent
	json.Unmarshal(out.Bytes(), &evt)
	if evt.Type != "tool_call_end" {
		t.Errorf("expected tool_call_end, got %s", evt.Type)
	}
	data := evt.Data.(map[string]any)
	if data["is_error"] != true {
		t.Errorf("expected is_error true, got %v", data["is_error"])
	}
}

func TestTruncate(t *testing.T) {
	if s := truncate("hello", 10); s != "hello" {
		t.Errorf("short string modified: %q", s)
	}
	s2 := truncate("hello world this is long", 10)
	if !strings.HasSuffix(s2, "…") {
		t.Errorf("expected truncated with ellipsis, got %q", s2)
	}
	if !strings.HasPrefix(s2, "hello worl") {
		t.Errorf("expected prefix preserved, got %q", s2)
	}
	if s := truncate("line1\nline2", 20); strings.Contains(s, "\n") {
		t.Error("newlines should be replaced")
	}
}

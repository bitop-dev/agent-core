package agent

import (
	"testing"
)

func TestLoadConfig(t *testing.T) {
	cfg, err := ParseConfig([]byte(`
name: test-agent
model: gpt-4o
system_prompt: You are a test agent.
max_turns: 5
`))
	if err != nil {
		t.Fatalf("parse config: %v", err)
	}
	if cfg.Name != "test-agent" {
		t.Errorf("expected name 'test-agent', got %q", cfg.Name)
	}
	if cfg.Model != "gpt-4o" {
		t.Errorf("expected model 'gpt-4o', got %q", cfg.Model)
	}
	if cfg.MaxTurns != 5 {
		t.Errorf("expected max_turns 5, got %d", cfg.MaxTurns)
	}
}

func TestNewBuilder(t *testing.T) {
	b := NewBuilder()
	if b == nil {
		t.Fatal("NewBuilder returned nil")
	}
}

func TestBuildRequiresConfig(t *testing.T) {
	_, err := NewBuilder().Build()
	if err == nil {
		t.Error("expected error when config is missing")
	}
}

func TestNewToolEngine(t *testing.T) {
	e := NewToolEngine()
	defs := e.Definitions()
	if len(defs) != 9 {
		t.Errorf("expected 9 built-in tools, got %d", len(defs))
	}

	names := make(map[string]bool)
	for _, d := range defs {
		names[d.Name] = true
	}
	for _, expected := range []string{"bash", "read_file", "write_file", "edit_file", "list_dir", "grep", "http_fetch", "tasks"} {
		if !names[expected] {
			t.Errorf("missing tool: %s", expected)
		}
	}
}

func TestEventTypeConstants(t *testing.T) {
	// Verify constants are accessible and have expected values
	if EventAgentStart != "agent_start" {
		t.Errorf("unexpected EventAgentStart: %s", EventAgentStart)
	}
	if EventTextDelta != "text_delta" {
		t.Errorf("unexpected EventTextDelta: %s", EventTextDelta)
	}
	if EventToolCallEnd != "tool_call_end" {
		t.Errorf("unexpected EventToolCallEnd: %s", EventToolCallEnd)
	}
	if EventAgentEnd != "agent_end" {
		t.Errorf("unexpected EventAgentEnd: %s", EventAgentEnd)
	}
}

package mcp

import (
	"encoding/json"
	"testing"
)

func TestNewRequest(t *testing.T) {
	req := NewRequest(1, "tools/list", struct{}{})
	if req.JSONRPC != "2.0" {
		t.Errorf("expected jsonrpc 2.0, got %s", req.JSONRPC)
	}
	if req.ID == nil || *req.ID != 1 {
		t.Error("expected id 1")
	}
	if req.Method != "tools/list" {
		t.Errorf("expected tools/list, got %s", req.Method)
	}
}

func TestNewNotification(t *testing.T) {
	n := NewNotification("notifications/initialized", struct{}{})
	if n.ID != nil {
		t.Error("notification should have nil id")
	}
	if n.Method != "notifications/initialized" {
		t.Errorf("unexpected method: %s", n.Method)
	}
}

func TestRequestSerialization(t *testing.T) {
	req := NewRequest(42, "tools/call", map[string]string{"name": "test"})
	data, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var parsed map[string]any
	json.Unmarshal(data, &parsed)
	if parsed["id"].(float64) != 42 {
		t.Error("id should be 42")
	}
	if parsed["method"] != "tools/call" {
		t.Error("method should be tools/call")
	}
}

func TestResponseDeserialization(t *testing.T) {
	raw := `{"jsonrpc":"2.0","id":1,"result":{"tools":[{"name":"test","inputSchema":{"type":"object"}}]}}`
	var resp Response
	if err := json.Unmarshal([]byte(raw), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp.Error != nil {
		t.Error("expected no error")
	}

	var toolList ToolsListResult
	json.Unmarshal(resp.Result, &toolList)
	if len(toolList.Tools) != 1 {
		t.Fatalf("expected 1 tool, got %d", len(toolList.Tools))
	}
	if toolList.Tools[0].Name != "test" {
		t.Errorf("expected tool name 'test', got %q", toolList.Tools[0].Name)
	}
}

func TestResponseError(t *testing.T) {
	raw := `{"jsonrpc":"2.0","id":1,"error":{"code":-32601,"message":"method not found"}}`
	var resp Response
	json.Unmarshal([]byte(raw), &resp)
	if resp.Error == nil {
		t.Fatal("expected error")
	}
	if resp.Error.Code != -32601 {
		t.Errorf("expected code -32601, got %d", resp.Error.Code)
	}
	if resp.Error.Message != "method not found" {
		t.Errorf("unexpected message: %s", resp.Error.Message)
	}
}

func TestToolDefDeserialization(t *testing.T) {
	raw := `{"name":"read_file","description":"Read a file","inputSchema":{"type":"object","properties":{"path":{"type":"string"}}}}`
	var def ToolDef
	if err := json.Unmarshal([]byte(raw), &def); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if def.Name != "read_file" {
		t.Errorf("expected name 'read_file', got %q", def.Name)
	}
	if def.Description != "Read a file" {
		t.Errorf("unexpected description: %s", def.Description)
	}
}

func TestCallToolResultDeserialization(t *testing.T) {
	raw := `{"content":[{"type":"text","text":"hello world"}],"isError":false}`
	var result CallToolResult
	if err := json.Unmarshal([]byte(raw), &result); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(result.Content) != 1 {
		t.Fatalf("expected 1 content block, got %d", len(result.Content))
	}
	if result.Content[0].Text != "hello world" {
		t.Errorf("unexpected text: %s", result.Content[0].Text)
	}
	if result.IsError {
		t.Error("expected isError false")
	}
}

func TestSanitizeName(t *testing.T) {
	tests := map[string]string{
		"context7":     "context7",
		"my-server":    "my_server",
		"AI Elements":  "AI_Elements",
		"test.123":     "test_123",
		"MCP/server_1": "MCP_server_1",
	}
	for input, expected := range tests {
		got := sanitizeName(input)
		if got != expected {
			t.Errorf("sanitizeName(%q) = %q, want %q", input, got, expected)
		}
	}
}

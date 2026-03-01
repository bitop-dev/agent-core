package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/bitop-dev/agent-core/internal/config"
	"github.com/bitop-dev/agent-core/internal/tool"
)

// ToolAdapter wraps an MCP tool as a standard tool.Tool.
type ToolAdapter struct {
	client  *Client
	toolDef ToolDef
	prefix  string // e.g., "mcp__context7__" to namespace tool names
}

// NewToolAdapter creates an adapter for a single MCP tool.
func NewToolAdapter(client *Client, def ToolDef, prefix string) *ToolAdapter {
	return &ToolAdapter{
		client:  client,
		toolDef: def,
		prefix:  prefix,
	}
}

func (a *ToolAdapter) Definition() tool.Definition {
	name := a.prefix + a.toolDef.Name
	desc := a.toolDef.Description
	if desc == "" {
		desc = fmt.Sprintf("MCP tool from %s", a.client.Name())
	}
	return tool.Definition{
		Name:        name,
		Description: desc,
		InputSchema: a.toolDef.InputSchema,
	}
}

func (a *ToolAdapter) Execute(ctx context.Context, input json.RawMessage) (tool.Result, error) {
	result, err := a.client.CallTool(a.toolDef.Name, input)
	if err != nil {
		return tool.Result{Content: fmt.Sprintf("MCP error: %v", err), IsError: true}, nil
	}

	// Collect text content from result
	var parts []string
	for _, block := range result.Content {
		if block.Type == "text" && block.Text != "" {
			parts = append(parts, block.Text)
		}
	}

	content := strings.Join(parts, "\n")
	if content == "" {
		content = "(no output)"
	}

	return tool.Result{Content: content, IsError: result.IsError}, nil
}

// RegisterAll connects to all configured MCP servers and registers their tools
// with the given tool engine. Returns the connected clients (caller should close them).
// Non-fatal errors (individual server failures) are logged but don't stop other servers.
func RegisterAll(servers []config.MCPServer, engine *tool.Engine) ([]*Client, []error) {
	var clients []*Client
	var errs []error

	for _, srv := range servers {
		client, err := Connect(srv)
		if err != nil {
			errs = append(errs, fmt.Errorf("MCP server %q: %w", srv.Name, err))
			continue
		}

		prefix := fmt.Sprintf("mcp_%s_", sanitizeName(srv.Name))
		for _, def := range client.Tools() {
			adapter := NewToolAdapter(client, def, prefix)
			engine.Register(adapter)
		}

		clients = append(clients, client)
	}

	return clients, errs
}

// sanitizeName replaces non-alphanumeric chars with underscores for tool name prefixes.
func sanitizeName(name string) string {
	var b strings.Builder
	for _, r := range name {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_' {
			b.WriteRune(r)
		} else {
			b.WriteRune('_')
		}
	}
	return b.String()
}

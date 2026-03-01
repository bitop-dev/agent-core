package mcp

import (
	"encoding/json"
	"fmt"
	"sync/atomic"
	"time"

	"github.com/bitop-dev/agent-core/internal/config"
)

// DefaultRecvTimeout is the timeout for handshake and tool list operations.
const DefaultRecvTimeout = 30 * time.Second

// DefaultToolTimeout is the timeout for individual tool calls.
const DefaultToolTimeout = 180 * time.Second

// Client is a connected MCP server with available tools.
type Client struct {
	name      string
	transport Transport
	tools     []ToolDef
	nextID    atomic.Int64
}

// Connect creates a transport, performs the initialize handshake,
// and fetches the tool list from an MCP server.
func Connect(serverCfg config.MCPServer) (*Client, error) {
	transport, err := createTransport(serverCfg)
	if err != nil {
		return nil, fmt.Errorf("create transport for %q: %w", serverCfg.Name, err)
	}

	c := &Client{
		name:      serverCfg.Name,
		transport: transport,
	}
	c.nextID.Store(1)

	// Initialize handshake
	initResp, err := c.call("initialize", InitializeParams{
		ProtocolVersion: MCPProtocolVersion,
		ClientInfo:      ClientInfo{Name: "agent-core", Version: "1.0.0"},
	})
	if err != nil {
		transport.Close()
		return nil, fmt.Errorf("initialize %q: %w", serverCfg.Name, err)
	}
	if initResp.Error != nil {
		transport.Close()
		return nil, fmt.Errorf("initialize %q rejected: %s", serverCfg.Name, initResp.Error.Message)
	}

	// Send initialized notification
	notif := NewNotification("notifications/initialized", struct{}{})
	transport.SendRecv(notif) // best effort

	// Fetch tool list
	listResp, err := c.call("tools/list", struct{}{})
	if err != nil {
		transport.Close()
		return nil, fmt.Errorf("tools/list %q: %w", serverCfg.Name, err)
	}
	if listResp.Error != nil {
		transport.Close()
		return nil, fmt.Errorf("tools/list %q error: %s", serverCfg.Name, listResp.Error.Message)
	}

	var toolList ToolsListResult
	if err := json.Unmarshal(listResp.Result, &toolList); err != nil {
		transport.Close()
		return nil, fmt.Errorf("parse tools/list from %q: %w", serverCfg.Name, err)
	}

	c.tools = toolList.Tools
	return c, nil
}

// Name returns the server's display name.
func (c *Client) Name() string { return c.name }

// Tools returns the tools advertised by this server.
func (c *Client) Tools() []ToolDef { return c.tools }

// CallTool invokes a tool on the server.
func (c *Client) CallTool(toolName string, arguments json.RawMessage) (CallToolResult, error) {
	params := map[string]any{
		"name":      toolName,
		"arguments": json.RawMessage(arguments),
	}

	resp, err := c.call("tools/call", params)
	if err != nil {
		return CallToolResult{}, fmt.Errorf("call %q on %q: %w", toolName, c.name, err)
	}
	if resp.Error != nil {
		return CallToolResult{
			Content: []ContentBlock{{Type: "text", Text: resp.Error.Message}},
			IsError: true,
		}, nil
	}

	var result CallToolResult
	if err := json.Unmarshal(resp.Result, &result); err != nil {
		return CallToolResult{}, fmt.Errorf("parse result of %q: %w", toolName, err)
	}
	return result, nil
}

// Close shuts down the transport.
func (c *Client) Close() error {
	return c.transport.Close()
}

// call sends a JSON-RPC request and returns the response.
func (c *Client) call(method string, params any) (Response, error) {
	id := c.nextID.Add(1)
	req := NewRequest(id, method, params)
	return c.transport.SendRecv(req)
}

// createTransport builds the appropriate transport from server config.
func createTransport(cfg config.MCPServer) (Transport, error) {
	switch cfg.Transport {
	case "stdio", "":
		if len(cfg.Command) == 0 {
			return nil, fmt.Errorf("stdio transport requires command")
		}
		cmd := cfg.Command[0]
		args := cfg.Command[1:]
		return NewStdioTransport(cmd, args, nil)

	case "http", "sse", "streamable-http":
		if cfg.URL == "" {
			return nil, fmt.Errorf("%s transport requires url", cfg.Transport)
		}
		headers := make(map[string]string)
		if cfg.Headers != nil {
			for k, v := range cfg.Headers {
				headers[k] = v
			}
		}
		return NewHTTPTransport(cfg.URL, headers), nil

	default:
		return nil, fmt.Errorf("unknown transport: %q", cfg.Transport)
	}
}

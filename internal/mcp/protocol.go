// Package mcp implements the Model Context Protocol (MCP) client.
// Supports stdio and HTTP transports for connecting to external tool servers.
package mcp

import "encoding/json"

const (
	JSONRPCVersion     = "2.0"
	MCPProtocolVersion = "2024-11-05"
)

// Request is an outbound JSON-RPC 2.0 request.
type Request struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      *int64          `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

// NewRequest creates a method call with a numeric ID.
func NewRequest(id int64, method string, params any) Request {
	p, _ := json.Marshal(params)
	return Request{
		JSONRPC: JSONRPCVersion,
		ID:      &id,
		Method:  method,
		Params:  p,
	}
}

// NewNotification creates a notification (no ID, no response expected).
func NewNotification(method string, params any) Request {
	p, _ := json.Marshal(params)
	return Request{
		JSONRPC: JSONRPCVersion,
		Method:  method,
		Params:  p,
	}
}

// Response is an inbound JSON-RPC 2.0 response.
type Response struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      *int64          `json:"id,omitempty"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *RPCError       `json:"error,omitempty"`
}

// RPCError is a JSON-RPC error object.
type RPCError struct {
	Code    int             `json:"code"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data,omitempty"`
}

func (e *RPCError) Error() string { return e.Message }

// ToolDef is a tool advertised by an MCP server (from tools/list).
type ToolDef struct {
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	InputSchema json.RawMessage `json:"inputSchema"`
}

// ToolsListResult is the expected shape of the tools/list result.
type ToolsListResult struct {
	Tools []ToolDef `json:"tools"`
}

// CallToolResult is the expected shape of the tools/call result.
type CallToolResult struct {
	Content []ContentBlock `json:"content"`
	IsError bool           `json:"isError,omitempty"`
}

// ContentBlock is a single content block in a tool result.
type ContentBlock struct {
	Type string `json:"type"` // "text", "image", "resource"
	Text string `json:"text,omitempty"`
}

// InitializeParams sent during the initialize handshake.
type InitializeParams struct {
	ProtocolVersion string     `json:"protocolVersion"`
	Capabilities    struct{}   `json:"capabilities"`
	ClientInfo      ClientInfo `json:"clientInfo"`
}

// ClientInfo identifies this client to the server.
type ClientInfo struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

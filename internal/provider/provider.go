// Package provider defines the LLM provider interface and implementations.
package provider

import "context"

// Provider is the interface all LLM providers implement.
type Provider interface {
	// Complete sends a completion request and returns a channel of streaming events.
	Complete(ctx context.Context, req CompletionRequest) (<-chan CompletionEvent, error)

	// Name returns the provider name (e.g., "anthropic", "openai").
	Name() string

	// Capabilities returns what this provider supports.
	Capabilities() Capabilities
}

// KeyRotatable is optionally implemented by providers that support API key rotation.
type KeyRotatable interface {
	SetAPIKey(key string)
}

// CompletionRequest is what gets sent to the LLM.
type CompletionRequest struct {
	Model        string
	SystemPrompt string
	Messages     []Message
	Tools        []ToolSpec
	MaxTokens    int
}

// Message is a conversation message.
type Message struct {
	Role    Role
	Content []ContentBlock
}

// Role is the message sender.
type Role string

const (
	RoleUser      Role = "user"
	RoleAssistant Role = "assistant"
	RoleToolResult Role = "tool_result"
)

// ContentBlock is a piece of content within a message.
type ContentBlock struct {
	Type       ContentType
	Text       string
	ToolCallID string
	ToolName   string
	Arguments  string // raw JSON for tool calls
	IsError    bool   // for tool results
}

// ContentType identifies the kind of content block.
type ContentType string

const (
	ContentText       ContentType = "text"
	ContentThinking   ContentType = "thinking"
	ContentToolCall   ContentType = "tool_call"
	ContentToolResult ContentType = "tool_result"
	ContentImage      ContentType = "image"
)

// ToolSpec describes a tool the LLM can call.
type ToolSpec struct {
	Name        string
	Description string
	InputSchema string // JSON Schema
}

// CompletionEvent is a streaming event from the LLM.
type CompletionEvent struct {
	Type      CompletionEventType
	Text      string         // for text_delta, thinking_delta
	ToolCall  *ToolCallEvent // for tool_call
	Usage     *Usage         // for usage (end of response)
	Error     error          // for error
	StopReason string        // for done
}

// CompletionEventType identifies the streaming event kind.
type CompletionEventType string

const (
	EventTextDelta     CompletionEventType = "text_delta"
	EventThinkingDelta CompletionEventType = "thinking_delta"
	EventToolCall      CompletionEventType = "tool_call"
	EventUsage         CompletionEventType = "usage"
	EventDone          CompletionEventType = "done"
	EventProviderError CompletionEventType = "error"
)

// ToolCallEvent is a complete tool call from the LLM.
type ToolCallEvent struct {
	ID        string
	Name      string
	Arguments string // raw JSON
}

// Usage tracks token counts for a completion.
type Usage struct {
	InputTokens  int
	OutputTokens int
}

// Capabilities declares what a provider supports.
type Capabilities struct {
	NativeToolCalling bool
	Vision            bool
	ExtendedThinking  bool
	Streaming         bool
}

package provider

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// OpenAIConfig holds connection settings for an OpenAI-compatible endpoint.
type OpenAIConfig struct {
	BaseURL string // e.g., "https://api.openai.com/v1" or a proxy URL
	APIKey  string
}

// openaiProvider implements Provider for OpenAI-compatible APIs.
type openaiProvider struct {
	config OpenAIConfig
	client *http.Client
}

// NewOpenAI creates a provider for any OpenAI-compatible API endpoint.
func NewOpenAI(cfg OpenAIConfig) Provider {
	return &openaiProvider{
		config: cfg,
		client: &http.Client{},
	}
}

func (p *openaiProvider) Name() string { return "openai" }

func (p *openaiProvider) Capabilities() Capabilities {
	return Capabilities{
		NativeToolCalling: true,
		Vision:            true,
		Streaming:         true,
	}
}

func (p *openaiProvider) Complete(ctx context.Context, req CompletionRequest) (<-chan CompletionEvent, error) {
	ch := make(chan CompletionEvent, 64)

	// Build request body
	body := p.buildRequestBody(req)

	jsonBody, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	url := strings.TrimRight(p.config.BaseURL, "/") + "/chat/completions"
	httpReq, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(jsonBody))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+p.config.APIKey)

	go func() {
		defer close(ch)
		p.stream(httpReq, ch)
	}()

	return ch, nil
}

// --- Request building ---

type oaiRequest struct {
	Model               string       `json:"model"`
	Messages            []oaiMessage `json:"messages"`
	Stream              bool         `json:"stream"`
	StreamOptions       *oaiStreamOp `json:"stream_options,omitempty"`
	MaxCompletionTokens int          `json:"max_completion_tokens,omitempty"`
	Tools               []oaiTool    `json:"tools,omitempty"`
}

type oaiStreamOp struct {
	IncludeUsage bool `json:"include_usage"`
}

type oaiMessage struct {
	Role       string          `json:"role"`
	Content    any             `json:"content"` // string or []oaiContentPart
	ToolCalls  []oaiToolCall   `json:"tool_calls,omitempty"`
	ToolCallID string          `json:"tool_call_id,omitempty"`
}

type oaiContentPart struct {
	Type string `json:"type"`
	Text string `json:"text,omitempty"`
}

type oaiTool struct {
	Type     string      `json:"type"`
	Function oaiFunction `json:"function"`
}

type oaiFunction struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Parameters  json.RawMessage `json:"parameters"`
}

type oaiToolCall struct {
	ID       string         `json:"id"`
	Type     string         `json:"type"`
	Function oaiToolCallFn  `json:"function"`
	Index    int            `json:"index"`
}

type oaiToolCallFn struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

func (p *openaiProvider) buildRequestBody(req CompletionRequest) oaiRequest {
	body := oaiRequest{
		Model:               req.Model,
		Stream:              true,
		StreamOptions:       &oaiStreamOp{IncludeUsage: true},
		MaxCompletionTokens: req.MaxTokens,
	}
	if body.MaxCompletionTokens == 0 {
		body.MaxCompletionTokens = 4096
	}

	// System prompt as first message
	if req.SystemPrompt != "" {
		body.Messages = append(body.Messages, oaiMessage{
			Role:    "system",
			Content: req.SystemPrompt,
		})
	}

	// Convert messages
	for _, msg := range req.Messages {
		body.Messages = append(body.Messages, convertMessage(msg))
	}

	// Convert tools
	for _, t := range req.Tools {
		body.Tools = append(body.Tools, oaiTool{
			Type: "function",
			Function: oaiFunction{
				Name:        t.Name,
				Description: t.Description,
				Parameters:  json.RawMessage(t.InputSchema),
			},
		})
	}

	return body
}

func convertMessage(msg Message) oaiMessage {
	switch msg.Role {
	case RoleUser:
		text := extractText(msg.Content)
		return oaiMessage{Role: "user", Content: text}

	case RoleAssistant:
		oai := oaiMessage{Role: "assistant"}
		text := extractText(msg.Content)
		if text != "" {
			oai.Content = text
		}
		// Collect tool calls
		for _, block := range msg.Content {
			if block.Type == ContentToolCall {
				oai.ToolCalls = append(oai.ToolCalls, oaiToolCall{
					ID:   block.ToolCallID,
					Type: "function",
					Function: oaiToolCallFn{
						Name:      block.ToolName,
						Arguments: block.Arguments,
					},
				})
			}
		}
		return oai

	case RoleToolResult:
		// Each tool result is a separate message with role "tool"
		block := msg.Content[0]
		return oaiMessage{
			Role:       "tool",
			Content:    block.Text,
			ToolCallID: block.ToolCallID,
		}

	default:
		return oaiMessage{Role: string(msg.Role), Content: extractText(msg.Content)}
	}
}

func extractText(blocks []ContentBlock) string {
	var parts []string
	for _, b := range blocks {
		if b.Type == ContentText {
			parts = append(parts, b.Text)
		}
	}
	return strings.Join(parts, "")
}

// --- Streaming ---

type oaiChunk struct {
	Choices []oaiChunkChoice `json:"choices"`
	Usage   *oaiUsage        `json:"usage,omitempty"`
}

type oaiChunkChoice struct {
	Delta      oaiChunkDelta `json:"delta"`
	FinishReason *string     `json:"finish_reason"`
}

type oaiChunkDelta struct {
	Content   string        `json:"content"`
	ToolCalls []oaiToolCall `json:"tool_calls,omitempty"`
}

type oaiUsage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
}

func (p *openaiProvider) stream(req *http.Request, ch chan<- CompletionEvent) {
	resp, err := p.client.Do(req)
	if err != nil {
		ch <- CompletionEvent{Type: EventProviderError, Error: fmt.Errorf("http request: %w", err)}
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		ch <- CompletionEvent{
			Type:  EventProviderError,
			Error: fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(body)),
		}
		return
	}

	// Track tool calls being assembled across chunks
	toolCalls := map[int]*ToolCallEvent{}

	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 0, 256*1024), 256*1024)

	for scanner.Scan() {
		line := scanner.Text()

		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		data := strings.TrimPrefix(line, "data: ")
		if data == "[DONE]" {
			break
		}

		var chunk oaiChunk
		if err := json.Unmarshal([]byte(data), &chunk); err != nil {
			continue
		}

		// Usage event (final chunk)
		if chunk.Usage != nil {
			ch <- CompletionEvent{
				Type: EventUsage,
				Usage: &Usage{
					InputTokens:  chunk.Usage.PromptTokens,
					OutputTokens: chunk.Usage.CompletionTokens,
				},
			}
		}

		if len(chunk.Choices) == 0 {
			continue
		}
		choice := chunk.Choices[0]

		// Text content
		if choice.Delta.Content != "" {
			ch <- CompletionEvent{Type: EventTextDelta, Text: choice.Delta.Content}
		}

		// Tool call deltas — accumulate across chunks
		for _, tc := range choice.Delta.ToolCalls {
			existing, ok := toolCalls[tc.Index]
			if !ok {
				existing = &ToolCallEvent{
					ID:   tc.ID,
					Name: tc.Function.Name,
				}
				toolCalls[tc.Index] = existing
			}
			if tc.ID != "" {
				existing.ID = tc.ID
			}
			if tc.Function.Name != "" {
				existing.Name = tc.Function.Name
			}
			existing.Arguments += tc.Function.Arguments
		}

		// Finish reason
		if choice.FinishReason != nil {
			// Emit accumulated tool calls
			if *choice.FinishReason == "tool_calls" {
				for i := 0; i < len(toolCalls); i++ {
					if tc, ok := toolCalls[i]; ok {
						ch <- CompletionEvent{Type: EventToolCall, ToolCall: tc}
					}
				}
			}
			ch <- CompletionEvent{Type: EventDone, StopReason: *choice.FinishReason}
		}
	}
}

func init() {
	Register("openai", func(apiKeys []string) (Provider, error) {
		if len(apiKeys) == 0 {
			return nil, fmt.Errorf("openai: at least one API key required")
		}
		return NewOpenAI(OpenAIConfig{
			BaseURL: "https://api.openai.com/v1",
			APIKey:  apiKeys[0],
		}), nil
	})
}

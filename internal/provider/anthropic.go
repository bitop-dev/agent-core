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

// AnthropicConfig holds connection settings for an Anthropic-compatible endpoint.
type AnthropicConfig struct {
	BaseURL string // e.g., "https://api.anthropic.com" or a proxy
	APIKey  string
}

type anthropicProvider struct {
	config AnthropicConfig
	client *http.Client
}

// NewAnthropic creates a provider for the Anthropic Messages API.
func NewAnthropic(cfg AnthropicConfig) Provider {
	if cfg.BaseURL == "" {
		cfg.BaseURL = "https://api.anthropic.com"
	}
	return &anthropicProvider{
		config: cfg,
		client: &http.Client{},
	}
}

func (p *anthropicProvider) Name() string { return "anthropic" }

func (p *anthropicProvider) Capabilities() Capabilities {
	return Capabilities{
		NativeToolCalling: true,
		Vision:            true,
		ExtendedThinking:  true,
		Streaming:         true,
	}
}

func (p *anthropicProvider) Complete(ctx context.Context, req CompletionRequest) (<-chan CompletionEvent, error) {
	ch := make(chan CompletionEvent, 64)

	body := p.buildRequestBody(req)
	jsonBody, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	url := strings.TrimRight(p.config.BaseURL, "/") + "/v1/messages"
	httpReq, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(jsonBody))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("x-api-key", p.config.APIKey)
	httpReq.Header.Set("anthropic-version", "2023-06-01")

	go func() {
		defer close(ch)
		p.stream(httpReq, ch)
	}()

	return ch, nil
}

// --- Request types ---

type anthRequest struct {
	Model     string        `json:"model"`
	MaxTokens int           `json:"max_tokens"`
	Stream    bool          `json:"stream"`
	System    string        `json:"system,omitempty"`
	Messages  []anthMessage `json:"messages"`
	Tools     []anthTool    `json:"tools,omitempty"`
}

type anthMessage struct {
	Role    string    `json:"role"`
	Content any       `json:"content"` // string or []anthContentBlock
}

type anthContentBlock struct {
	Type       string `json:"type"`
	Text       string `json:"text,omitempty"`
	ID         string `json:"id,omitempty"`          // tool_use
	Name       string `json:"name,omitempty"`        // tool_use
	Input      any    `json:"input,omitempty"`       // tool_use
	ToolUseID  string `json:"tool_use_id,omitempty"` // tool_result
	Content    string `json:"content,omitempty"`     // tool_result (when nested)
	IsError    bool   `json:"is_error,omitempty"`    // tool_result
}

type anthTool struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	InputSchema json.RawMessage `json:"input_schema"`
}

func (p *anthropicProvider) buildRequestBody(req CompletionRequest) anthRequest {
	body := anthRequest{
		Model:     req.Model,
		MaxTokens: req.MaxTokens,
		Stream:    true,
		System:    req.SystemPrompt,
	}
	if body.MaxTokens == 0 {
		body.MaxTokens = 4096
	}

	for _, msg := range req.Messages {
		body.Messages = append(body.Messages, convertAnthMessage(msg))
	}

	for _, t := range req.Tools {
		body.Tools = append(body.Tools, anthTool{
			Name:        t.Name,
			Description: t.Description,
			InputSchema: json.RawMessage(t.InputSchema),
		})
	}

	return body
}

func convertAnthMessage(msg Message) anthMessage {
	switch msg.Role {
	case RoleUser:
		text := extractText(msg.Content)
		return anthMessage{Role: "user", Content: text}

	case RoleAssistant:
		var blocks []anthContentBlock
		for _, b := range msg.Content {
			switch b.Type {
			case ContentText:
				if b.Text != "" {
					blocks = append(blocks, anthContentBlock{Type: "text", Text: b.Text})
				}
			case ContentToolCall:
				var input any
				json.Unmarshal([]byte(b.Arguments), &input)
				blocks = append(blocks, anthContentBlock{
					Type:  "tool_use",
					ID:    b.ToolCallID,
					Name:  b.ToolName,
					Input: input,
				})
			}
		}
		return anthMessage{Role: "assistant", Content: blocks}

	case RoleToolResult:
		var blocks []anthContentBlock
		for _, b := range msg.Content {
			blocks = append(blocks, anthContentBlock{
				Type:      "tool_result",
				ToolUseID: b.ToolCallID,
				Content:   b.Text,
				IsError:   b.IsError,
			})
		}
		return anthMessage{Role: "user", Content: blocks}

	default:
		return anthMessage{Role: string(msg.Role), Content: extractText(msg.Content)}
	}
}

// --- Streaming ---

// Anthropic SSE event types
type anthSSEvent struct {
	Event string
	Data  json.RawMessage
}

type anthMessageStart struct {
	Message struct {
		Usage struct {
			InputTokens  int `json:"input_tokens"`
			OutputTokens int `json:"output_tokens"`
		} `json:"usage"`
	} `json:"message"`
}

type anthContentBlockStart struct {
	Index        int `json:"index"`
	ContentBlock struct {
		Type  string `json:"type"`
		ID    string `json:"id,omitempty"`
		Name  string `json:"name,omitempty"`
		Text  string `json:"text,omitempty"`
	} `json:"content_block"`
}

type anthContentBlockDelta struct {
	Index int `json:"index"`
	Delta struct {
		Type        string `json:"type"`
		Text        string `json:"text,omitempty"`
		PartialJSON string `json:"partial_json,omitempty"`
	} `json:"delta"`
}

type anthMessageDelta struct {
	Delta struct {
		StopReason string `json:"stop_reason"`
	} `json:"delta"`
	Usage struct {
		OutputTokens int `json:"output_tokens"`
	} `json:"usage"`
}

func (p *anthropicProvider) stream(req *http.Request, ch chan<- CompletionEvent) {
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

	// Track active content blocks for tool call assembly
	type blockState struct {
		blockType string
		id        string
		name      string
		jsonBuf   string
	}
	blocks := map[int]*blockState{}
	var inputTokens int

	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 0, 256*1024), 256*1024)

	var currentEvent string
	for scanner.Scan() {
		line := scanner.Text()

		if strings.HasPrefix(line, "event: ") {
			currentEvent = strings.TrimPrefix(line, "event: ")
			continue
		}

		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		data := []byte(strings.TrimPrefix(line, "data: "))

		switch currentEvent {
		case "message_start":
			var ms anthMessageStart
			if json.Unmarshal(data, &ms) == nil {
				inputTokens = ms.Message.Usage.InputTokens
			}

		case "content_block_start":
			var cbs anthContentBlockStart
			if json.Unmarshal(data, &cbs) == nil {
				blocks[cbs.Index] = &blockState{
					blockType: cbs.ContentBlock.Type,
					id:        cbs.ContentBlock.ID,
					name:      cbs.ContentBlock.Name,
				}
			}

		case "content_block_delta":
			var cbd anthContentBlockDelta
			if json.Unmarshal(data, &cbd) == nil {
				switch cbd.Delta.Type {
				case "text_delta":
					ch <- CompletionEvent{Type: EventTextDelta, Text: cbd.Delta.Text}
				case "thinking_delta":
					ch <- CompletionEvent{Type: EventThinkingDelta, Text: cbd.Delta.Text}
				case "input_json_delta":
					if bs, ok := blocks[cbd.Index]; ok {
						bs.jsonBuf += cbd.Delta.PartialJSON
					}
				}
			}

		case "content_block_stop":
			// When a tool_use block stops, emit the complete tool call
			var stop struct {
				Index int `json:"index"`
			}
			if json.Unmarshal(data, &stop) == nil {
				if bs, ok := blocks[stop.Index]; ok && bs.blockType == "tool_use" {
					ch <- CompletionEvent{
						Type: EventToolCall,
						ToolCall: &ToolCallEvent{
							ID:        bs.id,
							Name:      bs.name,
							Arguments: bs.jsonBuf,
						},
					}
				}
			}

		case "message_delta":
			var md anthMessageDelta
			if json.Unmarshal(data, &md) == nil {
				// Emit usage with combined tokens
				ch <- CompletionEvent{
					Type: EventUsage,
					Usage: &Usage{
						InputTokens:  inputTokens,
						OutputTokens: md.Usage.OutputTokens,
					},
				}
				// Map stop reason
				stop := md.Delta.StopReason
				switch stop {
				case "end_turn":
					stop = "stop"
				case "tool_use":
					stop = "tool_calls"
				}
				ch <- CompletionEvent{Type: EventDone, StopReason: stop}
			}

		case "message_stop":
			// Stream complete
			return
		}
	}
}

func init() {
	Register("anthropic", func(apiKeys []string) (Provider, error) {
		if len(apiKeys) == 0 {
			return nil, fmt.Errorf("anthropic: at least one API key required")
		}
		return NewAnthropic(AnthropicConfig{
			BaseURL: "https://api.anthropic.com",
			APIKey:  apiKeys[0],
		}), nil
	})
}

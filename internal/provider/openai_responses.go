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

// OpenAIResponsesConfig holds settings for the OpenAI Responses API.
type OpenAIResponsesConfig struct {
	BaseURL string
	APIKey  string
}

type openaiResponsesProvider struct {
	config OpenAIResponsesConfig
	client *http.Client
}

// SetAPIKey rotates the API key for this provider.
func (p *openaiResponsesProvider) SetAPIKey(key string) {
	p.config.APIKey = key
}

// NewOpenAIResponses creates a provider using the OpenAI Responses API (/v1/responses).
func NewOpenAIResponses(cfg OpenAIResponsesConfig) Provider {
	return &openaiResponsesProvider{
		config: cfg,
		client: &http.Client{},
	}
}

func (p *openaiResponsesProvider) Name() string { return "openai-responses" }

func (p *openaiResponsesProvider) Capabilities() Capabilities {
	return Capabilities{
		NativeToolCalling: true,
		Vision:            true,
		Streaming:         true,
	}
}

func (p *openaiResponsesProvider) Complete(ctx context.Context, req CompletionRequest) (<-chan CompletionEvent, error) {
	ch := make(chan CompletionEvent, 64)

	body := p.buildRequestBody(req)
	jsonBody, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	url := strings.TrimRight(p.config.BaseURL, "/") + "/v1/responses"
	httpReq, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(jsonBody))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+p.config.APIKey)

	resp, err := p.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("http request: %w", err)
	}
	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(body))
	}

	go func() {
		defer close(ch)
		defer resp.Body.Close()
		p.streamFromReader(resp.Body, ch)
	}()

	return ch, nil
}

// --- Request types ---

type respRequest struct {
	Model              string     `json:"model"`
	Input              any        `json:"input"` // string or []respInputItem
	Stream             bool       `json:"stream"`
	Tools              []respTool `json:"tools,omitempty"`
	MaxOutputTokens    int        `json:"max_output_tokens,omitempty"`
	Instructions       string     `json:"instructions,omitempty"`
	PreviousResponseID string     `json:"previous_response_id,omitempty"`
}

type respInputItem struct {
	Type      string `json:"type"`
	Role      string `json:"role,omitempty"`
	Content   any    `json:"content,omitempty"` // string or []respContentPart
	// For function_call
	ID        string `json:"id,omitempty"`
	CallID    string `json:"call_id,omitempty"`
	Name      string `json:"name,omitempty"`
	Arguments string `json:"arguments,omitempty"`
	// For function_call_output
	Output    string `json:"output,omitempty"`
}

type respContentPart struct {
	Type string `json:"type"`
	Text string `json:"text,omitempty"`
}

type respTool struct {
	Type        string          `json:"type"`
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Parameters  json.RawMessage `json:"parameters"`
}

func (p *openaiResponsesProvider) buildRequestBody(req CompletionRequest) respRequest {
	body := respRequest{
		Model:           req.Model,
		Stream:          true,
		MaxOutputTokens: req.MaxTokens,
		Instructions:    req.SystemPrompt,
	}
	if body.MaxOutputTokens == 0 {
		body.MaxOutputTokens = 4096
	}

	// Convert messages to input items
	// If it's just one user message with no history, use a simple string input
	if len(req.Messages) == 1 && req.Messages[0].Role == RoleUser {
		body.Input = extractText(req.Messages[0].Content)
	} else {
		body.Input = p.convertMessages(req.Messages)
	}

	// Convert tools
	for _, t := range req.Tools {
		body.Tools = append(body.Tools, respTool{
			Type:        "function",
			Name:        t.Name,
			Description: t.Description,
			Parameters:  json.RawMessage(t.InputSchema),
		})
	}

	return body
}

func (p *openaiResponsesProvider) convertMessages(msgs []Message) []respInputItem {
	var items []respInputItem

	for _, msg := range msgs {
		switch msg.Role {
		case RoleUser:
			items = append(items, respInputItem{
				Type:    "message",
				Role:    "user",
				Content: extractText(msg.Content),
			})

		case RoleAssistant:
			// Assistant messages with text
			text := extractText(msg.Content)
			if text != "" {
				items = append(items, respInputItem{
					Type:    "message",
					Role:    "assistant",
					Content: text,
				})
			}
			// Assistant tool calls become function_call items
			for _, b := range msg.Content {
				if b.Type == ContentToolCall {
					items = append(items, respInputItem{
						Type:      "function_call",
						CallID:    b.ToolCallID,
						Name:      b.ToolName,
						Arguments: b.Arguments,
					})
				}
			}

		case RoleToolResult:
			// Tool results become function_call_output items
			for _, b := range msg.Content {
				items = append(items, respInputItem{
					Type:   "function_call_output",
					CallID: b.ToolCallID,
					Output: b.Text,
				})
			}
		}
	}

	return items
}

// --- Streaming ---

type respStreamEvent struct {
	Type string          `json:"type"`
	Data json.RawMessage `json:"-"` // we'll unmarshal per-type
}

func (p *openaiResponsesProvider) streamFromReader(body io.Reader, ch chan<- CompletionEvent) {
	// Track function calls being built across deltas
	type fcState struct {
		id   string
		name string
		args string
	}
	funcCalls := map[int]*fcState{} // output_index → state

	scanner := bufio.NewScanner(body)
	scanner.Buffer(make([]byte, 0, 512*1024), 512*1024)

	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		data := strings.TrimPrefix(line, "data: ")
		if data == "[DONE]" {
			break
		}

		// Parse event type first
		var evt struct {
			Type string `json:"type"`
		}
		if json.Unmarshal([]byte(data), &evt) != nil {
			continue
		}

		switch evt.Type {
		case "response.output_text.delta":
			var d struct {
				Delta string `json:"delta"`
			}
			if json.Unmarshal([]byte(data), &d) == nil && d.Delta != "" {
				ch <- CompletionEvent{Type: EventTextDelta, Text: d.Delta}
			}

		case "response.output_item.added":
			// Track new function call items
			var item struct {
				OutputIndex int `json:"output_index"`
				Item        struct {
					Type   string `json:"type"`
					ID     string `json:"id"`
					CallID string `json:"call_id"`
					Name   string `json:"name"`
				} `json:"item"`
			}
			if json.Unmarshal([]byte(data), &item) == nil && item.Item.Type == "function_call" {
				funcCalls[item.OutputIndex] = &fcState{
					id:   item.Item.CallID,
					name: item.Item.Name,
				}
			}

		case "response.function_call_arguments.delta":
			var d struct {
				OutputIndex int    `json:"output_index"`
				Delta       string `json:"delta"`
			}
			if json.Unmarshal([]byte(data), &d) == nil {
				if fc, ok := funcCalls[d.OutputIndex]; ok {
					fc.args += d.Delta
				}
			}

		case "response.function_call_arguments.done":
			var d struct {
				OutputIndex int    `json:"output_index"`
				Arguments   string `json:"arguments"`
			}
			if json.Unmarshal([]byte(data), &d) == nil {
				if fc, ok := funcCalls[d.OutputIndex]; ok {
					fc.args = d.Arguments // use the final complete version
				}
			}

		case "response.output_item.done":
			// Emit completed function calls
			var item struct {
				OutputIndex int `json:"output_index"`
				Item        struct {
					Type      string `json:"type"`
					CallID    string `json:"call_id"`
					Name      string `json:"name"`
					Arguments string `json:"arguments"`
				} `json:"item"`
			}
			if json.Unmarshal([]byte(data), &item) == nil && item.Item.Type == "function_call" {
				ch <- CompletionEvent{
					Type: EventToolCall,
					ToolCall: &ToolCallEvent{
						ID:        item.Item.CallID,
						Name:      item.Item.Name,
						Arguments: item.Item.Arguments,
					},
				}
			}

		case "response.completed":
			var completed struct {
				Response struct {
					Status string `json:"status"`
					Usage  struct {
						InputTokens  int `json:"input_tokens"`
						OutputTokens int `json:"output_tokens"`
					} `json:"usage"`
					Output []struct {
						Type string `json:"type"`
					} `json:"output"`
				} `json:"response"`
			}
			if json.Unmarshal([]byte(data), &completed) == nil {
				// Emit usage
				ch <- CompletionEvent{
					Type: EventUsage,
					Usage: &Usage{
						InputTokens:  completed.Response.Usage.InputTokens,
						OutputTokens: completed.Response.Usage.OutputTokens,
					},
				}

				// Determine stop reason
				stopReason := "stop"
				for _, out := range completed.Response.Output {
					if out.Type == "function_call" {
						stopReason = "tool_calls"
						break
					}
				}
				ch <- CompletionEvent{Type: EventDone, StopReason: stopReason}
			}
		}
	}
}

func init() {
	Register("openai-responses", func(apiKeys []string) (Provider, error) {
		if len(apiKeys) == 0 {
			return nil, fmt.Errorf("openai-responses: at least one API key required")
		}
		return NewOpenAIResponses(OpenAIResponsesConfig{
			BaseURL: "https://api.openai.com",
			APIKey:  apiKeys[0],
		}), nil
	})
}

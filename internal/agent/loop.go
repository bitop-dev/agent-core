package agent

import (
	"context"
	"encoding/json"
	"time"

	"github.com/bitop-dev/agent-core/internal/observer"
	"github.com/bitop-dev/agent-core/internal/provider"
	"github.com/bitop-dev/agent-core/internal/tool"
)

// loop is the main agent turn loop.
func (a *Agent) loop(ctx context.Context, mission string, ch chan<- RunEvent) {
	startTime := time.Now()
	totalTurns := 0

	ch <- RunEvent{Type: EventAgentStart}

	// Build system prompt and tool specs once
	systemPrompt := a.buildSystemPrompt()
	toolSpecs := a.buildToolSpecs()

	// Conversation history
	var history []provider.Message

	// First message is the user's mission
	history = append(history, provider.Message{
		Role: provider.RoleUser,
		Content: []provider.ContentBlock{
			{Type: provider.ContentText, Text: mission},
		},
	})

	for {
		totalTurns++
		ch <- RunEvent{Type: EventTurnStart, Data: totalTurns}

		// Check turn limit
		if totalTurns > a.config.MaxTurns {
			ch <- RunEvent{Type: EventAgentEnd, Data: AgentEndData{
				TotalTurns: totalTurns - 1,
				StopReason: "max_turns",
				DurationMs: time.Since(startTime).Milliseconds(),
			}}
			return
		}

		// Check context cancellation
		select {
		case <-ctx.Done():
			ch <- RunEvent{Type: EventAgentEnd, Data: AgentEndData{
				TotalTurns: totalTurns,
				StopReason: "timeout",
				DurationMs: time.Since(startTime).Milliseconds(),
			}}
			return
		default:
		}

		// Call the LLM
		req := provider.CompletionRequest{
			Model:        a.config.Model,
			SystemPrompt: systemPrompt,
			Messages:     history,
			Tools:        toolSpecs,
			MaxTokens:    16384,
		}

		stream, err := a.provider.Complete(ctx, req)
		if err != nil {
			ch <- RunEvent{Type: EventError, Data: err.Error()}
			ch <- RunEvent{Type: EventAgentEnd, Data: AgentEndData{
				TotalTurns: totalTurns,
				StopReason: "error",
				DurationMs: time.Since(startTime).Milliseconds(),
			}}
			return
		}

		// Consume the stream — collect text and tool calls
		var textContent string
		var toolCalls []provider.ContentBlock
		var stopReason string

		ch <- RunEvent{Type: EventMessageStart}

		for event := range stream {
			switch event.Type {
			case provider.EventTextDelta:
				textContent += event.Text
				ch <- RunEvent{Type: EventTextDelta, Data: TextDeltaData{Text: event.Text}}

			case provider.EventThinkingDelta:
				ch <- RunEvent{Type: EventThinkingDelta, Data: TextDeltaData{Text: event.Text}}

			case provider.EventToolCall:
				tc := event.ToolCall
				toolCalls = append(toolCalls, provider.ContentBlock{
					Type:       provider.ContentToolCall,
					ToolCallID: tc.ID,
					ToolName:   tc.Name,
					Arguments:  tc.Arguments,
				})
				ch <- RunEvent{Type: EventToolCallStart, Data: ToolCallStartData{
					ToolCallID: tc.ID,
					ToolName:   tc.Name,
					Arguments:  tc.Arguments,
				}}

			case provider.EventUsage:
				if event.Usage != nil {
					a.observer.OnEvent(observer.Event{
						Type: observer.ObsTokenUsage,
						Payload: event.Usage,
					})
				}

			case provider.EventDone:
				stopReason = event.StopReason

			case provider.EventProviderError:
				ch <- RunEvent{Type: EventError, Data: event.Error.Error()}
				ch <- RunEvent{Type: EventAgentEnd, Data: AgentEndData{
					TotalTurns: totalTurns,
					StopReason: "error",
					DurationMs: time.Since(startTime).Milliseconds(),
				}}
				return
			}
		}

		ch <- RunEvent{Type: EventMessageEnd}

		// Build assistant message for history
		var assistantBlocks []provider.ContentBlock
		if textContent != "" {
			assistantBlocks = append(assistantBlocks, provider.ContentBlock{
				Type: provider.ContentText,
				Text: textContent,
			})
		}
		assistantBlocks = append(assistantBlocks, toolCalls...)
		history = append(history, provider.Message{
			Role:    provider.RoleAssistant,
			Content: assistantBlocks,
		})

		// If no tool calls, agent is done
		if len(toolCalls) == 0 {
			ch <- RunEvent{Type: EventTurnEnd}
			ch <- RunEvent{Type: EventAgentEnd, Data: AgentEndData{
				TotalTurns: totalTurns,
				StopReason: "complete",
				DurationMs: time.Since(startTime).Milliseconds(),
			}}
			return
		}

		// Execute tool calls in parallel
		calls := make([]tool.Call, len(toolCalls))
		for i, tc := range toolCalls {
			calls[i] = tool.Call{
				ID:        tc.ToolCallID,
				Name:      tc.ToolName,
				Arguments: json.RawMessage(tc.Arguments),
			}
		}

		results := a.tools.Dispatch(ctx, calls)

		// Append tool results to history and emit events
		for i, result := range results {
			history = append(history, provider.Message{
				Role: provider.RoleToolResult,
				Content: []provider.ContentBlock{
					{
						Type:       provider.ContentToolResult,
						ToolCallID: calls[i].ID,
						Text:       result.Content,
						IsError:    result.IsError,
					},
				},
			})

			ch <- RunEvent{Type: EventToolCallEnd, Data: ToolCallEndData{
				ToolCallID: calls[i].ID,
				ToolName:   calls[i].Name,
				Content:    result.Content,
				IsError:    result.IsError,
			}}
		}

		ch <- RunEvent{Type: EventTurnEnd}
		_ = stopReason
	}
}

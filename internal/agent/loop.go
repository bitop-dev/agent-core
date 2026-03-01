package agent

import (
	"context"
	"encoding/json"
	"time"

	"github.com/bitop-dev/agent-core/internal/models"
	"github.com/bitop-dev/agent-core/internal/observer"
	"github.com/bitop-dev/agent-core/internal/provider"
	"github.com/bitop-dev/agent-core/internal/tool"
)

// loop is the main agent turn loop. It takes initial history (which must
// end with a user message) and runs until the LLM produces a text response
// with no tool calls, or a limit is hit.
func (a *Agent) loop(ctx context.Context, history []provider.Message, ch chan<- RunEvent) {
	startTime := time.Now()
	totalTurns := 0

	ch <- RunEvent{Type: EventAgentStart}

	systemPrompt := a.buildSystemPrompt()
	toolSpecs := a.buildToolSpecs()

	// Track token usage for proactive compaction
	var lastInputTokens int
	compactionThreshold := a.config.Context.CompactionThreshold
	if compactionThreshold == 0 {
		compactionThreshold = 0.8
	}
	// Get context window from model catalog (fallback to 128K)
	contextWindow := 128000
	if info := models.Get(a.config.Model); info != nil {
		contextWindow = info.ContextWindow
	}

	deferredRetries := 0

	for {
		totalTurns++
		ch <- RunEvent{Type: EventTurnStart, Data: totalTurns}

		// Check turn limit
		if totalTurns > a.config.MaxTurns {
			ch <- RunEvent{Type: EventAgentEnd, Data: AgentEndData{
				TotalTurns: totalTurns - 1,
				StopReason: "max_turns",
				DurationMs: time.Since(startTime).Milliseconds(),
				History:    history,
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
				History:    history,
			}}
			return
		default:
		}

		// Proactive compaction: if context is getting full, summarize older turns
		if shouldCompact(lastInputTokens, contextWindow, compactionThreshold) {
			ch <- RunEvent{Type: EventContextCompact}
			compacted, err := a.compactHistory(ctx, history)
			if err == nil {
				history = compacted
			}
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
			// Reactive compaction: if context window exceeded, compact and retry
			errClass := provider.ClassifyError(err)
			if errClass == provider.ErrorContextFull && len(history) > compactKeepRecent {
				ch <- RunEvent{Type: EventContextCompact}
				compacted, compactErr := a.compactHistory(ctx, history)
				if compactErr == nil {
					history = compacted
					totalTurns-- // don't count this failed attempt
					continue     // retry with compacted history
				}
			}

			ch <- RunEvent{Type: EventError, Data: err.Error()}
			ch <- RunEvent{Type: EventAgentEnd, Data: AgentEndData{
				TotalTurns: totalTurns,
				StopReason: "error",
				DurationMs: time.Since(startTime).Milliseconds(),
				History:    history,
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
					lastInputTokens = event.Usage.InputTokens
					a.observer.OnEvent(observer.Event{
						Type:    observer.ObsTokenUsage,
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
					History:    history,
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

		// If no tool calls, check for deferred-action pattern before finishing.
		// If the LLM says "I'll check X" but didn't emit a tool call, nudge it.
		if len(toolCalls) == 0 {
			if deferredRetries < maxDeferredRetries && LooksLikeDeferredAction(textContent) {
				deferredRetries++
				ch <- RunEvent{Type: EventDeferredAction, Data: textContent}
				history = append(history, provider.Message{
					Role: provider.RoleUser,
					Content: []provider.ContentBlock{{
						Type: provider.ContentText,
						Text: deferredActionRetryPrompt,
					}},
				})
				ch <- RunEvent{Type: EventTurnEnd}
				continue // retry — the LLM should now emit a tool call or give a final answer
			}

			ch <- RunEvent{Type: EventTurnEnd}
			ch <- RunEvent{Type: EventAgentEnd, Data: AgentEndData{
				TotalTurns: totalTurns,
				StopReason: "complete",
				DurationMs: time.Since(startTime).Milliseconds(),
				History:    history,
			}}
			return
		}

		// Reset deferred retry counter when tool calls are made
		deferredRetries = 0

		// Build tool calls and check approval for each
		calls := make([]tool.Call, len(toolCalls))
		for i, tc := range toolCalls {
			calls[i] = tool.Call{
				ID:        tc.ToolCallID,
				Name:      tc.ToolName,
				Arguments: json.RawMessage(tc.Arguments),
			}
		}

		// Approval gate: check each tool call before execution.
		// Denied calls get a synthetic error result; approved calls proceed.
		var approvedCalls []tool.Call
		var deniedResults []tool.Result
		var deniedIndices []int

		for i, call := range calls {
			if a.approval.NeedsApproval(call.Name) {
				ch <- RunEvent{Type: EventApprovalNeeded, Data: ToolCallStartData{
					ToolCallID: call.ID,
					ToolName:   call.Name,
					Arguments:  string(call.Arguments),
				}}
				resp := a.approval.RequestApproval(call.Name, string(call.Arguments))
				if resp == ApprovalNo {
					ch <- RunEvent{Type: EventApprovalDenied, Data: ToolCallStartData{
						ToolCallID: call.ID,
						ToolName:   call.Name,
						Arguments:  string(call.Arguments),
					}}
					deniedResults = append(deniedResults, tool.Result{
						Content: "Tool call denied by user.",
						IsError: true,
					})
					deniedIndices = append(deniedIndices, i)
					continue
				}
			}
			approvedCalls = append(approvedCalls, call)
		}

		// Execute only approved calls
		approvedResults := a.tools.Dispatch(ctx, approvedCalls)

		// Merge results back in order
		results := make([]tool.Result, len(calls))
		approvedIdx := 0
		deniedIdx := 0
		for i := range calls {
			if deniedIdx < len(deniedIndices) && deniedIndices[deniedIdx] == i {
				results[i] = deniedResults[deniedIdx]
				deniedIdx++
			} else {
				results[i] = approvedResults[approvedIdx]
				approvedIdx++
			}
		}

		// Append tool results to history and emit events.
		// Scrub credentials from tool output before it enters LLM context.
		for i, result := range results {
			scrubbedContent := scrubCredentials(result.Content)
			history = append(history, provider.Message{
				Role: provider.RoleToolResult,
				Content: []provider.ContentBlock{
					{
						Type:       provider.ContentToolResult,
						ToolCallID: calls[i].ID,
						Text:       scrubbedContent,
						IsError:    result.IsError,
					},
				},
			})

			ch <- RunEvent{Type: EventToolCallEnd, Data: ToolCallEndData{
				ToolCallID: calls[i].ID,
				ToolName:   calls[i].Name,
				Content:    scrubbedContent,
				IsError:    result.IsError,
			}}
		}

		// Record tool calls for loop detection and check for patterns
		for i, result := range results {
			a.loopDetector.RecordCall(
				calls[i].Name,
				string(calls[i].Arguments),
				result.Content,
				!result.IsError,
			)
		}

		verdict := a.loopDetector.Check()
		switch verdict.Verdict {
		case VerdictInjectWarning:
			// Inject warning as a user message so the LLM sees it
			ch <- RunEvent{Type: EventLoopDetected, Data: verdict.Message}
			history = append(history, provider.Message{
				Role: provider.RoleUser,
				Content: []provider.ContentBlock{{
					Type: provider.ContentText,
					Text: verdict.Message,
				}},
			})
		case VerdictHardStop:
			ch <- RunEvent{Type: EventLoopDetected, Data: verdict.Message}
			ch <- RunEvent{Type: EventTurnEnd}
			ch <- RunEvent{Type: EventAgentEnd, Data: AgentEndData{
				TotalTurns: totalTurns,
				StopReason: "loop_detected",
				DurationMs: time.Since(startTime).Milliseconds(),
				History:    history,
			}}
			return
		}

		// Safety heartbeat: inject reminder if interval reached
		if reminder, due := a.heartbeat.Tick(); due {
			ch <- RunEvent{Type: EventHeartbeat, Data: reminder}
			history = append(history, provider.Message{
				Role: provider.RoleUser,
				Content: []provider.ContentBlock{{
					Type: provider.ContentText,
					Text: reminder,
				}},
			})
		}

		ch <- RunEvent{Type: EventTurnEnd}
		_ = stopReason
	}
}

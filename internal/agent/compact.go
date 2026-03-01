package agent

import (
	"context"
	"fmt"
	"strings"

	"github.com/bitop-dev/agent-core/internal/provider"
)

// Compaction constants (from zeroclaw's history.rs)
const (
	compactKeepRecent    = 20    // always preserve last N messages
	compactMaxSourceChars = 12000 // cap what we send to the summarizer
	compactMaxSummaryChars = 2000 // cap the summary itself
)

// compactHistory summarizes older conversation turns to free up context space.
//
// Algorithm:
//  1. Preserve: last 20 messages (recent context)
//  2. Compact: everything before that
//     a. Build transcript of the "middle" section (capped at 12K chars)
//     b. LLM call: "Summarize this conversation concisely"
//     c. Replace compacted section with a single user message containing the summary
//  3. Never compact if total messages ≤ threshold
//  4. Respect tool message boundaries (never split tool call sequences)
func (a *Agent) compactHistory(ctx context.Context, history []provider.Message) ([]provider.Message, error) {
	if len(history) <= compactKeepRecent {
		return history, nil // nothing to compact
	}

	// Find the split point — keep last N messages, compact the rest
	splitIdx := len(history) - compactKeepRecent

	// Tool boundary guard: never split in the middle of a tool call sequence.
	// Walk the split point forward until we're past any tool_result messages.
	splitIdx = adjustSplitForToolBoundary(history, splitIdx)

	if splitIdx <= 0 {
		return history, nil // nothing left to compact
	}

	// Build transcript of the section to compact
	oldSection := history[:splitIdx]
	transcript := buildTranscript(oldSection, compactMaxSourceChars)

	if len(strings.TrimSpace(transcript)) == 0 {
		return history, nil
	}

	// LLM call to summarize
	summary, err := a.summarize(ctx, transcript)
	if err != nil {
		// If summarization fails, fall back to simple tail truncation
		return history[splitIdx:], nil
	}

	// Cap summary length
	if len(summary) > compactMaxSummaryChars {
		summary = summary[:compactMaxSummaryChars] + "\n[summary truncated]"
	}

	// Build new history: summary message + preserved recent messages
	summaryMsg := provider.Message{
		Role: provider.RoleUser,
		Content: []provider.ContentBlock{
			{
				Type: provider.ContentText,
				Text: fmt.Sprintf("[Previous conversation summary]\n%s", summary),
			},
		},
	}

	newHistory := make([]provider.Message, 0, 1+len(history)-splitIdx)
	newHistory = append(newHistory, summaryMsg)
	newHistory = append(newHistory, history[splitIdx:]...)

	return newHistory, nil
}

// adjustSplitForToolBoundary ensures we never split history in the middle
// of a tool call/result sequence. If the split point lands on a tool_result
// message, walk forward until we're past the tool sequence.
func adjustSplitForToolBoundary(history []provider.Message, splitIdx int) int {
	for splitIdx < len(history) && history[splitIdx].Role == provider.RoleToolResult {
		splitIdx++
	}
	// Also check: if the message just before the split is an assistant with
	// tool calls, we need to include its tool results too — walk forward
	if splitIdx > 0 && splitIdx < len(history) {
		prevMsg := history[splitIdx-1]
		if prevMsg.Role == provider.RoleAssistant && hasToolCalls(prevMsg) {
			// Walk past the tool results that belong to this assistant message
			for splitIdx < len(history) && history[splitIdx].Role == provider.RoleToolResult {
				splitIdx++
			}
		}
	}
	return splitIdx
}

func hasToolCalls(msg provider.Message) bool {
	for _, b := range msg.Content {
		if b.Type == provider.ContentToolCall {
			return true
		}
	}
	return false
}

// buildTranscript converts messages into a readable text format for the summarizer.
func buildTranscript(messages []provider.Message, maxChars int) string {
	var sb strings.Builder
	for _, msg := range messages {
		if sb.Len() >= maxChars {
			break
		}
		role := string(msg.Role)
		for _, block := range msg.Content {
			switch block.Type {
			case provider.ContentText:
				text := block.Text
				remaining := maxChars - sb.Len()
				if len(text) > remaining {
					text = text[:remaining]
				}
				fmt.Fprintf(&sb, "%s: %s\n", role, text)
			case provider.ContentToolCall:
				fmt.Fprintf(&sb, "%s: [called tool %s]\n", role, block.ToolName)
			case provider.ContentToolResult:
				text := block.Text
				if len(text) > 200 {
					text = text[:200] + "..."
				}
				fmt.Fprintf(&sb, "tool_result: %s\n", text)
			}
		}
	}
	return sb.String()
}

// summarize makes an LLM call to produce a concise summary of the transcript.
func (a *Agent) summarize(ctx context.Context, transcript string) (string, error) {
	req := provider.CompletionRequest{
		Model: a.config.Model,
		SystemPrompt: "You are a conversation summarizer. Produce a concise summary of the conversation below. " +
			"Preserve key facts, decisions, and context needed to continue the conversation. " +
			"Be brief but don't lose important details.",
		Messages: []provider.Message{
			{
				Role: provider.RoleUser,
				Content: []provider.ContentBlock{
					{Type: provider.ContentText, Text: "Summarize this conversation:\n\n" + transcript},
				},
			},
		},
		MaxTokens: 1024,
	}

	stream, err := a.provider.Complete(ctx, req)
	if err != nil {
		return "", fmt.Errorf("summarize LLM call: %w", err)
	}

	var summary strings.Builder
	for event := range stream {
		if event.Type == provider.EventTextDelta {
			summary.WriteString(event.Text)
		}
		if event.Type == provider.EventProviderError {
			return "", fmt.Errorf("summarize error: %v", event.Error)
		}
	}

	return summary.String(), nil
}

// shouldCompact checks if the context is getting too full based on
// the last known input token count and the model's context window.
func shouldCompact(inputTokens int, contextWindow int, threshold float64) bool {
	if contextWindow <= 0 || inputTokens <= 0 {
		return false
	}
	usage := float64(inputTokens) / float64(contextWindow)
	return usage >= threshold
}

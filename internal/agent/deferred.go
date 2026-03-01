package agent

import (
	"regexp"
	"strings"
)

// deferredActionRetryPrompt is injected when the LLM says "I'll do X" but
// emits no tool call — nudging it to either emit the call or give a final answer.
const deferredActionRetryPrompt = "Internal correction: your last reply indicated you were about to take an action, but no tool call was emitted. If a tool is needed, emit it now. If no tool is needed, provide the complete final answer now and do not defer action."

// maxDeferredRetries caps how many times we'll retry a deferred-action response
// before accepting it as the final answer.
const maxDeferredRetries = 2

// deferredActionRegex detects English "I'll do X" style promises that suggest
// a tool call should follow.
//
// Matches patterns like:
//   - "Let me check the file."
//   - "I'll run the tests now."
//   - "I am going to search for that."
//   - "We'll verify the output."
var deferredActionRegex = regexp.MustCompile(
	`(?i)\b(` +
		`i'll|i will|` +
		`i am going to|` +
		`let me|let's|let us|` +
		`we'll|we will` +
		`)\b` +
		`[^.!?\n]{0,160}` +
		`\b(` +
		`check|look|search|browse|open|read|write|run|execute|call|` +
		`inspect|analy[sz]e|verify|list|fetch|try|see|continue|` +
		`examine|test|review|scan|query|investigate` +
		`)\b`,
)

// LooksLikeDeferredAction returns true if the text appears to promise an
// action (suggesting a tool call should have been emitted) but wasn't
// accompanied by one.
//
// This is used when the LLM's response has text but no tool calls —
// it might be a deferred-action response that needs a retry nudge.
func LooksLikeDeferredAction(text string) bool {
	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		return false
	}

	// Check English patterns
	if deferredActionRegex.MatchString(trimmed) {
		return true
	}

	return false
}

package agent

import "context"

// loop is the main agent turn loop. It:
// 1. Builds context (system prompt + skills + history)
// 2. Calls the LLM (streaming)
// 3. If tool calls → execute tools (parallel) → append results → loop
// 4. If text response → done
// 5. Checks limits (turns, tokens, loops) each iteration
// 6. Compacts history when context window is nearly full
func (a *Agent) loop(ctx context.Context, mission string, ch chan<- RunEvent) {
	ch <- RunEvent{Type: EventAgentStart}

	// TODO: Implement the turn loop
	// This is the heart of agent-core. See agent-core-deep-dive.md
	// for the full algorithm with Go code samples.

	ch <- RunEvent{Type: EventAgentEnd, Data: AgentEndData{
		StopReason: "not_implemented",
	}}
}

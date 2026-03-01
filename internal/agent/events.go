package agent

// RunEvent is emitted by the agent during execution.
// Consumers (CLI renderer, WebSocket streamer) read from the event channel.
type RunEvent struct {
	Type RunEventType
	Data any
}

// RunEventType identifies what kind of event occurred.
type RunEventType string

const (
	EventAgentStart      RunEventType = "agent_start"
	EventTurnStart       RunEventType = "turn_start"
	EventMessageStart    RunEventType = "message_start"
	EventTextDelta       RunEventType = "text_delta"
	EventThinkingDelta   RunEventType = "thinking_delta"
	EventMessageEnd      RunEventType = "message_end"
	EventToolCallStart   RunEventType = "tool_call_start"
	EventToolCallEnd     RunEventType = "tool_call_end"
	EventTurnEnd         RunEventType = "turn_end"
	EventAgentEnd        RunEventType = "agent_end"
	EventError           RunEventType = "error"
	EventContextCompact  RunEventType = "context_compact"
	EventLoopDetected    RunEventType = "loop_detected"
)

// TextDeltaData carries a streaming text fragment.
type TextDeltaData struct {
	Text string
}

// ToolCallStartData carries info about a tool call beginning.
type ToolCallStartData struct {
	ToolCallID string
	ToolName   string
	Arguments  string // raw JSON
}

// ToolCallEndData carries the result of a tool call.
type ToolCallEndData struct {
	ToolCallID string
	ToolName   string
	Content    string
	IsError    bool
	DurationMs int64
}

// AgentEndData carries final run statistics.
type AgentEndData struct {
	TotalTurns    int
	TotalTokens   int
	TotalCostUSD  float64
	DurationMs    int64
	StopReason    string // "complete" | "max_turns" | "timeout" | "error" | "loop_detected"
}

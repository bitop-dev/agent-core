package output

import (
	"encoding/json"
	"io"

	"github.com/bitop-dev/agent-core/internal/agent"
)

// JSONEvent is the serializable form of a RunEvent.
type JSONEvent struct {
	Type string `json:"type"`
	Data any    `json:"data,omitempty"`
}

// JSONRenderer accumulates all events and writes a single JSON array on Flush.
type JSONRenderer struct {
	out    io.Writer
	events []JSONEvent
}

// NewJSONRenderer creates a JSON renderer that writes to the given stream.
func NewJSONRenderer(out io.Writer) *JSONRenderer {
	return &JSONRenderer{out: out}
}

func (r *JSONRenderer) Render(event agent.RunEvent) {
	r.events = append(r.events, toJSONEvent(event))
}

func (r *JSONRenderer) Flush() {
	enc := json.NewEncoder(r.out)
	enc.SetIndent("", "  ")
	enc.Encode(r.events) //nolint:errcheck
}

// JSONLRenderer writes one JSON object per event, one per line (streaming).
type JSONLRenderer struct {
	out io.Writer
}

// NewJSONLRenderer creates a JSONL renderer that writes to the given stream.
func NewJSONLRenderer(out io.Writer) *JSONLRenderer {
	return &JSONLRenderer{out: out}
}

func (r *JSONLRenderer) Render(event agent.RunEvent) {
	enc := json.NewEncoder(r.out)
	enc.Encode(toJSONEvent(event)) //nolint:errcheck
}

func (r *JSONLRenderer) Flush() {}

// toJSONEvent converts a RunEvent to its serializable form.
func toJSONEvent(event agent.RunEvent) JSONEvent {
	je := JSONEvent{Type: string(event.Type)}

	switch data := event.Data.(type) {
	case agent.TextDeltaData:
		je.Data = map[string]string{"text": data.Text}
	case agent.ToolCallStartData:
		je.Data = map[string]string{
			"tool_call_id": data.ToolCallID,
			"tool_name":    data.ToolName,
			"arguments":    data.Arguments,
		}
	case agent.ToolCallEndData:
		je.Data = map[string]any{
			"tool_call_id": data.ToolCallID,
			"tool_name":    data.ToolName,
			"content":      data.Content,
			"is_error":     data.IsError,
		}
	case agent.AgentEndData:
		je.Data = map[string]any{
			"total_turns": data.TotalTurns,
			"stop_reason": data.StopReason,
			"duration_ms": data.DurationMs,
		}
	case string:
		je.Data = data
	case int:
		je.Data = data
	case nil:
		// no data
	default:
		je.Data = data
	}

	return je
}

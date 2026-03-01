// Package observer defines the Observer interface for telemetry and logging.
// Pattern from zeroclaw: Noop → Log → Cost → Multi (fan-out).
package observer

// Observer receives events during agent execution for logging, metrics, and telemetry.
type Observer interface {
	// OnEvent is called for each significant event during a run.
	OnEvent(event Event)
}

// Event is an observation event with a type and payload.
type Event struct {
	Type    EventType
	Payload any
}

// EventType identifies the kind of observation.
type EventType string

const (
	ObsRunStart       EventType = "run_start"
	ObsRunEnd         EventType = "run_end"
	ObsTurnStart      EventType = "turn_start"
	ObsTurnEnd        EventType = "turn_end"
	ObsLLMRequest     EventType = "llm_request"
	ObsLLMResponse    EventType = "llm_response"
	ObsToolCall       EventType = "tool_call"
	ObsToolResult     EventType = "tool_result"
	ObsContextCompact EventType = "context_compact"
	ObsError          EventType = "error"
	ObsTokenUsage     EventType = "token_usage"
)

// Noop is an observer that does nothing. Used in tests and as default.
type Noop struct{}

func (Noop) OnEvent(Event) {}

// Multi fans out events to multiple observers.
type Multi struct {
	Observers []Observer
}

func (m Multi) OnEvent(e Event) {
	for _, o := range m.Observers {
		o.OnEvent(e)
	}
}

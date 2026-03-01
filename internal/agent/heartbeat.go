package agent

import "fmt"

// SafetyHeartbeat re-injects system constraints into the conversation
// at regular intervals. This prevents long-running agents from drifting
// away from their instructions after many turns and tool calls.
//
// The heartbeat fires every N turns (default: 10). It injects a short
// user-role message reminding the agent of its boundaries.
type SafetyHeartbeat struct {
	interval   int    // turns between heartbeats (0 = disabled)
	turnCount  int    // turns since last heartbeat (or start)
	agentName  string // agent name for the reminder
	constraints string // custom constraints text (optional)
}

// HeartbeatConfig configures the safety heartbeat.
type HeartbeatConfig struct {
	// Interval is the number of turns between heartbeat injections.
	// 0 means disabled. Default: 10.
	Interval int
	// AgentName is used in the reminder message.
	AgentName string
	// Constraints is optional extra text injected with the heartbeat.
	// If empty, a generic reminder is used.
	Constraints string
}

// NewSafetyHeartbeat creates a new heartbeat with the given config.
func NewSafetyHeartbeat(cfg HeartbeatConfig) *SafetyHeartbeat {
	interval := cfg.Interval
	if interval == 0 {
		interval = 10
	}
	return &SafetyHeartbeat{
		interval:    interval,
		agentName:   cfg.AgentName,
		constraints: cfg.Constraints,
	}
}

// Tick increments the turn counter and returns a reminder message if
// the heartbeat interval has been reached. Returns ("", false) if
// no heartbeat is due.
func (h *SafetyHeartbeat) Tick() (string, bool) {
	if h.interval <= 0 {
		return "", false
	}

	h.turnCount++
	if h.turnCount < h.interval {
		return "", false
	}

	h.turnCount = 0 // reset for next interval

	if h.constraints != "" {
		return fmt.Sprintf(
			"[SYSTEM REMINDER — Turn %d checkpoint]\n%s\n"+
				"Stay focused on the original task. Do not deviate from your instructions.",
			h.turnCount+h.interval, h.constraints,
		), true
	}

	return fmt.Sprintf(
		"[SYSTEM REMINDER — Turn %d checkpoint]\n"+
			"You are %s. Stay focused on the original task. "+
			"Do not execute commands or access resources outside your configured scope. "+
			"If you cannot complete the task, explain why and stop.",
		h.turnCount+h.interval, h.agentName,
	), true
}

// Reset clears the turn counter (e.g., after compaction).
func (h *SafetyHeartbeat) Reset() {
	h.turnCount = 0
}

// Disabled returns true if the heartbeat is disabled.
func (h *SafetyHeartbeat) Disabled() bool {
	return h.interval <= 0
}

package agent

import (
	"strings"
	"testing"
)

func TestHeartbeat_FiresAtInterval(t *testing.T) {
	h := NewSafetyHeartbeat(HeartbeatConfig{Interval: 3, AgentName: "test-agent"})

	// Turns 1, 2 should not fire
	for i := 1; i <= 2; i++ {
		if _, fired := h.Tick(); fired {
			t.Errorf("should not fire on turn %d", i)
		}
	}

	// Turn 3 should fire
	msg, fired := h.Tick()
	if !fired {
		t.Fatal("should fire on turn 3")
	}
	if !strings.Contains(msg, "SYSTEM REMINDER") {
		t.Errorf("expected SYSTEM REMINDER, got: %s", msg)
	}
	if !strings.Contains(msg, "test-agent") {
		t.Errorf("expected agent name, got: %s", msg)
	}
}

func TestHeartbeat_ResetsAfterFiring(t *testing.T) {
	h := NewSafetyHeartbeat(HeartbeatConfig{Interval: 2, AgentName: "agent"})

	h.Tick() // 1
	h.Tick() // 2 — fires

	// Should not fire on next tick (turn 1 of new cycle)
	if _, fired := h.Tick(); fired {
		t.Error("should not fire immediately after reset")
	}

	// Turn 2 of new cycle — should fire again
	if _, fired := h.Tick(); !fired {
		t.Error("should fire at next interval")
	}
}

func TestHeartbeat_DisabledWithZeroInterval(t *testing.T) {
	h := NewSafetyHeartbeat(HeartbeatConfig{Interval: -1})
	if !h.Disabled() {
		t.Error("should be disabled with negative interval")
	}
	for i := 0; i < 100; i++ {
		if _, fired := h.Tick(); fired {
			t.Fatal("disabled heartbeat should never fire")
		}
	}
}

func TestHeartbeat_CustomConstraints(t *testing.T) {
	h := NewSafetyHeartbeat(HeartbeatConfig{
		Interval:    1,
		AgentName:   "agent",
		Constraints: "Only modify files in ./src/. Never run rm.",
	})

	msg, fired := h.Tick()
	if !fired {
		t.Fatal("should fire on turn 1 with interval 1")
	}
	if !strings.Contains(msg, "Only modify files") {
		t.Errorf("expected custom constraints, got: %s", msg)
	}
	if !strings.Contains(msg, "Stay focused") {
		t.Errorf("expected stay focused, got: %s", msg)
	}
}

func TestHeartbeat_Reset(t *testing.T) {
	h := NewSafetyHeartbeat(HeartbeatConfig{Interval: 3, AgentName: "agent"})

	h.Tick() // 1
	h.Tick() // 2
	h.Reset()

	// After reset, need 3 more ticks to fire
	if _, fired := h.Tick(); fired {
		t.Error("should not fire after reset + 1 tick")
	}
	if _, fired := h.Tick(); fired {
		t.Error("should not fire after reset + 2 ticks")
	}
	if _, fired := h.Tick(); !fired {
		t.Error("should fire after reset + 3 ticks")
	}
}

func TestHeartbeat_DefaultInterval(t *testing.T) {
	h := NewSafetyHeartbeat(HeartbeatConfig{AgentName: "agent"})
	// Default interval is 10
	for i := 1; i <= 9; i++ {
		if _, fired := h.Tick(); fired {
			t.Errorf("should not fire on turn %d with default interval", i)
		}
	}
	if _, fired := h.Tick(); !fired {
		t.Error("should fire on turn 10 (default interval)")
	}
}

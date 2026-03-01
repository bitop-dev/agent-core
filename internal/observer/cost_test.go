package observer

import (
	"strings"
	"testing"

	"github.com/bitop-dev/agent-core/internal/provider"
)

func TestCostTracker_AccumulatesUsage(t *testing.T) {
	ct := NewCostTracker("gpt-4o")

	ct.OnEvent(Event{Type: ObsTokenUsage, Payload: &provider.Usage{InputTokens: 100, OutputTokens: 50}})
	ct.OnEvent(Event{Type: ObsTokenUsage, Payload: &provider.Usage{InputTokens: 200, OutputTokens: 100}})

	if ct.InputTokens() != 300 {
		t.Errorf("expected 300 input, got %d", ct.InputTokens())
	}
	if ct.OutputTokens() != 150 {
		t.Errorf("expected 150 output, got %d", ct.OutputTokens())
	}
	if ct.TotalTokens() != 450 {
		t.Errorf("expected 450 total, got %d", ct.TotalTokens())
	}
}

func TestCostTracker_CalculatesCost(t *testing.T) {
	ct := NewCostTracker("gpt-4o")

	// 1M input + 1M output should be $2.50 + $10.00 = $12.50
	ct.OnEvent(Event{Type: ObsTokenUsage, Payload: &provider.Usage{
		InputTokens: 1_000_000, OutputTokens: 1_000_000,
	}})

	cost := ct.CostUSD()
	if cost < 12.49 || cost > 12.51 {
		t.Errorf("expected ~$12.50, got $%.4f", cost)
	}
}

func TestCostTracker_UnknownModelNoCost(t *testing.T) {
	ct := NewCostTracker("some-unknown-model")

	ct.OnEvent(Event{Type: ObsTokenUsage, Payload: &provider.Usage{InputTokens: 100, OutputTokens: 50}})

	if ct.CostUSD() != 0 {
		t.Errorf("unknown model should have $0 cost, got $%.4f", ct.CostUSD())
	}
	if ct.TotalTokens() != 150 {
		t.Errorf("tokens should still be tracked: %d", ct.TotalTokens())
	}
}

func TestCostTracker_IgnoresOtherEvents(t *testing.T) {
	ct := NewCostTracker("gpt-4o")

	ct.OnEvent(Event{Type: ObsRunStart, Payload: nil})
	ct.OnEvent(Event{Type: ObsError, Payload: "something"})

	if ct.TotalTokens() != 0 {
		t.Error("should ignore non-usage events")
	}
}

func TestCostTracker_Summary(t *testing.T) {
	ct := NewCostTracker("gpt-4o")

	// Empty
	if ct.Summary() != "" {
		t.Error("empty tracker should return empty summary")
	}

	ct.OnEvent(Event{Type: ObsTokenUsage, Payload: &provider.Usage{InputTokens: 500, OutputTokens: 200}})

	s := ct.Summary()
	if !strings.Contains(s, "700 tokens") {
		t.Errorf("expected total tokens in summary: %s", s)
	}
	if !strings.Contains(s, "500 in") {
		t.Errorf("expected input tokens in summary: %s", s)
	}
	if !strings.Contains(s, "$") {
		t.Errorf("expected cost in summary: %s", s)
	}
}

func TestCostTracker_SummaryNoCost(t *testing.T) {
	ct := NewCostTracker("unknown-model")
	ct.OnEvent(Event{Type: ObsTokenUsage, Payload: &provider.Usage{InputTokens: 100, OutputTokens: 50}})

	s := ct.Summary()
	if strings.Contains(s, "$") {
		t.Errorf("unknown model should not show cost: %s", s)
	}
	if !strings.Contains(s, "150 tokens") {
		t.Errorf("should still show tokens: %s", s)
	}
}

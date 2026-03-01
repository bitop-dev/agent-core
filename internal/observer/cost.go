package observer

import (
	"fmt"
	"sync"

	"github.com/bitop-dev/agent-core/internal/provider"
)

// CostPerToken holds pricing for a model (per 1M tokens).
type CostPerToken struct {
	InputPer1M  float64
	OutputPer1M float64
}

// KnownCosts maps model name prefixes to pricing.
// Prices as of late 2025 — approximate, used for estimation only.
var KnownCosts = map[string]CostPerToken{
	"gpt-4o":           {InputPer1M: 2.50, OutputPer1M: 10.00},
	"gpt-4o-mini":      {InputPer1M: 0.15, OutputPer1M: 0.60},
	"gpt-5":            {InputPer1M: 10.00, OutputPer1M: 30.00},
	"claude-sonnet-4":  {InputPer1M: 3.00, OutputPer1M: 15.00},
	"claude-haiku-3.5": {InputPer1M: 0.80, OutputPer1M: 4.00},
	"claude-opus-4":    {InputPer1M: 15.00, OutputPer1M: 75.00},
}

// CostTracker is an observer that accumulates token usage and estimated cost.
type CostTracker struct {
	mu           sync.Mutex
	model        string
	inputTokens  int
	outputTokens int
	costUSD      float64
	cost         CostPerToken
}

// NewCostTracker creates a cost tracking observer for the given model.
func NewCostTracker(model string) *CostTracker {
	ct := &CostTracker{model: model}

	// Find matching cost by prefix
	for prefix, cost := range KnownCosts {
		if len(model) >= len(prefix) && model[:len(prefix)] == prefix {
			ct.cost = cost
			break
		}
	}

	return ct
}

func (ct *CostTracker) OnEvent(e Event) {
	if e.Type != ObsTokenUsage {
		return
	}

	usage, ok := e.Payload.(*provider.Usage)
	if !ok {
		return
	}

	ct.mu.Lock()
	defer ct.mu.Unlock()

	ct.inputTokens += usage.InputTokens
	ct.outputTokens += usage.OutputTokens

	inputCost := float64(usage.InputTokens) * ct.cost.InputPer1M / 1_000_000
	outputCost := float64(usage.OutputTokens) * ct.cost.OutputPer1M / 1_000_000
	ct.costUSD += inputCost + outputCost
}

// Summary returns a human-readable cost summary.
func (ct *CostTracker) Summary() string {
	ct.mu.Lock()
	defer ct.mu.Unlock()

	if ct.inputTokens == 0 && ct.outputTokens == 0 {
		return ""
	}

	total := ct.inputTokens + ct.outputTokens
	if ct.costUSD > 0 {
		return fmt.Sprintf("%d tokens (%d in + %d out) ≈ $%.4f",
			total, ct.inputTokens, ct.outputTokens, ct.costUSD)
	}
	return fmt.Sprintf("%d tokens (%d in + %d out)",
		total, ct.inputTokens, ct.outputTokens)
}

// TotalTokens returns total input + output tokens.
func (ct *CostTracker) TotalTokens() int {
	ct.mu.Lock()
	defer ct.mu.Unlock()
	return ct.inputTokens + ct.outputTokens
}

// CostUSD returns estimated total cost in USD.
func (ct *CostTracker) CostUSD() float64 {
	ct.mu.Lock()
	defer ct.mu.Unlock()
	return ct.costUSD
}

// InputTokens returns total input tokens.
func (ct *CostTracker) InputTokens() int {
	ct.mu.Lock()
	defer ct.mu.Unlock()
	return ct.inputTokens
}

// OutputTokens returns total output tokens.
func (ct *CostTracker) OutputTokens() int {
	ct.mu.Lock()
	defer ct.mu.Unlock()
	return ct.outputTokens
}

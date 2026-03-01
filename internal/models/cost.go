package models

import "sync"

// CostTracker accumulates token usage and cost across a run.
type CostTracker struct {
	mu           sync.Mutex
	inputTokens  int
	outputTokens int
	costUSD      float64
}

// NewCostTracker creates a zero-value cost tracker.
func NewCostTracker() *CostTracker {
	return &CostTracker{}
}

// Add records token usage for a single LLM call.
func (ct *CostTracker) Add(modelID string, inputTokens, outputTokens int) {
	ct.mu.Lock()
	defer ct.mu.Unlock()

	ct.inputTokens += inputTokens
	ct.outputTokens += outputTokens

	info := Get(modelID)
	if info != nil {
		ct.costUSD += float64(inputTokens) / 1_000_000 * info.InputCostPer1M
		ct.costUSD += float64(outputTokens) / 1_000_000 * info.OutputCostPer1M
	}
}

// Total returns accumulated stats.
func (ct *CostTracker) Total() (inputTokens, outputTokens int, costUSD float64) {
	ct.mu.Lock()
	defer ct.mu.Unlock()
	return ct.inputTokens, ct.outputTokens, ct.costUSD
}

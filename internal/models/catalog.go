// Package models contains the model catalog — context windows, costs, and capabilities.
package models

// ModelInfo describes a known LLM model.
type ModelInfo struct {
	ID            string  // e.g., "claude-sonnet-4-20250514"
	Provider      string  // e.g., "anthropic"
	DisplayName   string  // e.g., "Claude Sonnet 4"
	ContextWindow int     // max tokens
	MaxOutput     int     // max output tokens
	InputCostPer1M  float64 // USD per 1M input tokens
	OutputCostPer1M float64 // USD per 1M output tokens
	SupportsTools   bool
	SupportsVision  bool
	SupportsThinking bool
}

// catalog is the embedded model registry.
// TODO: Populate with real model data.
var catalog = map[string]ModelInfo{
	"claude-sonnet-4-20250514": {
		ID:              "claude-sonnet-4-20250514",
		Provider:        "anthropic",
		DisplayName:     "Claude Sonnet 4",
		ContextWindow:   200000,
		MaxOutput:       8192,
		InputCostPer1M:  3.0,
		OutputCostPer1M: 15.0,
		SupportsTools:   true,
		SupportsVision:  true,
		SupportsThinking: true,
	},
}

// Get returns model info by ID, or nil if not found.
func Get(modelID string) *ModelInfo {
	info, ok := catalog[modelID]
	if !ok {
		return nil
	}
	return &info
}

// All returns all known models.
func All() []ModelInfo {
	result := make([]ModelInfo, 0, len(catalog))
	for _, m := range catalog {
		result = append(result, m)
	}
	return result
}

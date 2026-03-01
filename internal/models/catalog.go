// Package models contains the model catalog — context windows, costs, and capabilities.
package models

// ModelInfo describes a known LLM model.
type ModelInfo struct {
	ID               string  // e.g., "claude-sonnet-4-20250514"
	Provider         string  // e.g., "anthropic"
	DisplayName      string  // e.g., "Claude Sonnet 4"
	ContextWindow    int     // max tokens
	MaxOutput        int     // max output tokens
	InputCostPer1M   float64 // USD per 1M input tokens
	OutputCostPer1M  float64 // USD per 1M output tokens
	SupportsTools    bool
	SupportsVision   bool
	SupportsThinking bool
	IsReasoning      bool // uses max_completion_tokens instead of max_tokens
}

// catalog is the embedded model registry.
var catalog = map[string]ModelInfo{
	// --- Anthropic ---
	"claude-sonnet-4-20250514": {
		ID: "claude-sonnet-4-20250514", Provider: "anthropic", DisplayName: "Claude Sonnet 4",
		ContextWindow: 200000, MaxOutput: 8192,
		InputCostPer1M: 3.0, OutputCostPer1M: 15.0,
		SupportsTools: true, SupportsVision: true, SupportsThinking: true,
	},
	"claude-sonnet-4.6": {
		ID: "claude-sonnet-4.6", Provider: "anthropic", DisplayName: "Claude Sonnet 4.6",
		ContextWindow: 200000, MaxOutput: 16384,
		InputCostPer1M: 3.0, OutputCostPer1M: 15.0,
		SupportsTools: true, SupportsVision: true, SupportsThinking: true,
	},
	"claude-opus-4-20250514": {
		ID: "claude-opus-4-20250514", Provider: "anthropic", DisplayName: "Claude Opus 4",
		ContextWindow: 200000, MaxOutput: 32768,
		InputCostPer1M: 15.0, OutputCostPer1M: 75.0,
		SupportsTools: true, SupportsVision: true, SupportsThinking: true,
	},
	"claude-haiku-3.5": {
		ID: "claude-haiku-3.5", Provider: "anthropic", DisplayName: "Claude Haiku 3.5",
		ContextWindow: 200000, MaxOutput: 8192,
		InputCostPer1M: 0.80, OutputCostPer1M: 4.0,
		SupportsTools: true, SupportsVision: true, SupportsThinking: false,
	},

	// --- OpenAI ---
	"gpt-4o": {
		ID: "gpt-4o", Provider: "openai", DisplayName: "GPT-4o",
		ContextWindow: 128000, MaxOutput: 16384,
		InputCostPer1M: 2.50, OutputCostPer1M: 10.0,
		SupportsTools: true, SupportsVision: true, SupportsThinking: false,
	},
	"gpt-4o-mini": {
		ID: "gpt-4o-mini", Provider: "openai", DisplayName: "GPT-4o Mini",
		ContextWindow: 128000, MaxOutput: 16384,
		InputCostPer1M: 0.15, OutputCostPer1M: 0.60,
		SupportsTools: true, SupportsVision: true, SupportsThinking: false,
	},
	"gpt-5": {
		ID: "gpt-5", Provider: "openai", DisplayName: "GPT-5",
		ContextWindow: 256000, MaxOutput: 32768,
		InputCostPer1M: 10.0, OutputCostPer1M: 30.0,
		SupportsTools: true, SupportsVision: true, SupportsThinking: true, IsReasoning: true,
	},
	"o3": {
		ID: "o3", Provider: "openai", DisplayName: "O3",
		ContextWindow: 200000, MaxOutput: 100000,
		InputCostPer1M: 10.0, OutputCostPer1M: 40.0,
		SupportsTools: true, SupportsVision: true, SupportsThinking: true, IsReasoning: true,
	},
	"o3-mini": {
		ID: "o3-mini", Provider: "openai", DisplayName: "O3 Mini",
		ContextWindow: 200000, MaxOutput: 100000,
		InputCostPer1M: 1.10, OutputCostPer1M: 4.40,
		SupportsTools: true, SupportsVision: false, SupportsThinking: true, IsReasoning: true,
	},
	"o4-mini": {
		ID: "o4-mini", Provider: "openai", DisplayName: "O4 Mini",
		ContextWindow: 200000, MaxOutput: 100000,
		InputCostPer1M: 1.10, OutputCostPer1M: 4.40,
		SupportsTools: true, SupportsVision: true, SupportsThinking: true, IsReasoning: true,
	},

	// --- Open/Local ---
	"llama3.1": {
		ID: "llama3.1", Provider: "ollama", DisplayName: "Llama 3.1 8B",
		ContextWindow: 128000, MaxOutput: 4096,
		InputCostPer1M: 0, OutputCostPer1M: 0, // local
		SupportsTools: true, SupportsVision: false,
	},
	"deepseek-r1": {
		ID: "deepseek-r1", Provider: "openai", DisplayName: "DeepSeek R1",
		ContextWindow: 128000, MaxOutput: 8192,
		InputCostPer1M: 0.55, OutputCostPer1M: 2.19,
		SupportsTools: true, SupportsVision: false, SupportsThinking: true,
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

// ContextWindowFor returns the context window for a model, or a default if unknown.
func ContextWindowFor(modelID string) int {
	if info := Get(modelID); info != nil {
		return info.ContextWindow
	}
	return 128000 // sensible default
}

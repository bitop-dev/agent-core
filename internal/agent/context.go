package agent

import (
	"github.com/bitop-dev/agent-core/internal/provider"
)

// buildSystemPrompt constructs the full system prompt with skill instructions injected.
func (a *Agent) buildSystemPrompt() string {
	prompt := a.config.SystemPrompt
	// TODO: Inject skill instructions from loaded skills
	return prompt
}

// buildToolSpecs converts the tool engine's definitions to provider ToolSpecs.
func (a *Agent) buildToolSpecs() []provider.ToolSpec {
	defs := a.tools.Definitions()
	specs := make([]provider.ToolSpec, len(defs))
	for i, d := range defs {
		specs[i] = provider.ToolSpec{
			Name:        d.Name,
			Description: d.Description,
			InputSchema: string(d.InputSchema),
		}
	}
	return specs
}

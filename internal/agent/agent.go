// Package agent contains the core agent runtime — the turn loop,
// event model, context management, and loop detection.
package agent

import (
	"context"
	"fmt"

	"github.com/bitop-dev/agent-core/internal/config"
	"github.com/bitop-dev/agent-core/internal/observer"
	"github.com/bitop-dev/agent-core/internal/provider"
	sk "github.com/bitop-dev/agent-core/internal/skill"
	"github.com/bitop-dev/agent-core/internal/tool"
)

// Agent is the core runtime. It holds a provider, tool engine, config,
// skills, and observer, and executes the turn loop when Run is called.
type Agent struct {
	config   *config.AgentConfig
	provider provider.Provider
	tools    *tool.Engine
	skills   []*sk.Skill
	observer observer.Observer
}

// Builder constructs an Agent with all dependencies.
type Builder struct {
	config   *config.AgentConfig
	provider provider.Provider
	tools    *tool.Engine
	skills   []*sk.Skill
	observer observer.Observer
}

// NewBuilder creates a new Agent builder.
func NewBuilder() *Builder {
	return &Builder{
		observer: observer.Noop{},
	}
}

func (b *Builder) WithConfig(cfg *config.AgentConfig) *Builder {
	b.config = cfg
	return b
}

func (b *Builder) WithProvider(p provider.Provider) *Builder {
	b.provider = p
	return b
}

func (b *Builder) WithTools(e *tool.Engine) *Builder {
	b.tools = e
	return b
}

func (b *Builder) WithSkills(skills []*sk.Skill) *Builder {
	b.skills = skills
	return b
}

func (b *Builder) WithObserver(o observer.Observer) *Builder {
	b.observer = o
	return b
}

func (b *Builder) Build() (*Agent, error) {
	if b.config == nil {
		return nil, fmt.Errorf("config is required")
	}
	if b.provider == nil {
		return nil, fmt.Errorf("provider is required")
	}
	if b.tools == nil {
		b.tools = tool.NewEngine()
	}
	return &Agent{
		config:   b.config,
		provider: b.provider,
		tools:    b.tools,
		skills:   b.skills,
		observer: b.observer,
	}, nil
}

// Run executes the agent turn loop with the given mission.
// It streams RunEvents through the returned channel.
func (a *Agent) Run(ctx context.Context, mission string) (<-chan RunEvent, error) {
	history := []provider.Message{
		{
			Role:    provider.RoleUser,
			Content: []provider.ContentBlock{{Type: provider.ContentText, Text: mission}},
		},
	}
	return a.RunWithHistory(ctx, history)
}

// RunWithHistory executes the agent turn loop with existing conversation history.
// The last message should be a user message. Returns events via channel.
// Use HistoryFromEvents to extract the updated history after the run completes.
func (a *Agent) RunWithHistory(ctx context.Context, history []provider.Message) (<-chan RunEvent, error) {
	if len(history) == 0 {
		return nil, fmt.Errorf("history must contain at least one message")
	}

	ch := make(chan RunEvent, 64)
	go func() {
		defer close(ch)
		a.loop(ctx, history, ch)
	}()
	return ch, nil
}

// SystemPrompt returns the constructed system prompt (for session persistence).
func (a *Agent) SystemPrompt() string {
	return a.buildSystemPrompt()
}

// ToolSpecs returns tool definitions (for session persistence).
func (a *Agent) ToolSpecs() []provider.ToolSpec {
	return a.buildToolSpecs()
}

// Package agent contains the core agent runtime — the turn loop,
// event model, context management, and loop detection.
package agent

import (
	"context"
	"fmt"

	"github.com/bitop-dev/agent-core/internal/config"
	"github.com/bitop-dev/agent-core/internal/observer"
	"github.com/bitop-dev/agent-core/internal/provider"
	"github.com/bitop-dev/agent-core/internal/tool"
)

// Agent is the core runtime. It holds a provider, tool engine, config,
// and observer, and executes the turn loop when Run is called.
type Agent struct {
	config   *config.AgentConfig
	provider provider.Provider
	tools    *tool.Engine
	observer observer.Observer
}

// Builder constructs an Agent with all dependencies.
type Builder struct {
	config   *config.AgentConfig
	provider provider.Provider
	tools    *tool.Engine
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
		observer: b.observer,
	}, nil
}

// Run executes the agent turn loop with the given mission.
// It streams RunEvents through the returned channel.
func (a *Agent) Run(ctx context.Context, mission string) (<-chan RunEvent, error) {
	ch := make(chan RunEvent, 64)
	go func() {
		defer close(ch)
		a.loop(ctx, mission, ch)
	}()
	return ch, nil
}

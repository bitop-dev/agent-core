// Package agent is the public API for embedding agent-core in other Go programs.
// This is what platform-api imports.
//
//	import "github.com/bitop-dev/agent-core/pkg/agent"
//
// This package provides a clean, stable API surface while keeping
// implementation details in internal/. It re-exports the essential types
// and provides convenience functions for common setup patterns.
package agent

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"

	iagent "github.com/bitop-dev/agent-core/internal/agent"
	"github.com/bitop-dev/agent-core/internal/config"
	"github.com/bitop-dev/agent-core/internal/observer"
	"github.com/bitop-dev/agent-core/internal/provider"
	"github.com/bitop-dev/agent-core/internal/sandbox"
	"github.com/bitop-dev/agent-core/internal/skill"
	"github.com/bitop-dev/agent-core/internal/tool"
	"github.com/bitop-dev/agent-core/internal/tool/builtin"
)

// ─── Re-exported types ───────────────────────────────────────────────────────

// Config is the agent configuration loaded from YAML.
type Config = config.AgentConfig

// RunEvent is emitted during agent execution.
type RunEvent = iagent.RunEvent

// RunEventType identifies what kind of event occurred.
type RunEventType = iagent.RunEventType

// Event type constants.
const (
	EventAgentStart     = iagent.EventAgentStart
	EventTurnStart      = iagent.EventTurnStart
	EventMessageStart   = iagent.EventMessageStart
	EventTextDelta      = iagent.EventTextDelta
	EventThinkingDelta  = iagent.EventThinkingDelta
	EventMessageEnd     = iagent.EventMessageEnd
	EventToolCallStart  = iagent.EventToolCallStart
	EventToolCallEnd    = iagent.EventToolCallEnd
	EventTurnEnd        = iagent.EventTurnEnd
	EventAgentEnd       = iagent.EventAgentEnd
	EventError          = iagent.EventError
	EventContextCompact = iagent.EventContextCompact
	EventLoopDetected   = iagent.EventLoopDetected
	EventApprovalNeeded = iagent.EventApprovalNeeded
	EventApprovalDenied = iagent.EventApprovalDenied
	EventHeartbeat      = iagent.EventHeartbeat
	EventDeferredAction = iagent.EventDeferredAction
)

// Event data types.
type (
	TextDeltaData    = iagent.TextDeltaData
	ToolCallStartData = iagent.ToolCallStartData
	ToolCallEndData  = iagent.ToolCallEndData
	AgentEndData     = iagent.AgentEndData
)

// Provider is the LLM provider interface.
type Provider = provider.Provider

// Message is a conversation message.
type Message = provider.Message

// ToolEngine manages tool registration and dispatch.
type ToolEngine = tool.Engine

// Skill is a loaded skill package.
type Skill = skill.Skill

// Observer receives telemetry events.
type Observer = observer.Observer

// NoopObserver discards all events.
type NoopObserver = observer.Noop

// ─── Config loading ──────────────────────────────────────────────────────────

// LoadConfig reads and parses an agent config from a YAML file.
func LoadConfig(path string) (*Config, error) {
	return config.Load(path)
}

// ParseConfig parses an agent config from YAML bytes.
func ParseConfig(data []byte) (*Config, error) {
	return config.Parse(data)
}

// ─── Agent builder ───────────────────────────────────────────────────────────

// Agent is the core runtime that executes the turn loop.
type Agent struct {
	inner *iagent.Agent
}

// Builder constructs an Agent with all dependencies.
type Builder struct {
	inner *iagent.Builder
}

// NewBuilder creates a new Agent builder.
func NewBuilder() *Builder {
	return &Builder{inner: iagent.NewBuilder()}
}

func (b *Builder) WithConfig(cfg *Config) *Builder {
	b.inner.WithConfig(cfg)
	return b
}

func (b *Builder) WithProvider(p Provider) *Builder {
	b.inner.WithProvider(p)
	return b
}

func (b *Builder) WithTools(e *ToolEngine) *Builder {
	b.inner.WithTools(e)
	return b
}

func (b *Builder) WithSkills(skills []*Skill) *Builder {
	b.inner.WithSkills(skills)
	return b
}

func (b *Builder) WithObserver(o Observer) *Builder {
	b.inner.WithObserver(o)
	return b
}

func (b *Builder) Build() (*Agent, error) {
	inner, err := b.inner.Build()
	if err != nil {
		return nil, err
	}
	return &Agent{inner: inner}, nil
}

// ─── Agent methods ───────────────────────────────────────────────────────────

// Run executes the agent with a mission string.
// Returns a channel of RunEvents. The channel is closed when the agent completes.
func (a *Agent) Run(ctx context.Context, mission string) (<-chan RunEvent, error) {
	return a.inner.Run(ctx, mission)
}

// RunWithHistory executes the agent with existing conversation history.
// The last message should be a user message.
func (a *Agent) RunWithHistory(ctx context.Context, history []Message) (<-chan RunEvent, error) {
	return a.inner.RunWithHistory(ctx, history)
}

// SystemPrompt returns the constructed system prompt.
func (a *Agent) SystemPrompt() string {
	return a.inner.SystemPrompt()
}

// ─── Provider factories ──────────────────────────────────────────────────────

// NewOpenAIProvider creates an OpenAI-compatible provider.
func NewOpenAIProvider(apiKey, baseURL string) Provider {
	return provider.NewOpenAI(provider.OpenAIConfig{
		APIKey:  apiKey,
		BaseURL: baseURL,
	})
}

// NewAnthropicProvider creates an Anthropic provider.
func NewAnthropicProvider(apiKey, baseURL string) Provider {
	return provider.NewAnthropic(provider.AnthropicConfig{
		APIKey:  apiKey,
		BaseURL: baseURL,
	})
}

// NewReliableProvider wraps a provider with retry, backoff, and key rotation.
func NewReliableProvider(p Provider) Provider {
	return provider.NewReliable(p, provider.DefaultReliableConfig())
}

// ─── Tool engine factory ─────────────────────────────────────────────────────

// NewToolEngine creates a tool engine with all built-in tools registered.
func NewToolEngine() *ToolEngine {
	e := tool.NewEngine()
	for _, t := range builtin.All() {
		e.Register(t)
	}
	return e
}

// NewToolEngineWithOptions creates a tool engine with configured built-in tools.
func NewToolEngineWithOptions(opts builtin.BuiltinOptions) *ToolEngine {
	e := tool.NewEngine()
	for _, t := range builtin.AllWithOptions(opts) {
		e.Register(t)
	}
	return e
}

// ─── Skill helpers ───────────────────────────────────────────────────────────

// NewSkill creates a Skill from name, instructions, and optional description.
// Used by platform-api to create skills from DB records without needing local files.
func NewSkill(name, description, instructions string) *Skill {
	return &skill.Skill{
		Name:         name,
		Description:  description,
		Instructions: instructions,
	}
}

// BuildSkillPrompt produces the system prompt fragment for a set of skills.
func BuildSkillPrompt(skills []*Skill) string {
	return skill.BuildSystemPromptFragment(skills)
}

// ParseSkillMD parses a SKILL.md file into a Skill struct.
func ParseSkillMD(data []byte) (*Skill, error) {
	return skill.ParseSkillMD(data)
}

// DefaultSkillDir returns the default local skill installation directory (~/.agent-core/skills/).
func DefaultSkillDir() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".agent-core", "skills")
}

// DefaultSkillSource is the default GitHub skill registry.
const DefaultSkillSource = "github.com/bitop-dev/agent-platform-skills"

// InstallSkill installs a skill from a registry source to a local directory.
func InstallSkill(source, name, destDir string) error {
	return skill.InstallSkill(source, name, destDir)
}

// SandboxRegistry is the runtime registry for tool sandboxing.
type SandboxRegistry = sandbox.Registry

// SandboxCapabilities defines what a sandboxed tool can access.
type SandboxCapabilities = sandbox.Capabilities

// SandboxRuntimeType identifies a sandbox backend.
type SandboxRuntimeType = sandbox.RuntimeType

// Sandbox runtime type constants.
const (
	RuntimeNative    = sandbox.RuntimeNative
	RuntimeWASM      = sandbox.RuntimeWASM
	RuntimeContainer = sandbox.RuntimeContainer
)

// NewSandboxRegistry creates an empty sandbox runtime registry.
func NewSandboxRegistry() *SandboxRegistry {
	return sandbox.NewRegistry()
}

// NewWASMRuntime creates a Wazero-backed WASM runtime.
func NewWASMRuntime(ctx context.Context) (*sandbox.WASMRuntime, error) {
	return sandbox.NewWASMRuntime(ctx)
}

// NewContainerRuntime creates a Docker/Podman-backed container runtime.
func NewContainerRuntime() (*sandbox.ContainerRuntime, error) {
	return sandbox.NewContainerRuntime()
}

// RegisterSkillTools finds and registers WASM skill tools using the sandbox system.
// Tools are dispatched through the appropriate runtime (WASM or container)
// based on the skill's runtime declaration or the default mode.
//
// Parameters:
//   - engine: the tool engine to register tools in
//   - reg: the sandbox runtime registry
//   - names: skill names to load
//   - defaultMode: fallback runtime when skill doesn't declare one ("wasm", "container")
//   - caps: default capabilities for tool execution
//   - dirs: skill directories to search (default: ~/.agent-core/skills/)
func RegisterSkillTools(
	engine *ToolEngine,
	reg *SandboxRegistry,
	names []string,
	defaultMode string,
	caps SandboxCapabilities,
	dirs ...string,
) []*Skill {
	if len(names) == 0 {
		return nil
	}
	if len(dirs) == 0 {
		dirs = []string{DefaultSkillDir()}
	}

	loader := skill.NewLoader(dirs...)
	loaded, _ := loader.LoadByName(names)

	for _, sk := range loaded {
		for _, td := range sk.Tools {
			var module string
			var rt sandbox.RuntimeType

			if sk.Runtime == "container" && sk.Image != "" {
				module = sk.Image
				rt = sandbox.RuntimeContainer
			} else {
				execPath, execType := skill.FindToolExec(sk.Dir, td.Name)
				if execPath == "" {
					continue
				}
				module = execPath
				rt = resolveRuntime(sk.Runtime, execType, defaultMode)
			}

			workDir := "."
			if rt == sandbox.RuntimeContainer {
				workDir = "/tmp"
			}

			st := tool.NewSandboxedTool(tool.SandboxedToolConfig{
				Def: tool.Definition{
					Name:        td.Name,
					Description: td.Description,
					InputSchema: json.RawMessage(td.Parameters),
				},
				Runtime:  rt,
				Module:   module,
				WorkDir:  workDir,
				Caps:     caps,
				Registry: reg,
			})
			engine.Register(st)
		}
	}

	return loaded
}

// resolveRuntime picks the sandbox runtime for a tool.
// Priority: skill-level declaration > exec type detection > agent default mode.
func resolveRuntime(skillRuntime, execType, defaultMode string) sandbox.RuntimeType {
	// Skill explicitly declares runtime
	switch skillRuntime {
	case "wasm":
		return sandbox.RuntimeWASM
	case "container":
		return sandbox.RuntimeContainer
	}

	// Auto-detect from executable type
	if execType == "wasm" {
		return sandbox.RuntimeWASM
	}

	// Fall back to agent default
	switch defaultMode {
	case "container":
		return sandbox.RuntimeContainer
	}

	// Default: WASM
	return sandbox.RuntimeWASM
}

// ─── Quick run ───────────────────────────────────────────────────────────────

// QuickRun is a convenience function for running a one-shot agent.
// It creates a minimal agent with the given provider and model, runs the mission,
// and collects all text output into a string.
func QuickRun(ctx context.Context, p Provider, model, mission string) (string, error) {
	cfg := &Config{
		Name:    "quick",
		Model:   model,
		MaxTurns: 20,
		TimeoutSeconds: 300,
	}

	engine := NewToolEngine()

	a, err := NewBuilder().
		WithConfig(cfg).
		WithProvider(p).
		WithTools(engine).
		Build()
	if err != nil {
		return "", err
	}

	events, err := a.Run(ctx, mission)
	if err != nil {
		return "", err
	}

	var text string
	for event := range events {
		if event.Type == EventTextDelta {
			if data, ok := event.Data.(TextDeltaData); ok {
				text += data.Text
			}
		}
	}
	return text, nil
}

package builtin

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/bitop-dev/agent-core/internal/config"
	"github.com/bitop-dev/agent-core/internal/tool"
)

// AgentSpawnDeps holds dependencies injected from the agent builder.
// This avoids circular imports between tool and agent packages.
type AgentSpawnDeps struct {
	// RunSubAgent creates and runs a sub-agent with the given config and mission.
	// It returns the collected text output and an error.
	RunSubAgent func(ctx context.Context, cfg *config.AgentConfig, mission string) (string, error)
	// CurrentDepth tracks nesting level to enforce limits.
	CurrentDepth int
	// MaxDepth is the maximum nesting allowed (default 3).
	MaxDepth int
}

var agentSpawnDeps *AgentSpawnDeps

// SetAgentSpawnDeps injects the dependencies for agent_spawn.
// Called by the agent builder before the loop starts.
func SetAgentSpawnDeps(deps *AgentSpawnDeps) {
	agentSpawnDeps = deps
}

type agentSpawnTool struct{}

func (t *agentSpawnTool) Definition() tool.Definition {
	return tool.Definition{
		Name:        "agent_spawn",
		Description: "Spawn a sub-agent to perform a specific task. Call this tool DIRECTLY (do NOT use bash). The sub-agent runs with its own conversation loop and returns its text output. Use this to delegate complex sub-tasks to specialist agents.",
		InputSchema: json.RawMessage(`{
			"type": "object",
			"properties": {
				"name": {
					"type": "string",
					"description": "Name for the sub-agent (for logging)"
				},
				"mission": {
					"type": "string",
					"description": "The task/mission for the sub-agent to complete"
				},
				"system_prompt": {
					"type": "string",
					"description": "Optional system prompt override for the sub-agent"
				},
				"max_turns": {
					"type": "integer",
					"description": "Max turns for the sub-agent (default: 10)"
				}
			},
			"required": ["name", "mission"]
		}`),
	}
}

func (t *agentSpawnTool) Execute(ctx context.Context, input json.RawMessage) (tool.Result, error) {
	if agentSpawnDeps == nil || agentSpawnDeps.RunSubAgent == nil {
		return tool.Result{Content: "agent_spawn is not available in this context", IsError: true}, nil
	}

	var args struct {
		Name         string `json:"name"`
		Mission      string `json:"mission"`
		SystemPrompt string `json:"system_prompt"`
		MaxTurns     int    `json:"max_turns"`
	}
	if err := json.Unmarshal(input, &args); err != nil {
		return tool.Result{Content: fmt.Sprintf("invalid arguments: %v", err), IsError: true}, nil
	}

	if args.Mission == "" {
		return tool.Result{Content: "mission is required", IsError: true}, nil
	}

	// Check depth limit
	depth := agentSpawnDeps.CurrentDepth + 1
	maxDepth := agentSpawnDeps.MaxDepth
	if maxDepth <= 0 {
		maxDepth = 3
	}
	if depth > maxDepth {
		return tool.Result{
			Content: fmt.Sprintf("max agent nesting depth reached (%d/%d)", depth, maxDepth),
			IsError: true,
		}, nil
	}

	maxTurns := args.MaxTurns
	if maxTurns <= 0 {
		maxTurns = 10
	}

	// Build sub-agent config
	subCfg := &config.AgentConfig{
		Name:           args.Name,
		Model:          "", // inherited from parent via RunSubAgent
		MaxTurns:       maxTurns,
		TimeoutSeconds: 120,
	}
	if args.SystemPrompt != "" {
		subCfg.SystemPrompt = args.SystemPrompt
	}

	// Run with a timeout
	subCtx, cancel := context.WithTimeout(ctx, time.Duration(subCfg.TimeoutSeconds)*time.Second)
	defer cancel()

	startTime := time.Now()
	output, err := agentSpawnDeps.RunSubAgent(subCtx, subCfg, args.Mission)
	elapsed := time.Since(startTime)

	if err != nil {
		return tool.Result{
			Content: fmt.Sprintf("sub-agent %q failed after %s: %v", args.Name, elapsed.Round(time.Millisecond), err),
			IsError: true,
		}, nil
	}

	// Truncate very long outputs
	if len(output) > 50000 {
		output = output[:50000] + "\n... (output truncated at 50K chars)"
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Sub-agent %q completed in %s:\n\n", args.Name, elapsed.Round(time.Millisecond)))
	sb.WriteString(output)
	return tool.Result{Content: sb.String()}, nil
}

func newAgentSpawn() tool.Tool {
	return &agentSpawnTool{}
}

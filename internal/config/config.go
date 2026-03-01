// Package config handles agent YAML configuration loading and validation.
package config

// Version is set at build time via ldflags.
var Version = "dev"

// AgentConfig is the top-level configuration for an agent.
type AgentConfig struct {
	Name           string            `yaml:"name"`
	Description    string            `yaml:"description"`
	Provider       string            `yaml:"provider"`
	Model          string            `yaml:"model"`
	SystemPrompt   string            `yaml:"system_prompt"`
	DefaultMission string            `yaml:"default_mission"`
	Skills         []SkillRef        `yaml:"skills"`
	Tools          ToolsConfig       `yaml:"tools"`
	MCP            MCPConfig         `yaml:"mcp"`
	MaxTurns       int               `yaml:"max_turns"`
	TimeoutSeconds int               `yaml:"timeout_seconds"`
	MaxTokensTotal int               `yaml:"max_tokens_total"`
	Context        ContextConfig     `yaml:"context"`
	LoopDetection  LoopDetection     `yaml:"loop_detection"`
	Heartbeat      HeartbeatConfig   `yaml:"heartbeat"`
	Approval       ApprovalConfig    `yaml:"approval"`
	Output         OutputConfig      `yaml:"output"`
}

// SkillRef is either a string (skill name) or a map (skill name → config).
// Parsed from YAML where skills can be:
//   - web_search           (string, default config)
//   - web_search:          (map, with config)
//       backend: ddg
type SkillRef struct {
	Name   string
	Config map[string]any
}

// ToolsConfig defines which core tools are enabled and their settings.
type ToolsConfig struct {
	Core map[string]map[string]any `yaml:"core"`
}

// MCPConfig defines external MCP servers.
type MCPConfig struct {
	Servers []MCPServer `yaml:"servers"`
}

// MCPServer is a single MCP server connection.
type MCPServer struct {
	Name           string            `yaml:"name"`
	Transport      string            `yaml:"transport"` // stdio | http | sse | streamable-http
	Command        []string          `yaml:"command"`
	URL            string            `yaml:"url"`
	Headers        map[string]string `yaml:"headers"`
	TimeoutSeconds int               `yaml:"timeout_seconds"`
}

// ContextConfig controls context window management.
type ContextConfig struct {
	Strategy            string  `yaml:"strategy"` // tail_window | compaction
	TailWindowSize      int     `yaml:"tail_window_size"`
	CompactionThreshold float64 `yaml:"compaction_threshold"`
}

// LoopDetection settings.
type LoopDetection struct {
	NoProgressThreshold  int `yaml:"no_progress_threshold"`
	PingPongCycles       int `yaml:"ping_pong_cycles"`
	FailureStreakThreshold int `yaml:"failure_streak_threshold"`
}

// HeartbeatConfig for safety re-injection.
type HeartbeatConfig struct {
	Enabled  bool `yaml:"enabled"`
	Interval int  `yaml:"interval"`
}

// ApprovalConfig for human-in-the-loop tool approval.
type ApprovalConfig struct {
	Policy        string   `yaml:"policy"` // never | always | once | list
	RequiredTools []string `yaml:"required_tools"`
}

// OutputConfig controls how results are rendered.
type OutputConfig struct {
	Stream   bool   `yaml:"stream"`
	Format   string `yaml:"format"`   // text | json | jsonl
	Thinking bool   `yaml:"thinking"`
	Color    string `yaml:"color"`    // auto | always | never
	LogFile  string `yaml:"log_file"`
}

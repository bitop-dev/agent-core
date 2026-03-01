package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/bitop-dev/agent-core/internal/agent"
	"github.com/bitop-dev/agent-core/internal/config"
	"github.com/bitop-dev/agent-core/internal/mcp"
	"github.com/bitop-dev/agent-core/internal/observer"
	"github.com/bitop-dev/agent-core/internal/output"
	"github.com/bitop-dev/agent-core/internal/provider"
	"github.com/bitop-dev/agent-core/internal/tool"
	"github.com/bitop-dev/agent-core/internal/tool/builtin"
)

func main() {
	if err := rootCmd().Execute(); err != nil {
		os.Exit(1)
	}
}

func rootCmd() *cobra.Command {
	root := &cobra.Command{
		Use:   "agent-core",
		Short: "Run AI agents from the command line",
		Long:  "agent-core is a standalone binary for running autonomous AI agents.",
	}

	root.AddCommand(runCmd())
	root.AddCommand(chatCmd())
	root.AddCommand(skillCmd())
	root.AddCommand(sessionsCmd())
	root.AddCommand(toolsCmd())
	root.AddCommand(modelsCmd())
	root.AddCommand(validateCmd())
	root.AddCommand(versionCmd())
	root.AddCommand(mcpCmd())

	return root
}

// --- run ---

func runCmd() *cobra.Command {
	var (
		configPath   string
		mission      string
		modelFlag    string
		providerFlag string
		baseURL      string
		apiKey       string
		formatFlag   string
		approveFlag  bool
	)

	cmd := &cobra.Command{
		Use:   "run",
		Short: "Run an agent with a mission",
		RunE: func(cmd *cobra.Command, args []string) error {
			// Load config
			var cfg *config.AgentConfig
			var err error
			if configPath != "" {
				cfg, err = config.Load(configPath)
				if err != nil {
					return fmt.Errorf("load config: %w", err)
				}
			} else {
				cfg = &config.AgentConfig{
					Provider:       "openai",
					Model:          "gpt-4o",
					MaxTurns:       20,
					TimeoutSeconds: 300,
				}
			}

			// Override from flags
			if modelFlag != "" {
				cfg.Model = modelFlag
			}
			if providerFlag != "" {
				cfg.Provider = providerFlag
			}

			// Resolve mission
			if mission == "" && len(args) > 0 {
				mission = strings.Join(args, " ")
			}
			if mission == "" && cfg.DefaultMission != "" {
				mission = cfg.DefaultMission
			}
			if mission == "" {
				return fmt.Errorf("mission is required (use --mission or pass as argument)")
			}

			// Resolve API key
			key := apiKey
			if key == "" {
				key = os.Getenv("OPENAI_API_KEY")
			}
			if key == "" {
				key = os.Getenv("ANTHROPIC_API_KEY")
			}
			if key == "" {
				key = os.Getenv("AGENT_CORE_API_KEY")
			}
			if key == "" {
				return fmt.Errorf("API key required (--api-key, OPENAI_API_KEY, ANTHROPIC_API_KEY, or AGENT_CORE_API_KEY)")
			}

			// Resolve base URL
			url := baseURL
			if url == "" {
				url = os.Getenv("OPENAI_BASE_URL")
			}
			if url == "" {
				url = os.Getenv("AGENT_CORE_BASE_URL")
			}

			// Auto-detect provider from model name if not explicitly set
			provName := cfg.Provider
			if provName == "" {
				provName = detectProvider(cfg.Model)
			}

			// Create provider
			p := createProvider(provName, key, url)

			// Create tool engine with built-in tools
			engine := tool.NewEngine()
			registerBuiltins(engine, cfg)

			// Initialize sandbox runtimes
			cleanupSandbox := initSandboxRegistry(cfg)
			defer cleanupSandbox()

			// Load skills
			skills, err := loadSkills(cfg, engine)
			if err != nil {
				return fmt.Errorf("load skills: %w", err)
			}

			// Connect MCP servers
			if len(cfg.MCP.Servers) > 0 {
				mcpClients, mcpErrs := mcp.RegisterAll(cfg.MCP.Servers, engine)
				for _, e := range mcpErrs {
					fmt.Fprintf(os.Stderr, "\033[33m⚠ %v\033[0m\n", e)
				}
				// Close MCP clients when done
				defer func() {
					for _, c := range mcpClients {
						c.Close()
					}
				}()
				if len(mcpClients) > 0 {
					total := 0
					for _, c := range mcpClients {
						total += len(c.Tools())
					}
					fmt.Fprintf(os.Stderr, "\033[90m✓ %d MCP server(s) connected, %d tool(s)\033[0m\n", len(mcpClients), total)
				}
			}

			// Wrap provider with reliability layer
			reliableCfg := provider.DefaultReliableConfig()
			rp := provider.NewReliable(p, reliableCfg)

			// Set up cost tracking
			costTracker := observer.NewCostTracker(cfg.Model)

			// Build agent
			builder := agent.NewBuilder().
				WithConfig(cfg).
				WithProvider(rp).
				WithTools(engine).
				WithSkills(skills).
				WithObserver(costTracker)

			// Wire approval mode
			if approveFlag || cfg.Approval.Policy == "always" || cfg.Approval.Policy == "list" {
				approvalCfg := agent.ApprovalConfig{
					Mode:      agent.ApprovalSupervised,
					AlwaysAsk: cfg.Approval.RequiredTools,
				}
				// Auto-approve read-only tools by default
				approvalCfg.AutoApprove = []string{"read_file", "list_dir", "grep", "tasks"}
				builder = builder.WithApproval(approvalCfg)
			}

			a, err := builder.Build()
			if err != nil {
				return fmt.Errorf("build agent: %w", err)
			}

			// Run with signal handling
			ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
			defer cancel()

			if cfg.TimeoutSeconds > 0 {
				var timeoutCancel context.CancelFunc
				ctx, timeoutCancel = context.WithTimeout(ctx, time.Duration(cfg.TimeoutSeconds)*time.Second)
				defer timeoutCancel()
			}

			events, err := a.Run(ctx, mission)
			if err != nil {
				return fmt.Errorf("run agent: %w", err)
			}

			// Select renderer based on format
			format := formatFlag
			if format == "" {
				format = cfg.Output.Format
			}
			if format == "" {
				format = "text"
			}

			renderer := createRenderer(format)

			for event := range events {
				renderer.Render(event)
			}
			renderer.Flush()

			// Show cost summary (text mode only)
			if format == "text" {
				if summary := costTracker.Summary(); summary != "" {
					fmt.Fprintf(os.Stderr, "\033[90m    %s\033[0m\n", summary)
				}
			}
			return nil
		},
	}

	cmd.Flags().StringVarP(&configPath, "config", "c", "", "Path to agent YAML config")
	cmd.Flags().StringVarP(&mission, "mission", "m", "", "Mission for the agent")
	cmd.Flags().StringVar(&modelFlag, "model", "", "Override model from config")
	cmd.Flags().StringVarP(&providerFlag, "provider", "p", "", "Provider: openai, anthropic, ollama, openai-responses (auto-detected from model)")
	cmd.Flags().StringVar(&baseURL, "base-url", "", "Override API base URL")
	cmd.Flags().StringVar(&apiKey, "api-key", "", "API key (or set OPENAI_API_KEY / ANTHROPIC_API_KEY)")
	cmd.Flags().StringVarP(&formatFlag, "format", "f", "", "Output format: text (default), json, jsonl")
	cmd.Flags().BoolVar(&approveFlag, "approve", false, "Enable approval prompts for dangerous tools (bash, write_file, etc.)")

	return cmd
}

// detectProvider guesses the provider from the model name.
func detectProvider(model string) string {
	m := strings.ToLower(model)
	if strings.Contains(m, "claude") || strings.Contains(m, "sonnet") || strings.Contains(m, "opus") || strings.Contains(m, "haiku") {
		return "anthropic"
	}
	if strings.Contains(m, "llama") || strings.Contains(m, "mistral") || strings.Contains(m, "gemma") ||
		strings.Contains(m, "phi") || strings.Contains(m, "qwen") || strings.Contains(m, "deepseek") ||
		strings.Contains(m, "codestral") || strings.Contains(m, "starcoder") {
		return "ollama"
	}
	return "openai" // default: OpenAI-compatible
}

// createProvider builds the right provider based on name, key, and optional base URL.
func createProvider(name, apiKey, baseURL string) provider.Provider {
	switch name {
	case "anthropic":
		url := baseURL
		if url == "" {
			url = os.Getenv("ANTHROPIC_BASE_URL")
		}
		if url == "" {
			url = "https://api.anthropic.com"
		}
		return provider.NewAnthropic(provider.AnthropicConfig{
			BaseURL: url,
			APIKey:  apiKey,
		})
	case "openai-responses":
		url := baseURL
		if url == "" {
			url = "https://api.openai.com"
		}
		return provider.NewOpenAIResponses(provider.OpenAIResponsesConfig{
			BaseURL: url,
			APIKey:  apiKey,
		})
	case "ollama":
		url := baseURL
		if url == "" {
			url = os.Getenv("OLLAMA_BASE_URL")
		}
		if url == "" {
			url = "http://localhost:11434/v1"
		}
		// Ollama doesn't require an API key but the OpenAI client needs one
		key := apiKey
		if key == "" {
			key = "ollama"
		}
		return provider.NewOpenAI(provider.OpenAIConfig{
			BaseURL: url,
			APIKey:  key,
		})
	default: // "openai" and anything else
		url := baseURL
		if url == "" {
			url = "https://api.openai.com/v1"
		}
		return provider.NewOpenAI(provider.OpenAIConfig{
			BaseURL: url,
			APIKey:  apiKey,
		})
	}
}

// registerBuiltins adds core tools to the engine based on config.
// If no tools are configured, all tools are registered (bash is opt-out = on by default).
// If tools.core is specified, only those tools are registered.
// Sandbox settings are parsed from tool configs (allowed_paths, denied_paths, etc.)
func registerBuiltins(engine *tool.Engine, cfg *config.AgentConfig) {
	// Build sandbox policy from config
	sandbox := buildSandboxPolicy(cfg)

	opts := builtin.BuiltinOptions{
		TaskStore: builtin.NewTaskStore(),
	}
	if sandbox != nil {
		opts.Sandbox = sandbox
		engine.Sandbox = *sandbox
	}

	// Parse working dir from bash config
	if bashCfg, ok := cfg.Tools.Core["bash"]; ok {
		if wd, ok := bashCfg["working_dir"].(string); ok {
			opts.WorkingDir = wd
		}
	}

	allTools := builtin.ByNameWithOptions(opts)

	if len(cfg.Tools.Core) == 0 {
		for _, t := range allTools {
			engine.Register(t)
		}
		return
	}

	for name := range cfg.Tools.Core {
		// Check for explicit disable
		if toolCfg, ok := cfg.Tools.Core[name]; ok {
			if enabled, ok := toolCfg["enabled"].(bool); ok && !enabled {
				continue
			}
		}
		if t, ok := allTools[name]; ok {
			engine.Register(t)
		}
	}
}

// buildSandboxPolicy creates a SandboxPolicy from the agent config.
// Returns nil if no sandbox settings are configured.
func buildSandboxPolicy(cfg *config.AgentConfig) *tool.SandboxPolicy {
	var hasSettings bool
	p := tool.DefaultSandboxPolicy()

	// Check for global sandbox settings in any tool config
	for _, toolCfg := range cfg.Tools.Core {
		if paths, ok := toolCfg["allowed_paths"]; ok {
			if list, ok := toStringSlice(paths); ok {
				p.AllowedPaths = list
				hasSettings = true
			}
		}
		if paths, ok := toolCfg["denied_paths"]; ok {
			if list, ok := toStringSlice(paths); ok {
				p.DeniedPaths = list
				hasSettings = true
			}
		}
		if keys, ok := toolCfg["allowed_env"]; ok {
			if list, ok := toStringSlice(keys); ok {
				p.AllowedEnvKeys = list
				hasSettings = true
			}
		}
		if maxBytes, ok := toolCfg["max_output_bytes"]; ok {
			if v, ok := maxBytes.(int); ok {
				p.MaxOutputBytes = v
				hasSettings = true
			}
		}
		if timeout, ok := toolCfg["timeout_seconds"]; ok {
			if v, ok := timeout.(int); ok {
				p.DefaultTimeoutSec = v
				hasSettings = true
			}
		}
	}

	if !hasSettings {
		return nil
	}
	return &p
}

// toStringSlice converts an any (from YAML) to []string.
func toStringSlice(v any) ([]string, bool) {
	switch val := v.(type) {
	case []any:
		result := make([]string, 0, len(val))
		for _, item := range val {
			if s, ok := item.(string); ok {
				result = append(result, s)
			}
		}
		return result, len(result) > 0
	case []string:
		return val, len(val) > 0
	default:
		return nil, false
	}
}

// createRenderer builds the appropriate renderer for the format.
func createRenderer(format string) output.Renderer {
	switch format {
	case "json":
		return output.NewJSONRenderer(os.Stdout)
	case "jsonl":
		return output.NewJSONLRenderer(os.Stdout)
	default:
		return output.NewTextRenderer(os.Stdout, os.Stderr)
	}
}

// --- version ---

func versionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Show version info",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Printf("agent-core %s\n", config.Version)
		},
	}
}

// --- tools ---

func toolsCmd() *cobra.Command {
	var configPath string

	cmd := &cobra.Command{
		Use:   "tools",
		Short: "List available tools",
		RunE: func(cmd *cobra.Command, args []string) error {
			var cfg *config.AgentConfig
			if configPath != "" {
				var err error
				cfg, err = config.Load(configPath)
				if err != nil {
					return err
				}
			} else {
				cfg = &config.AgentConfig{}
			}

			engine := tool.NewEngine()
			registerBuiltins(engine, cfg)

			fmt.Println("Available tools:")
			for _, def := range engine.Definitions() {
				fmt.Printf("  %-15s %s\n", def.Name, def.Description)
			}
			return nil
		},
	}
	cmd.Flags().StringVarP(&configPath, "config", "c", "", "Agent config file")
	return cmd
}

// --- models ---

func modelsCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "models",
		Short: "List known models",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Println("Use any model supported by the configured provider.")
			fmt.Println("Pass --model <name> to the run command.")
		},
	}
}

// --- validate ---

func validateCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "validate [config-file]",
		Short: "Validate an agent config file",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load(args[0])
			if err != nil {
				return err
			}
			if err := cfg.Validate(); err != nil {
				return err
			}
			fmt.Printf("✓ %s is valid (agent: %s, provider: %s, model: %s)\n",
				args[0], cfg.Name, cfg.Provider, cfg.Model)
			return nil
		},
	}
}

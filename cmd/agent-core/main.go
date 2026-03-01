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
	"github.com/bitop-dev/agent-core/internal/observer"
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
	root.AddCommand(versionCmd())
	root.AddCommand(toolsCmd())
	root.AddCommand(modelsCmd())
	root.AddCommand(validateCmd())

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

			// Build agent
			a, err := agent.NewBuilder().
				WithConfig(cfg).
				WithProvider(p).
				WithTools(engine).
				WithObserver(observer.Noop{}).
				Build()
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

			// Render events to terminal
			return renderEvents(events)
		},
	}

	cmd.Flags().StringVarP(&configPath, "config", "c", "", "Path to agent YAML config")
	cmd.Flags().StringVarP(&mission, "mission", "m", "", "Mission for the agent")
	cmd.Flags().StringVar(&modelFlag, "model", "", "Override model from config")
	cmd.Flags().StringVarP(&providerFlag, "provider", "p", "", "Provider: openai or anthropic (auto-detected from model)")
	cmd.Flags().StringVar(&baseURL, "base-url", "", "Override API base URL")
	cmd.Flags().StringVar(&apiKey, "api-key", "", "API key (or set OPENAI_API_KEY / ANTHROPIC_API_KEY)")

	return cmd
}

// detectProvider guesses the provider from the model name.
func detectProvider(model string) string {
	m := strings.ToLower(model)
	if strings.Contains(m, "claude") || strings.Contains(m, "sonnet") || strings.Contains(m, "opus") || strings.Contains(m, "haiku") {
		return "anthropic"
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
// If no tools are configured, all tools except bash are registered.
// If tools.core is specified, only those tools are registered.
// bash is opt-out: included by default unless tools.core is set and bash is excluded.
func registerBuiltins(engine *tool.Engine, cfg *config.AgentConfig) {
	allTools := builtin.ByName()

	if len(cfg.Tools.Core) == 0 {
		// No tool config — register everything (bash opt-out means it's on by default)
		for _, t := range allTools {
			engine.Register(t)
		}
		return
	}

	// Register only configured tools
	for name := range cfg.Tools.Core {
		if t, ok := allTools[name]; ok {
			engine.Register(t)
		}
	}
}

// renderEvents prints agent events to the terminal.
func renderEvents(events <-chan agent.RunEvent) error {
	for event := range events {
		switch event.Type {
		case agent.EventTextDelta:
			if data, ok := event.Data.(agent.TextDeltaData); ok {
				fmt.Print(data.Text)
			}

		case agent.EventToolCallStart:
			if data, ok := event.Data.(agent.ToolCallStartData); ok {
				fmt.Fprintf(os.Stderr, "\n\033[36m⚙ %s\033[0m(%s)\n", data.ToolName, truncate(data.Arguments, 100))
			}

		case agent.EventToolCallEnd:
			if data, ok := event.Data.(agent.ToolCallEndData); ok {
				if data.IsError {
					fmt.Fprintf(os.Stderr, "\033[31m✗ %s: %s\033[0m\n", data.ToolName, truncate(data.Content, 200))
				} else {
					fmt.Fprintf(os.Stderr, "\033[32m✓ %s\033[0m (%s)\n", data.ToolName, truncate(data.Content, 100))
				}
			}

		case agent.EventError:
			fmt.Fprintf(os.Stderr, "\033[31merror: %v\033[0m\n", event.Data)

		case agent.EventAgentEnd:
			if data, ok := event.Data.(agent.AgentEndData); ok {
				fmt.Fprintf(os.Stderr, "\n\033[90m--- %s | %d turns | %dms ---\033[0m\n",
					data.StopReason, data.TotalTurns, data.DurationMs)
			}
		}
	}
	return nil
}

func truncate(s string, max int) string {
	s = strings.ReplaceAll(s, "\n", " ")
	if len(s) > max {
		return s[:max] + "…"
	}
	return s
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

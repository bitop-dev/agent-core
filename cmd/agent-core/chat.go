package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"

	"github.com/chzyer/readline"
	"github.com/spf13/cobra"

	"github.com/bitop-dev/agent-core/internal/agent"
	"github.com/bitop-dev/agent-core/internal/config"
	"github.com/bitop-dev/agent-core/internal/observer"
	"github.com/bitop-dev/agent-core/internal/provider"
	"github.com/bitop-dev/agent-core/internal/session"
	"github.com/bitop-dev/agent-core/internal/tool"
)

func chatCmd() *cobra.Command {
	var (
		configPath   string
		modelFlag    string
		providerFlag string
		baseURL      string
		apiKey       string
		sessionID    string
		resume       bool
	)

	cmd := &cobra.Command{
		Use:   "chat",
		Short: "Interactive multi-turn chat with an agent",
		Long:  "Start an interactive chat session. Conversation history is preserved across turns and optionally persisted to disk.",
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
					MaxTurns:       50,
					TimeoutSeconds: 0, // no timeout in chat mode
				}
			}

			if modelFlag != "" {
				cfg.Model = modelFlag
			}
			if providerFlag != "" {
				cfg.Provider = providerFlag
			}

			// High turn limit for chat mode
			if cfg.MaxTurns < 50 {
				cfg.MaxTurns = 50
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

			// Create provider
			provName := cfg.Provider
			if provName == "" {
				provName = detectProvider(cfg.Model)
			}
			p := createProvider(provName, key, url)

			// Wrap with reliability
			reliableCfg := provider.DefaultReliableConfig()
			rp := provider.NewReliable(p, reliableCfg)

			// Create tool engine
			engine := tool.NewEngine()
			registerBuiltins(engine, cfg)

			// Load skills
			skills, err := loadSkills(cfg, engine)
			if err != nil {
				return fmt.Errorf("load skills: %w", err)
			}

			// Build agent
			a, err := agent.NewBuilder().
				WithConfig(cfg).
				WithProvider(rp).
				WithTools(engine).
				WithSkills(skills).
				WithObserver(observer.Noop{}).
				Build()
			if err != nil {
				return fmt.Errorf("build agent: %w", err)
			}

			// Session management
			store, err := session.NewStore(session.DefaultDir())
			if err != nil {
				return fmt.Errorf("session store: %w", err)
			}

			var sess *session.Session
			if sessionID == "" {
				sessionID = session.GenerateID()
			}

			if resume && store.Exists(sessionID) {
				sess, err = store.Load(sessionID)
				if err != nil {
					return fmt.Errorf("load session: %w", err)
				}
				fmt.Fprintf(os.Stderr, "\033[90mResumed session %s (%d messages)\033[0m\n", sessionID, len(sess.Messages))
			} else {
				sess = session.New(sessionID)
				sess.Metadata["model"] = cfg.Model
				sess.Metadata["agent"] = cfg.Name
			}

			// Signal handling
			ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
			defer cancel()

			return runChat(ctx, a, sess, store, cfg)
		},
	}

	cmd.Flags().StringVarP(&configPath, "config", "c", "", "Path to agent YAML config")
	cmd.Flags().StringVar(&modelFlag, "model", "", "Override model from config")
	cmd.Flags().StringVarP(&providerFlag, "provider", "p", "", "Provider: openai, anthropic, openai-responses")
	cmd.Flags().StringVar(&baseURL, "base-url", "", "Override API base URL")
	cmd.Flags().StringVar(&apiKey, "api-key", "", "API key")
	cmd.Flags().StringVarP(&sessionID, "session", "s", "", "Session ID (auto-generated if not set)")
	cmd.Flags().BoolVar(&resume, "resume", false, "Resume a previous session")

	return cmd
}

func runChat(ctx context.Context, a *agent.Agent, sess *session.Session, store *session.Store, cfg *config.AgentConfig) error {
	// Print header
	name := cfg.Name
	if name == "" {
		name = cfg.Model
	}
	fmt.Fprintf(os.Stderr, "\033[1m%s\033[0m \033[90m(session: %s)\033[0m\n", name, sess.ID)
	fmt.Fprintf(os.Stderr, "\033[90mType /help for commands, Ctrl+D to exit\033[0m\n\n")

	// Setup readline
	rl, err := readline.NewEx(&readline.Config{
		Prompt:          "\033[1;34m> \033[0m",
		InterruptPrompt: "^C",
		EOFPrompt:       "exit",
	})
	if err != nil {
		return fmt.Errorf("readline: %w", err)
	}
	defer rl.Close()

	for {
		// Read user input
		line, err := rl.Readline()
		if err == readline.ErrInterrupt {
			continue
		}
		if err == io.EOF {
			// Save session on exit
			if len(sess.Messages) > 0 {
				store.Save(sess)
				fmt.Fprintf(os.Stderr, "\n\033[90mSession saved: %s\033[0m\n", sess.ID)
			}
			return nil
		}

		input := strings.TrimSpace(line)
		if input == "" {
			continue
		}

		// Handle slash commands
		if strings.HasPrefix(input, "/") {
			if handleSlashCommand(input, sess, store) {
				continue
			}
		}

		// Add user message to session
		userMsg := provider.Message{
			Role: provider.RoleUser,
			Content: []provider.ContentBlock{
				{Type: provider.ContentText, Text: input},
			},
		}
		sess.Append(userMsg)

		// Run the agent with full history
		events, err := a.RunWithHistory(ctx, sess.Messages)
		if err != nil {
			fmt.Fprintf(os.Stderr, "\033[31merror: %v\033[0m\n", err)
			// Remove the user message we just added since the run failed
			sess.Messages = sess.Messages[:len(sess.Messages)-1]
			continue
		}

		// Render events and capture updated history
		var updatedHistory []provider.Message
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
					updatedHistory = data.History
					fmt.Fprintf(os.Stderr, "\n\033[90m[%d turns | %dms]\033[0m\n\n",
						data.TotalTurns, data.DurationMs)
				}
			}
		}

		// Update session with the full history from the agent
		if updatedHistory != nil {
			sess.Messages = updatedHistory
		}

		// Auto-save after each exchange
		store.Save(sess)
	}
}

// handleSlashCommand processes /commands. Returns true if handled.
func handleSlashCommand(input string, sess *session.Session, store *session.Store) bool {
	parts := strings.Fields(input)
	cmd := parts[0]

	switch cmd {
	case "/help":
		fmt.Println(`Commands:
  /help      Show this help
  /history   Show conversation history (message count)
  /clear     Clear conversation history
  /save      Save session to disk
  /session   Show current session ID
  /exit      Exit chat`)
		return true

	case "/history":
		userCount := 0
		assistantCount := 0
		for _, m := range sess.Messages {
			switch m.Role {
			case provider.RoleUser:
				userCount++
			case provider.RoleAssistant:
				assistantCount++
			}
		}
		fmt.Printf("Session %s: %d messages (%d user, %d assistant, %d tool)\n",
			sess.ID, len(sess.Messages), userCount, assistantCount,
			len(sess.Messages)-userCount-assistantCount)
		return true

	case "/clear":
		sess.Messages = nil
		fmt.Println("History cleared.")
		return true

	case "/save":
		store.Save(sess)
		fmt.Printf("Session saved: %s\n", sess.ID)
		return true

	case "/session":
		fmt.Printf("Session: %s\n", sess.ID)
		return true

	case "/exit", "/quit":
		if len(sess.Messages) > 0 {
			store.Save(sess)
			fmt.Fprintf(os.Stderr, "\033[90mSession saved: %s\033[0m\n", sess.ID)
		}
		os.Exit(0)
		return true

	default:
		fmt.Printf("Unknown command: %s (type /help for commands)\n", cmd)
		return true
	}
}

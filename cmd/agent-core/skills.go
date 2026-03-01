package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/bitop-dev/agent-core/internal/config"
	"github.com/bitop-dev/agent-core/internal/sandbox"
	"github.com/bitop-dev/agent-core/internal/skill"
	"github.com/bitop-dev/agent-core/internal/tool"
)

// defaultSkillDirs returns the standard directories to search for skills.
func defaultSkillDirs() []string {
	home, _ := os.UserHomeDir()
	return []string{
		filepath.Join(home, ".agent-core", "skills"),
	}
}

// sandboxRegistry is the global sandbox registry for WASM/container runtimes.
// Initialized lazily when a skill needs it.
var sandboxRegistry *sandbox.Registry

// initSandboxRegistry creates the sandbox runtimes based on config.
// Returns a cleanup function to close runtimes.
func initSandboxRegistry(cfg *config.AgentConfig) func() {
	sandboxRegistry = sandbox.NewRegistry()

	// Initialize WASM runtime (always available — pure Go, no deps)
	wasmRT, err := sandbox.NewWASMRuntime(context.Background())
	if err != nil {
		fmt.Fprintf(os.Stderr, "\033[33mwarning: WASM runtime unavailable: %v\033[0m\n", err)
	} else {
		sandboxRegistry.Register(wasmRT)
		fmt.Fprintf(os.Stderr, "\033[90m✓ WASM sandbox runtime ready\033[0m\n")
	}

	// Initialize container runtime if available
	containerRT, err := sandbox.NewContainerRuntime()
	if err == nil {
		sandboxRegistry.Register(containerRT)
		fmt.Fprintf(os.Stderr, "\033[90m✓ Container sandbox runtime ready (%s)\033[0m\n", containerRT.Engine())
	}

	return func() {
		sandboxRegistry.Close()
	}
}

// buildSandboxCaps converts agent config to sandbox capabilities.
func buildSandboxCaps(cfg *config.AgentConfig) sandbox.Capabilities {
	caps := sandbox.DefaultCapabilities()
	if len(cfg.Sandbox.AllowedPaths) > 0 {
		caps.AllowedPaths = cfg.Sandbox.AllowedPaths
	}
	if len(cfg.Sandbox.ReadOnlyPaths) > 0 {
		caps.ReadOnlyPaths = cfg.Sandbox.ReadOnlyPaths
	}
	if len(cfg.Sandbox.AllowedHosts) > 0 {
		caps.AllowedHosts = cfg.Sandbox.AllowedHosts
	}
	if cfg.Sandbox.MaxMemoryMB > 0 {
		caps.MaxMemoryMB = cfg.Sandbox.MaxMemoryMB
	}
	if cfg.Sandbox.MaxTimeoutSec > 0 {
		caps.MaxTimeoutSec = cfg.Sandbox.MaxTimeoutSec
	}
	return caps
}

// resolveToolRuntime determines which sandbox runtime to use for a tool.
func resolveToolRuntime(skillRuntime, execType, defaultMode string) sandbox.RuntimeType {
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
	// Agent default
	if defaultMode == "container" {
		return sandbox.RuntimeContainer
	}
	// Default: WASM
	return sandbox.RuntimeWASM
}

// loadSkills loads skills referenced in the agent config.
// Returns loaded skills and registers their tools in the engine.
// If a skill isn't found locally, it tries to install from skill_sources.
// All skill tools are dispatched through the WASM sandbox runtime.
func loadSkills(cfg *config.AgentConfig, engine *tool.Engine) ([]*skill.Skill, error) {
	if len(cfg.Skills) == 0 {
		return nil, nil
	}

	destDir := defaultSkillDirs()[0]
	loader := skill.NewLoader(defaultSkillDirs()...)

	// Collect skill names from config
	var names []string
	skillConfigs := make(map[string]map[string]any)
	for _, ref := range cfg.Skills {
		names = append(names, ref.Name)
		if ref.Config != nil {
			skillConfigs[ref.Name] = ref.Config
		}
	}

	// Load skills — first pass (local)
	skills, warnings := loader.LoadByName(names)

	// Find missing skills
	loaded := make(map[string]bool)
	for _, s := range skills {
		loaded[s.Name] = true
	}

	var missing []string
	for _, n := range names {
		if !loaded[n] {
			missing = append(missing, n)
		}
	}

	// Auto-install missing skills from sources
	if len(missing) > 0 {
		sources := cfg.SkillSources
		if len(sources) == 0 {
			sources = []string{skill.DefaultSource}
		}

		for _, name := range missing {
			installed := false
			for _, src := range sources {
				fmt.Fprintf(os.Stderr, "\033[36mInstalling skill %q from %s...\033[0m\n", name, src)
				if err := skill.InstallSkill(src, name, destDir); err != nil {
					continue // try next source
				}
				fmt.Fprintf(os.Stderr, "\033[32m✓ Installed %s\033[0m\n", name)
				installed = true
				break
			}
			if !installed {
				warnings = append(warnings, fmt.Sprintf("skill %q not found in any source", name))
			}
		}

		// Reload after installation
		loader = skill.NewLoader(defaultSkillDirs()...)
		skills, warnings = loader.LoadByName(names)
	}

	for _, w := range warnings {
		fmt.Fprintf(os.Stderr, "\033[33mwarning: %s\033[0m\n", w)
	}

	// Build sandbox capabilities from config
	caps := buildSandboxCaps(cfg)
	defaultMode := cfg.Sandbox.Mode

	// Register skill tools in the engine
	for _, sk := range skills {
		for _, td := range sk.Tools {
			// Determine the module reference and runtime type.
			// For container skills, Module is the image name.
			// For WASM skills, Module is the .wasm file path.
			var module string
			var rt sandbox.RuntimeType

			if sk.Runtime == "container" && sk.Image != "" {
				// Container skill — Module is the Docker/Podman image
				module = sk.Image
				rt = sandbox.RuntimeContainer
			} else {
				// WASM or auto-detect — find the .wasm file
				execPath, execType := skill.FindToolExec(sk.Dir, td.Name)
				if execPath == "" {
					fmt.Fprintf(os.Stderr, "\033[33mwarning: no executable found for tool %s in skill %s\033[0m\n", td.Name, sk.Name)
					continue
				}
				module = execPath
				rt = resolveToolRuntime(sk.Runtime, execType, defaultMode)
			}

			if sandboxRegistry == nil {
				fmt.Fprintf(os.Stderr, "\033[33mwarning: sandbox not initialized, cannot register tool %s\033[0m\n", td.Name)
				continue
			}
			if _, err := sandboxRegistry.Get(rt); err != nil {
				fmt.Fprintf(os.Stderr, "\033[33mwarning: runtime %s not available for tool %s\033[0m\n", rt, td.Name)
				continue
			}

			// Containers need absolute workdir; WASM doesn't use it
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
				Runtime:     rt,
				Module:      module,
				WorkDir:     workDir,
				Caps:        caps,
				Registry:    sandboxRegistry,
				SkillConfig: skillConfigs[sk.Name],
			})
			engine.Register(st)
			fmt.Fprintf(os.Stderr, "\033[90m  ✓ %s → %s sandbox\033[0m\n", td.Name, rt)
		}
	}

	return skills, nil
}

// skillCmd handles skill management commands.
func skillCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "skill",
		Short: "Skill management",
	}

	cmd.AddCommand(skillListCmd())
	cmd.AddCommand(skillShowCmd())
	cmd.AddCommand(skillTestCmd())
	cmd.AddCommand(skillInstallCmd())
	cmd.AddCommand(skillRemoveCmd())
	cmd.AddCommand(skillUpdateCmd())
	cmd.AddCommand(skillSearchCmd())

	return cmd
}

func skillListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List installed skills",
		RunE: func(cmd *cobra.Command, args []string) error {
			loader := skill.NewLoader(defaultSkillDirs()...)
			skills, warnings := loader.LoadAll()

			for _, w := range warnings {
				fmt.Fprintf(os.Stderr, "\033[33m%s\033[0m\n", w)
			}

			if len(skills) == 0 {
				fmt.Println("No skills installed.")
				fmt.Printf("Install skills to: %s\n", defaultSkillDirs()[0])
				return nil
			}

			fmt.Printf("%-20s  %-8s  %-6s  %s\n", "Name", "Version", "Tools", "Description")
			fmt.Printf("%-20s  %-8s  %-6s  %s\n", "---", "---", "---", "---")
			for _, sk := range skills {
				emoji := sk.Emoji
				if emoji == "" {
					emoji = " "
				}
				desc := sk.Description
				if len(desc) > 60 {
					desc = desc[:60] + "..."
				}
				fmt.Printf("%s %-18s  %-8s  %-6d  %s\n",
					emoji, sk.Name, sk.Version, len(sk.Tools), desc)
			}
			return nil
		},
	}
}

func skillShowCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "show [skill-name]",
		Short: "Show skill details",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			loader := skill.NewLoader(defaultSkillDirs()...)
			skills, _ := loader.LoadByName([]string{args[0]})

			if len(skills) == 0 {
				return fmt.Errorf("skill %q not found", args[0])
			}

			sk := skills[0]
			fmt.Printf("Name:        %s %s\n", sk.Emoji, sk.Name)
			fmt.Printf("Version:     %s\n", sk.Version)
			fmt.Printf("Author:      %s\n", sk.Author)
			fmt.Printf("Description: %s\n", sk.Description)
			fmt.Printf("Tags:        %v\n", sk.Tags)
			fmt.Printf("Dir:         %s\n", sk.Dir)

			if len(sk.Requires.Bins) > 0 {
				fmt.Printf("Requires:    bins=%v\n", sk.Requires.Bins)
			}
			if len(sk.Requires.Env) > 0 {
				fmt.Printf("Requires:    env=%v\n", sk.Requires.Env)
			}

			if len(sk.Tools) > 0 {
				fmt.Printf("\nTools:\n")
				for _, t := range sk.Tools {
					fmt.Printf("  %-20s %s\n", t.Name, t.Description)
				}
			}

			return nil
		},
	}
}

func skillInstallCmd() *cobra.Command {
	var source string
	cmd := &cobra.Command{
		Use:   "install [skill-name]",
		Short: "Install a skill from a registry",
		Long:  "Install a skill from a skill source (GitHub repo with registry.json).\nDefault source: " + skill.DefaultSource,
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := args[0]
			dest := defaultSkillDirs()[0]

			fmt.Printf("Installing skill %q from %s...\n", name, source)
			if err := skill.InstallSkill(source, name, dest); err != nil {
				return fmt.Errorf("❌ %w", err)
			}
			fmt.Printf("✓ Installed %s to %s/%s\n", name, dest, name)
			return nil
		},
	}
	cmd.Flags().StringVar(&source, "source", skill.DefaultSource, "Skill source (GitHub repo URL)")
	return cmd
}

func skillRemoveCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "remove [skill-name]",
		Short: "Remove an installed skill",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := args[0]
			dest := defaultSkillDirs()[0]

			if err := skill.RemoveSkill(name, dest); err != nil {
				return fmt.Errorf("❌ %w", err)
			}
			fmt.Printf("✓ Removed %s\n", name)
			return nil
		},
	}
}

func skillUpdateCmd() *cobra.Command {
	var source string
	cmd := &cobra.Command{
		Use:   "update [skill-name]",
		Short: "Update an installed skill to the latest version",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := args[0]
			dest := defaultSkillDirs()[0]

			fmt.Printf("Updating skill %q from %s...\n", name, source)
			if err := skill.UpdateSkill(source, name, dest); err != nil {
				return fmt.Errorf("❌ %w", err)
			}
			fmt.Printf("✓ Updated %s\n", name)
			return nil
		},
	}
	cmd.Flags().StringVar(&source, "source", skill.DefaultSource, "Skill source (GitHub repo URL)")
	return cmd
}

func skillSearchCmd() *cobra.Command {
	var sources []string
	cmd := &cobra.Command{
		Use:   "search",
		Short: "Search available skills from registries",
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(sources) == 0 {
				sources = []string{skill.DefaultSource}
			}

			items, err := skill.ListRegistrySkills(sources)
			if err != nil {
				return fmt.Errorf("❌ %w", err)
			}

			if len(items) == 0 {
				fmt.Println("No skills found in registries.")
				return nil
			}

			// Check which are already installed
			installed := make(map[string]bool)
			loader := skill.NewLoader(defaultSkillDirs()...)
			localSkills, _ := loader.LoadAll()
			for _, s := range localSkills {
				installed[s.Name] = true
			}

			fmt.Printf("%-20s  %-8s  %-10s  %-10s  %s\n", "Name", "Version", "Tier", "Status", "Description")
			fmt.Printf("%-20s  %-8s  %-10s  %-10s  %s\n", "---", "---", "---", "---", "---")
			for _, item := range items {
				status := "available"
				if installed[item.Name] {
					status = "installed"
				}
				desc := item.Description
				if len(desc) > 50 {
					desc = desc[:50] + "..."
				}
				fmt.Printf("%-20s  %-8s  %-10s  %-10s  %s\n",
					item.Name, item.Version, item.Tier, status, desc)
			}
			return nil
		},
	}
	cmd.Flags().StringSliceVar(&sources, "source", nil, "Skill sources (can specify multiple)")
	return cmd
}

func skillTestCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "test [skill-dir]",
		Short: "Test a skill (validate structure and eligibility)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			loader := skill.NewLoader()
			sk, err := loader.Load(args[0])
			if err != nil {
				return fmt.Errorf("❌ load failed: %w", err)
			}
			fmt.Printf("✓ Loaded: %s %s v%s\n", sk.Emoji, sk.Name, sk.Version)
			fmt.Printf("  Description: %s\n", sk.Description)
			fmt.Printf("  Tools: %d\n", len(sk.Tools))
			fmt.Printf("  Instructions: %d chars\n", len(sk.Instructions))

			// Check eligibility
			errs := skill.CheckEligibility(sk)
			if len(errs) > 0 {
				fmt.Println("\n❌ Eligibility checks failed:")
				for _, e := range errs {
					fmt.Printf("  - %s\n", e)
				}
				return fmt.Errorf("skill not eligible")
			}
			fmt.Println("✓ Eligibility: all requirements met")

			return nil
		},
	}
}

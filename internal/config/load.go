package config

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// Load reads and parses an agent config from a YAML file.
func Load(path string) (*AgentConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config: %w", err)
	}
	return Parse(data)
}

// Parse parses an agent config from YAML bytes.
func Parse(data []byte) (*AgentConfig, error) {
	var cfg AgentConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}
	// Apply defaults
	if cfg.MaxTurns <= 0 {
		cfg.MaxTurns = 20
	}
	if cfg.TimeoutSeconds <= 0 {
		cfg.TimeoutSeconds = 300
	}
	if cfg.Context.Strategy == "" {
		cfg.Context.Strategy = "compaction"
	}
	if cfg.Context.CompactionThreshold == 0 {
		cfg.Context.CompactionThreshold = 0.8
	}
	if cfg.Output.Format == "" {
		cfg.Output.Format = "text"
	}
	if cfg.Output.Color == "" {
		cfg.Output.Color = "auto"
	}
	return &cfg, nil
}

// UnmarshalYAML implements custom YAML unmarshaling for SkillRef
// to handle both string and map forms:
//   - web_search                  → SkillRef{Name: "web_search"}
//   - web_search:                 → SkillRef{Name: "web_search", Config: {...}}
//       backend: ddg
func (s *SkillRef) UnmarshalYAML(node *yaml.Node) error {
	// Simple string form
	if node.Kind == yaml.ScalarNode {
		s.Name = node.Value
		return nil
	}

	// Map form: single key → value map
	if node.Kind == yaml.MappingNode {
		if len(node.Content) != 2 {
			return fmt.Errorf("skill ref must be a string or single-key map")
		}
		s.Name = node.Content[0].Value
		s.Config = make(map[string]any)
		return node.Content[1].Decode(&s.Config)
	}

	return fmt.Errorf("invalid skill ref: expected string or map")
}

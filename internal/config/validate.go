package config

import "fmt"

// Validate checks an AgentConfig for required fields and valid values.
func (c *AgentConfig) Validate() error {
	if c.Provider == "" {
		return fmt.Errorf("provider is required")
	}
	if c.Model == "" {
		return fmt.Errorf("model is required")
	}
	if c.MaxTurns <= 0 {
		c.MaxTurns = 20
	}
	if c.TimeoutSeconds <= 0 {
		c.TimeoutSeconds = 300
	}
	return nil
}

package provider

import "fmt"

// Factory creates a Provider from API keys and options.
type Factory func(apiKeys []string) (Provider, error)

// registry maps provider names to their factories.
var registry = map[string]Factory{}

// Register adds a provider factory to the registry.
func Register(name string, factory Factory) {
	registry[name] = factory
}

// New creates a provider by name from the registry.
func New(name string, apiKeys []string) (Provider, error) {
	factory, ok := registry[name]
	if !ok {
		return nil, fmt.Errorf("unknown provider: %s (available: %v)", name, Names())
	}
	return factory(apiKeys)
}

// Names returns all registered provider names.
func Names() []string {
	names := make([]string, 0, len(registry))
	for name := range registry {
		names = append(names, name)
	}
	return names
}

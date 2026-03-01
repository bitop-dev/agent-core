package tool

// SandboxPolicy defines the security constraints for tool execution.
type SandboxPolicy struct {
	AllowedPaths   []string // filesystem paths tools can access
	AllowedHosts   []string // HTTP hosts tools can reach (empty = unrestricted)
	TimeoutSeconds int      // max execution time per tool call
	MaxOutputBytes int64    // max stdout bytes per tool call
	EnvAllowlist   []string // env vars passed to subprocess tools
}

// DefaultPolicy returns the default sandbox policy.
func DefaultPolicy() SandboxPolicy {
	return SandboxPolicy{
		TimeoutSeconds: 30,
		MaxOutputBytes: 1 << 20, // 1MB
	}
}

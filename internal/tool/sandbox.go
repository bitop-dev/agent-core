package tool

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// SandboxPolicy defines the security boundary for tool execution.
type SandboxPolicy struct {
	// AllowedPaths is the list of directory prefixes tools may access.
	// An empty list means unrestricted. Paths are resolved to absolute.
	AllowedPaths []string

	// DeniedPaths is the list of directory prefixes tools must NOT access.
	// Checked after AllowedPaths. Useful for excluding .git, .env, etc.
	DeniedPaths []string

	// AllowedEnvKeys is the list of env var names tools may see.
	// An empty list means all env vars are inherited. When set,
	// only these keys (plus PATH, HOME, TMPDIR) are passed to subprocesses.
	AllowedEnvKeys []string

	// MaxOutputBytes caps the size of tool output before truncation.
	// 0 means unlimited. Default: 1MB.
	MaxOutputBytes int

	// DefaultTimeoutSec is the default subprocess timeout.
	// 0 means 60 seconds.
	DefaultTimeoutSec int
}

// DefaultSandboxPolicy returns a permissive policy (no path restrictions).
func DefaultSandboxPolicy() SandboxPolicy {
	return SandboxPolicy{
		MaxOutputBytes:    1024 * 1024, // 1 MB
		DefaultTimeoutSec: 60,
	}
}

// CheckPath verifies that the given path is allowed by the policy.
// Returns nil if allowed, error if denied.
func (p *SandboxPolicy) CheckPath(path string) error {
	abs, err := filepath.Abs(path)
	if err != nil {
		return fmt.Errorf("cannot resolve path: %w", err)
	}

	// Check denied paths first (explicit deny beats allow)
	for _, denied := range p.DeniedPaths {
		deniedAbs, err := filepath.Abs(denied)
		if err != nil {
			continue
		}
		if strings.HasPrefix(abs, deniedAbs+string(filepath.Separator)) || abs == deniedAbs {
			return fmt.Errorf("path %q is in denied area %q", path, denied)
		}
	}

	// If no allowed paths configured, everything (not denied) is allowed
	if len(p.AllowedPaths) == 0 {
		return nil
	}

	// Check if path falls under an allowed prefix
	for _, allowed := range p.AllowedPaths {
		allowedAbs, err := filepath.Abs(allowed)
		if err != nil {
			continue
		}
		if strings.HasPrefix(abs, allowedAbs+string(filepath.Separator)) || abs == allowedAbs {
			return nil
		}
	}

	return fmt.Errorf("path %q is outside allowed areas %v", path, p.AllowedPaths)
}

// FilteredEnv returns an environment slice filtered by the policy.
// Always includes PATH, HOME, and TMPDIR.
func (p *SandboxPolicy) FilteredEnv() []string {
	if len(p.AllowedEnvKeys) == 0 {
		return nil // nil means inherit all — default Go behavior
	}

	// Always-allowed keys
	allowed := map[string]bool{
		"PATH":   true,
		"HOME":   true,
		"TMPDIR": true,
	}
	for _, key := range p.AllowedEnvKeys {
		allowed[strings.ToUpper(key)] = true
	}

	var env []string
	for _, kv := range os.Environ() {
		key, _, ok := strings.Cut(kv, "=")
		if ok && allowed[strings.ToUpper(key)] {
			env = append(env, kv)
		}
	}
	return env
}

// TruncateOutput truncates output to MaxOutputBytes if set.
// Returns the (possibly truncated) string and whether it was truncated.
func (p *SandboxPolicy) TruncateOutput(s string) (string, bool) {
	if p.MaxOutputBytes <= 0 || len(s) <= p.MaxOutputBytes {
		return s, false
	}
	return s[:p.MaxOutputBytes] + "\n... [output truncated]", true
}

// Timeout returns the effective timeout in seconds.
func (p *SandboxPolicy) Timeout() int {
	if p.DefaultTimeoutSec > 0 {
		return p.DefaultTimeoutSec
	}
	return 60
}

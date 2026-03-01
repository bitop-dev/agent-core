package tool

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCheckPath_Unrestricted(t *testing.T) {
	p := DefaultSandboxPolicy()
	if err := p.CheckPath("/any/path"); err != nil {
		t.Errorf("unrestricted policy should allow any path: %v", err)
	}
}

func TestCheckPath_AllowedPaths(t *testing.T) {
	tmpDir := t.TempDir()
	p := SandboxPolicy{
		AllowedPaths: []string{tmpDir},
	}

	// Inside allowed
	if err := p.CheckPath(filepath.Join(tmpDir, "file.txt")); err != nil {
		t.Errorf("should allow path inside allowed dir: %v", err)
	}

	// The allowed dir itself
	if err := p.CheckPath(tmpDir); err != nil {
		t.Errorf("should allow the allowed dir itself: %v", err)
	}

	// Outside allowed
	if err := p.CheckPath("/etc/passwd"); err == nil {
		t.Error("should deny path outside allowed dirs")
	}
}

func TestCheckPath_DeniedPaths(t *testing.T) {
	tmpDir := t.TempDir()
	gitDir := filepath.Join(tmpDir, ".git")
	os.MkdirAll(gitDir, 0755)

	p := SandboxPolicy{
		AllowedPaths: []string{tmpDir},
		DeniedPaths:  []string{gitDir},
	}

	// Allowed but not in denied
	if err := p.CheckPath(filepath.Join(tmpDir, "main.go")); err != nil {
		t.Errorf("should allow non-denied path: %v", err)
	}

	// In denied subdir
	if err := p.CheckPath(filepath.Join(gitDir, "config")); err == nil {
		t.Error("should deny path inside denied dir")
	}

	// The denied dir itself
	if err := p.CheckPath(gitDir); err == nil {
		t.Error("should deny the denied dir itself")
	}
}

func TestCheckPath_DeniedOverridesAllowed(t *testing.T) {
	tmpDir := t.TempDir()
	secretDir := filepath.Join(tmpDir, "secrets")
	os.MkdirAll(secretDir, 0755)

	p := SandboxPolicy{
		AllowedPaths: []string{tmpDir},
		DeniedPaths:  []string{secretDir},
	}

	if err := p.CheckPath(filepath.Join(secretDir, "key.pem")); err == nil {
		t.Error("denied should override allowed")
	}
}

func TestFilteredEnv_NoRestrictions(t *testing.T) {
	p := DefaultSandboxPolicy()
	env := p.FilteredEnv()
	if env != nil {
		t.Error("unrestricted policy should return nil (inherit all)")
	}
}

func TestFilteredEnv_Allowlist(t *testing.T) {
	os.Setenv("TEST_SANDBOX_ALLOWED", "yes")
	os.Setenv("TEST_SANDBOX_BLOCKED", "no")
	defer os.Unsetenv("TEST_SANDBOX_ALLOWED")
	defer os.Unsetenv("TEST_SANDBOX_BLOCKED")

	p := SandboxPolicy{
		AllowedEnvKeys: []string{"TEST_SANDBOX_ALLOWED"},
	}

	env := p.FilteredEnv()
	if env == nil {
		t.Fatal("expected filtered env, got nil")
	}

	envMap := make(map[string]string)
	for _, kv := range env {
		k, v, _ := strings.Cut(kv, "=")
		envMap[k] = v
	}

	// Should have the allowed key
	if envMap["TEST_SANDBOX_ALLOWED"] != "yes" {
		t.Error("TEST_SANDBOX_ALLOWED should be present")
	}

	// Should NOT have the blocked key
	if _, ok := envMap["TEST_SANDBOX_BLOCKED"]; ok {
		t.Error("TEST_SANDBOX_BLOCKED should be filtered out")
	}

	// Should always have PATH and HOME
	if _, ok := envMap["PATH"]; !ok {
		t.Error("PATH should always be present")
	}
	if _, ok := envMap["HOME"]; !ok {
		t.Error("HOME should always be present")
	}
}

func TestTruncateOutput(t *testing.T) {
	p := SandboxPolicy{MaxOutputBytes: 10}

	// Short output — not truncated
	s, truncated := p.TruncateOutput("hello")
	if truncated || s != "hello" {
		t.Errorf("short output should not be truncated: %q %v", s, truncated)
	}

	// Long output — truncated
	s, truncated = p.TruncateOutput("hello world this is long")
	if !truncated {
		t.Error("long output should be truncated")
	}
	if !strings.HasPrefix(s, "hello worl") {
		t.Errorf("should preserve prefix: %q", s)
	}
	if !strings.Contains(s, "[output truncated]") {
		t.Errorf("should contain truncation marker: %q", s)
	}
}

func TestTruncateOutput_Unlimited(t *testing.T) {
	p := SandboxPolicy{MaxOutputBytes: 0}
	big := strings.Repeat("x", 10_000_000)
	s, truncated := p.TruncateOutput(big)
	if truncated || len(s) != len(big) {
		t.Error("unlimited should not truncate")
	}
}

func TestTimeout(t *testing.T) {
	p := DefaultSandboxPolicy()
	if p.Timeout() != 60 {
		t.Errorf("default timeout should be 60, got %d", p.Timeout())
	}

	p.DefaultTimeoutSec = 120
	if p.Timeout() != 120 {
		t.Errorf("custom timeout should be 120, got %d", p.Timeout())
	}

	p.DefaultTimeoutSec = 0
	if p.Timeout() != 60 {
		t.Errorf("zero timeout should fallback to 60, got %d", p.Timeout())
	}
}

package agent

import (
	"bytes"
	"strings"
	"testing"
)

func supervisedConfig() ApprovalConfig {
	return ApprovalConfig{
		Mode:        ApprovalSupervised,
		AutoApprove: []string{"read_file", "list_dir"},
		AlwaysAsk:   []string{"bash"},
	}
}

func fullConfig() ApprovalConfig {
	return ApprovalConfig{
		Mode: ApprovalFull,
	}
}

// 1. auto_approve tools skip prompt
func TestAutoApproveToolsSkipPrompt(t *testing.T) {
	mgr := NewApprovalManager(supervisedConfig())
	if mgr.NeedsApproval("read_file") {
		t.Error("read_file should not need approval (auto_approve)")
	}
	if mgr.NeedsApproval("list_dir") {
		t.Error("list_dir should not need approval (auto_approve)")
	}
}

// 2. always_ask tools always prompt
func TestAlwaysAskToolsAlwaysPrompt(t *testing.T) {
	mgr := NewApprovalManager(supervisedConfig())
	if !mgr.NeedsApproval("bash") {
		t.Error("bash should always need approval (always_ask)")
	}
}

// 3. Unknown tools need approval in supervised mode
func TestUnknownToolNeedsApprovalInSupervised(t *testing.T) {
	mgr := NewApprovalManager(supervisedConfig())
	if !mgr.NeedsApproval("write_file") {
		t.Error("write_file should need approval (not in auto_approve)")
	}
}

// 4. Full autonomy never prompts
func TestFullAutonomyNeverPrompts(t *testing.T) {
	mgr := NewApprovalManager(fullConfig())
	for _, tool := range []string{"bash", "write_file", "anything"} {
		if mgr.NeedsApproval(tool) {
			t.Errorf("%s should not need approval in full mode", tool)
		}
	}
}

// 5. "Always" response adds to session allowlist
func TestAlwaysAddsToSessionAllowlist(t *testing.T) {
	mgr := NewApprovalManager(supervisedConfig())
	if !mgr.NeedsApproval("write_file") {
		t.Fatal("write_file should need approval initially")
	}

	// Simulate "always" response
	mgr.promptFunc = func(_, _ string) ApprovalResponse { return ApprovalAlways }
	mgr.RequestApproval("write_file", `{"path":"test.txt"}`)

	if mgr.NeedsApproval("write_file") {
		t.Error("write_file should not need approval after 'always'")
	}
}

// 6. always_ask overrides session allowlist
func TestAlwaysAskOverridesSessionAllowlist(t *testing.T) {
	mgr := NewApprovalManager(supervisedConfig())
	mgr.promptFunc = func(_, _ string) ApprovalResponse { return ApprovalAlways }
	mgr.RequestApproval("bash", `{"command":"ls"}`)

	// bash is in always_ask, so it should still need approval
	if !mgr.NeedsApproval("bash") {
		t.Error("bash should still need approval (always_ask overrides session allowlist)")
	}
}

// 7. "Yes" does not add to session allowlist
func TestYesDoesNotAddToAllowlist(t *testing.T) {
	mgr := NewApprovalManager(supervisedConfig())
	mgr.promptFunc = func(_, _ string) ApprovalResponse { return ApprovalYes }
	mgr.RequestApproval("write_file", `{}`)

	if !mgr.NeedsApproval("write_file") {
		t.Error("write_file should still need approval after 'yes' (not 'always')")
	}
}

// 8. Audit log records decisions
func TestAuditLogRecordsDecisions(t *testing.T) {
	mgr := NewApprovalManager(supervisedConfig())

	mgr.promptFunc = func(_, _ string) ApprovalResponse { return ApprovalNo }
	mgr.RequestApproval("bash", `{"command":"rm -rf ./build/"}`)

	mgr.promptFunc = func(_, _ string) ApprovalResponse { return ApprovalYes }
	mgr.RequestApproval("write_file", `{"path":"out.txt"}`)

	log := mgr.AuditLog()
	if len(log) != 2 {
		t.Fatalf("expected 2 log entries, got %d", len(log))
	}
	if log[0].ToolName != "bash" || log[0].Decision != ApprovalNo {
		t.Errorf("unexpected first entry: %+v", log[0])
	}
	if log[1].ToolName != "write_file" || log[1].Decision != ApprovalYes {
		t.Errorf("unexpected second entry: %+v", log[1])
	}
}

// 9. CLI prompt reads "y"
func TestPromptFromReader_Yes(t *testing.T) {
	r := strings.NewReader("y\n")
	w := &bytes.Buffer{}
	resp := promptFromReader(r, w, "bash", `command: ls`)
	if resp != ApprovalYes {
		t.Errorf("expected Yes, got %v", resp)
	}
	if !strings.Contains(w.String(), "bash") {
		t.Error("prompt should mention tool name")
	}
}

// 10. CLI prompt reads "a"
func TestPromptFromReader_Always(t *testing.T) {
	r := strings.NewReader("a\n")
	w := &bytes.Buffer{}
	resp := promptFromReader(r, w, "write_file", `path: test.txt`)
	if resp != ApprovalAlways {
		t.Errorf("expected Always, got %v", resp)
	}
}

// 11. CLI prompt reads "n" (or anything else)
func TestPromptFromReader_No(t *testing.T) {
	r := strings.NewReader("n\n")
	w := &bytes.Buffer{}
	resp := promptFromReader(r, w, "bash", `command: rm -rf /`)
	if resp != ApprovalNo {
		t.Errorf("expected No, got %v", resp)
	}
}

// 12. CLI prompt with empty input defaults to No
func TestPromptFromReader_EmptyInput(t *testing.T) {
	r := strings.NewReader("")
	w := &bytes.Buffer{}
	resp := promptFromReader(r, w, "bash", `command: ls`)
	if resp != ApprovalNo {
		t.Errorf("expected No for empty input, got %v", resp)
	}
}

// 13. Session allowlist snapshot
func TestSessionAllowlistSnapshot(t *testing.T) {
	mgr := NewApprovalManager(supervisedConfig())
	mgr.promptFunc = func(_, _ string) ApprovalResponse { return ApprovalAlways }
	mgr.RequestApproval("write_file", `{}`)
	mgr.RequestApproval("edit_file", `{}`)

	allowlist := mgr.SessionAllowlist()
	if len(allowlist) != 2 {
		t.Fatalf("expected 2 items, got %d", len(allowlist))
	}
}

// 14. Summarize arguments truncation
func TestSummarizeArguments(t *testing.T) {
	short := `{"command": "ls"}`
	if s := summarizeArguments(short); s != short {
		t.Errorf("short args should not be truncated, got: %s", s)
	}

	long := strings.Repeat("x", 300)
	s := summarizeArguments(long)
	if len(s) > 210 {
		t.Errorf("long args should be truncated, got len %d", len(s))
	}
	if !strings.HasSuffix(s, "…") {
		t.Error("truncated args should end with ellipsis")
	}
}

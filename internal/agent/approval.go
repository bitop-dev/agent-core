package agent

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"time"
)

// ApprovalMode controls when tools require interactive approval.
type ApprovalMode int

const (
	// ApprovalFull means full autonomy — no tool requires approval.
	ApprovalFull ApprovalMode = iota
	// ApprovalSupervised means unknown tools require approval.
	// auto_approve tools skip, always_ask tools always prompt.
	ApprovalSupervised
)

// ApprovalResponse is the user's answer to an approval prompt.
type ApprovalResponse int

const (
	ApprovalYes    ApprovalResponse = iota // Execute this one call
	ApprovalNo                             // Deny this call
	ApprovalAlways                         // Execute and add to session allowlist
)

func (r ApprovalResponse) String() string {
	switch r {
	case ApprovalYes:
		return "yes"
	case ApprovalNo:
		return "no"
	case ApprovalAlways:
		return "always"
	default:
		return "unknown"
	}
}

// ApprovalLogEntry records one approval decision.
type ApprovalLogEntry struct {
	Timestamp string
	ToolName  string
	ArgsSummary string
	Decision  ApprovalResponse
}

// ApprovalManager handles the interactive approval workflow.
// It maintains auto-approve/always-ask lists and a session-scoped
// allowlist built from "Always" responses.
type ApprovalManager struct {
	mu               sync.RWMutex
	mode             ApprovalMode
	autoApprove      map[string]bool // tools that never need approval
	alwaysAsk        map[string]bool // tools that always need approval (overrides session allowlist)
	sessionAllowlist map[string]bool // built from "Always" responses
	auditLog         []ApprovalLogEntry

	// promptFunc can be replaced in tests to avoid stdin reads.
	promptFunc func(toolName, argsSummary string) ApprovalResponse
}

// ApprovalConfig configures the approval manager.
type ApprovalConfig struct {
	Mode        ApprovalMode
	AutoApprove []string // tools that never need approval
	AlwaysAsk   []string // tools that always need approval
}

// NewApprovalManager creates a new approval manager from config.
func NewApprovalManager(cfg ApprovalConfig) *ApprovalManager {
	autoApprove := make(map[string]bool)
	for _, t := range cfg.AutoApprove {
		autoApprove[t] = true
	}
	alwaysAsk := make(map[string]bool)
	for _, t := range cfg.AlwaysAsk {
		alwaysAsk[t] = true
	}

	mgr := &ApprovalManager{
		mode:             cfg.Mode,
		autoApprove:      autoApprove,
		alwaysAsk:        alwaysAsk,
		sessionAllowlist: make(map[string]bool),
	}
	mgr.promptFunc = mgr.promptCLI
	return mgr
}

// NeedsApproval returns true if the tool call requires a prompt.
func (m *ApprovalManager) NeedsApproval(toolName string) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()

	// Full autonomy never prompts
	if m.mode == ApprovalFull {
		return false
	}

	// always_ask overrides everything
	if m.alwaysAsk[toolName] {
		return true
	}

	// auto_approve skips
	if m.autoApprove[toolName] {
		return false
	}

	// Session allowlist from prior "Always" responses
	if m.sessionAllowlist[toolName] {
		return false
	}

	// Default: supervised mode requires approval
	return true
}

// RequestApproval prompts the user and records the decision.
// Returns the user's response.
func (m *ApprovalManager) RequestApproval(toolName, arguments string) ApprovalResponse {
	summary := summarizeArguments(arguments)
	response := m.promptFunc(toolName, summary)

	m.mu.Lock()
	defer m.mu.Unlock()

	// If "Always", add to session allowlist
	if response == ApprovalAlways {
		m.sessionAllowlist[toolName] = true
	}

	// Record audit entry
	m.auditLog = append(m.auditLog, ApprovalLogEntry{
		Timestamp:   time.Now().UTC().Format(time.RFC3339),
		ToolName:    toolName,
		ArgsSummary: summary,
		Decision:    response,
	})

	return response
}

// AuditLog returns a copy of the audit trail.
func (m *ApprovalManager) AuditLog() []ApprovalLogEntry {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]ApprovalLogEntry, len(m.auditLog))
	copy(out, m.auditLog)
	return out
}

// SessionAllowlist returns the current session-scoped auto-approve set.
func (m *ApprovalManager) SessionAllowlist() []string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	var out []string
	for t := range m.sessionAllowlist {
		out = append(out, t)
	}
	return out
}

// promptCLI displays the approval prompt and reads from stdin.
func (m *ApprovalManager) promptCLI(toolName, argsSummary string) ApprovalResponse {
	return promptFromReader(os.Stdin, os.Stderr, toolName, argsSummary)
}

// promptFromReader is the testable core of the CLI prompt.
func promptFromReader(r io.Reader, w io.Writer, toolName, argsSummary string) ApprovalResponse {
	fmt.Fprintln(w)
	fmt.Fprintf(w, "🔧 Agent wants to execute: %s\n", toolName)
	fmt.Fprintf(w, "   %s\n", argsSummary)
	fmt.Fprintf(w, "   [Y]es / [N]o / [A]lways for %s: ", toolName)

	scanner := bufio.NewScanner(r)
	if !scanner.Scan() {
		return ApprovalNo
	}

	switch strings.ToLower(strings.TrimSpace(scanner.Text())) {
	case "y", "yes":
		return ApprovalYes
	case "a", "always":
		return ApprovalAlways
	default:
		return ApprovalNo
	}
}

// summarizeArguments produces a short human-readable summary of JSON arguments.
func summarizeArguments(argsJSON string) string {
	// Simple approach: truncate the raw JSON for display
	s := strings.TrimSpace(argsJSON)
	if len(s) > 200 {
		s = s[:200] + "…"
	}
	return s
}

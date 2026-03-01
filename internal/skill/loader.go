package skill

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// Loader reads skill directories and parses SKILL.md frontmatter + body.
type Loader struct {
	dirs []string // directories to scan for skills
}

// NewLoader creates a skill loader that scans the given directories.
func NewLoader(dirs ...string) *Loader {
	return &Loader{dirs: dirs}
}

// Load reads a single skill from a directory containing SKILL.md.
func (l *Loader) Load(dir string) (*Skill, error) {
	skillPath := filepath.Join(dir, "SKILL.md")
	data, err := os.ReadFile(skillPath)
	if err != nil {
		return nil, fmt.Errorf("read SKILL.md: %w", err)
	}

	skill, err := ParseSkillMD(data)
	if err != nil {
		return nil, fmt.Errorf("parse SKILL.md in %s: %w", dir, err)
	}
	skill.Dir = dir

	// Load tool schemas from tools/ directory
	toolsDir := filepath.Join(dir, "tools")
	if info, err := os.Stat(toolsDir); err == nil && info.IsDir() {
		entries, _ := os.ReadDir(toolsDir)
		for _, entry := range entries {
			if !strings.HasSuffix(entry.Name(), ".json") {
				continue
			}
			toolPath := filepath.Join(toolsDir, entry.Name())
			td, err := LoadToolDef(toolPath)
			if err != nil {
				continue
			}
			skill.Tools = append(skill.Tools, *td)
		}
	}

	return skill, nil
}

// LoadAll scans all configured directories and returns loaded skills.
// Skills that fail eligibility checks are skipped with a warning.
func (l *Loader) LoadAll() ([]*Skill, []string) {
	var skills []*Skill
	var warnings []string

	for _, dir := range l.dirs {
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		for _, entry := range entries {
			if !entry.IsDir() {
				continue
			}
			skillDir := filepath.Join(dir, entry.Name())
			skillMD := filepath.Join(skillDir, "SKILL.md")
			if _, err := os.Stat(skillMD); err != nil {
				continue
			}

			skill, err := l.Load(skillDir)
			if err != nil {
				warnings = append(warnings, fmt.Sprintf("skip %s: %v", entry.Name(), err))
				continue
			}

			// Check eligibility
			if errs := CheckEligibility(skill); len(errs) > 0 {
				for _, e := range errs {
					warnings = append(warnings, fmt.Sprintf("skip %s: %s", skill.Name, e))
				}
				continue
			}

			skills = append(skills, skill)
		}
	}

	return skills, warnings
}

// LoadByName loads specific skills by name from the configured directories.
func (l *Loader) LoadByName(names []string) ([]*Skill, []string) {
	var skills []*Skill
	var warnings []string

	nameSet := make(map[string]bool)
	for _, n := range names {
		nameSet[n] = true
	}

	for _, dir := range l.dirs {
		for name := range nameSet {
			skillDir := filepath.Join(dir, name)
			skillMD := filepath.Join(skillDir, "SKILL.md")
			if _, err := os.Stat(skillMD); err != nil {
				continue
			}

			skill, err := l.Load(skillDir)
			if err != nil {
				warnings = append(warnings, fmt.Sprintf("skip %s: %v", name, err))
				continue
			}

			if errs := CheckEligibility(skill); len(errs) > 0 {
				for _, e := range errs {
					warnings = append(warnings, fmt.Sprintf("skip %s: %s", name, e))
				}
				continue
			}

			skills = append(skills, skill)
			delete(nameSet, name) // found it, don't search other dirs
		}
	}

	// Warn about skills not found
	for name := range nameSet {
		warnings = append(warnings, fmt.Sprintf("skill %q not found in any skill directory", name))
	}

	return skills, warnings
}

// ParseSkillMD parses YAML frontmatter and markdown body from SKILL.md content.
func ParseSkillMD(data []byte) (*Skill, error) {
	content := string(data)

	// Split frontmatter from body
	if !strings.HasPrefix(content, "---\n") {
		return nil, fmt.Errorf("SKILL.md must start with --- frontmatter")
	}

	rest := content[4:]
	endIdx := strings.Index(rest, "\n---\n")
	if endIdx < 0 {
		// Try Windows line endings
		endIdx = strings.Index(rest, "\n---\r\n")
		if endIdx < 0 {
			return nil, fmt.Errorf("SKILL.md frontmatter not closed (missing ---)")
		}
	}

	frontmatter := rest[:endIdx]
	body := strings.TrimSpace(rest[endIdx+5:]) // skip \n---\n

	// Parse frontmatter
	var skill Skill
	if err := yaml.Unmarshal([]byte(frontmatter), &skill); err != nil {
		return nil, fmt.Errorf("parse frontmatter: %w", err)
	}

	skill.Instructions = body
	return &skill, nil
}

// CheckEligibility verifies that a skill's requirements are met.
// Returns a list of unmet requirements (empty = eligible).
func CheckEligibility(skill *Skill) []string {
	var errors []string

	// Check required binaries (all must exist)
	for _, bin := range skill.Requires.Bins {
		if _, err := exec.LookPath(bin); err != nil {
			errors = append(errors, fmt.Sprintf("required binary %q not found in PATH", bin))
		}
	}

	// Check any-of binaries (at least one must exist)
	if len(skill.Requires.AnyBins) > 0 {
		found := false
		for _, bin := range skill.Requires.AnyBins {
			if _, err := exec.LookPath(bin); err == nil {
				found = true
				break
			}
		}
		if !found {
			errors = append(errors, fmt.Sprintf("none of required binaries found: %v", skill.Requires.AnyBins))
		}
	}

	// Check required env vars
	for _, env := range skill.Requires.Env {
		if os.Getenv(env) == "" {
			errors = append(errors, fmt.Sprintf("required env var %q not set", env))
		}
	}

	return errors
}

// ToolDef is a tool schema loaded from a JSON file.
type ToolDef struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Parameters  json.RawMessage `json:"parameters"`
}

// LoadToolDef reads a tool definition from a JSON file.
func LoadToolDef(path string) (*ToolDef, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var td ToolDef
	if err := json.Unmarshal(data, &td); err != nil {
		return nil, err
	}
	return &td, nil
}

// BuildSystemPromptFragment produces the text to inject into the system prompt
// for a set of loaded skills.
func BuildSystemPromptFragment(skills []*Skill) string {
	if len(skills) == 0 {
		return ""
	}

	var sb strings.Builder
	sb.WriteString("\n\n## Available Skills\n\n")
	sb.WriteString("The following skills provide specialized instructions for specific tasks.\n\n")

	for _, skill := range skills {
		if skill.Emoji != "" {
			sb.WriteString(fmt.Sprintf("### %s %s\n\n", skill.Emoji, skill.Name))
		} else {
			sb.WriteString(fmt.Sprintf("### %s\n\n", skill.Name))
		}

		if skill.Description != "" {
			sb.WriteString(skill.Description)
			sb.WriteString("\n\n")
		}

		if skill.Instructions != "" {
			sb.WriteString(skill.Instructions)
			sb.WriteString("\n\n")
		}
	}

	return sb.String()
}

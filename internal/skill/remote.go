package skill

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// DefaultSource is the official skill registry.
const DefaultSource = "github.com/bitop-dev/agent-platform-skills"

// RegistryJSON is the index file at the root of a skill source repo.
type RegistryJSON struct {
	Version string         `json:"version"`
	Skills  []RegistryItem `json:"skills"`
}

// RegistryItem is one skill entry in registry.json.
type RegistryItem struct {
	Name         string   `json:"name"`
	Version      string   `json:"version"`
	Path         string   `json:"path"`
	Description  string   `json:"description"`
	Author       string   `json:"author"`
	Tags         []string `json:"tags"`
	Tier         string   `json:"tier"`
	HasTools     bool     `json:"has_tools"`
	RequiresBins []string `json:"requires_bins"`
	RequiresEnv  []string `json:"requires_env"`
}

// FetchRegistry downloads and parses registry.json from a skill source.
func FetchRegistry(source string) (*RegistryJSON, error) {
	rawBase := toRawGitHub(source)
	url := rawBase + "/registry.json"

	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		return nil, fmt.Errorf("fetch %s: %w", url, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("registry returned HTTP %d from %s", resp.StatusCode, url)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var reg RegistryJSON
	if err := json.Unmarshal(body, &reg); err != nil {
		return nil, fmt.Errorf("parse registry.json: %w", err)
	}
	return &reg, nil
}

// FindSkillInRegistry looks up a skill by name in the registry.
func FindSkillInRegistry(reg *RegistryJSON, name string) *RegistryItem {
	for _, s := range reg.Skills {
		if s.Name == name {
			return &s
		}
	}
	return nil
}

// InstallSkill clones a skill from a source repo into the local skills directory.
// Uses git sparse-checkout to only fetch the skill's subdirectory.
func InstallSkill(source, skillName, destDir string) error {
	// Fetch registry to find the skill path
	reg, err := FetchRegistry(source)
	if err != nil {
		return fmt.Errorf("fetch registry from %s: %w", source, err)
	}

	item := FindSkillInRegistry(reg, skillName)
	if item == nil {
		return fmt.Errorf("skill %q not found in %s", skillName, source)
	}

	// Target directory
	skillDir := filepath.Join(destDir, skillName)
	if _, err := os.Stat(filepath.Join(skillDir, "SKILL.md")); err == nil {
		return fmt.Errorf("skill %q already installed at %s", skillName, skillDir)
	}

	repoURL := toHTTPS(source)

	// Try sparse checkout first (efficient — only fetches the skill dir)
	if err := sparseClone(repoURL, item.Path, skillDir); err != nil {
		// Fallback: full clone + copy
		return fullCloneExtract(repoURL, item.Path, skillDir)
	}

	return nil
}

// RemoveSkill deletes a skill directory.
func RemoveSkill(name, destDir string) error {
	skillDir := filepath.Join(destDir, name)
	if _, err := os.Stat(skillDir); err != nil {
		return fmt.Errorf("skill %q not installed", name)
	}
	return os.RemoveAll(skillDir)
}

// UpdateSkill removes and re-installs a skill.
func UpdateSkill(source, name, destDir string) error {
	_ = RemoveSkill(name, destDir)
	return InstallSkill(source, name, destDir)
}

// ListRegistrySkills returns all skills available in a source's registry.
func ListRegistrySkills(sources []string) ([]RegistryItem, error) {
	if len(sources) == 0 {
		sources = []string{DefaultSource}
	}

	var all []RegistryItem
	seen := make(map[string]bool)

	for _, src := range sources {
		reg, err := FetchRegistry(src)
		if err != nil {
			return nil, fmt.Errorf("source %s: %w", src, err)
		}
		for _, item := range reg.Skills {
			if !seen[item.Name] {
				all = append(all, item)
				seen[item.Name] = true
			}
		}
	}
	return all, nil
}

// sparseClone does a git sparse-checkout to only get one subdirectory.
func sparseClone(repoURL, skillPath, destDir string) error {
	// Create a temp dir for the sparse checkout
	tmpDir, err := os.MkdirTemp("", "agent-core-skill-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmpDir)

	cmds := [][]string{
		{"git", "clone", "--filter=blob:none", "--no-checkout", "--depth=1", repoURL, tmpDir},
		{"git", "-C", tmpDir, "sparse-checkout", "init", "--cone"},
		{"git", "-C", tmpDir, "sparse-checkout", "set", skillPath},
		{"git", "-C", tmpDir, "checkout"},
	}

	for _, args := range cmds {
		cmd := exec.Command(args[0], args[1:]...)
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("git command %v: %w", args, err)
		}
	}

	// Copy the skill directory to dest
	srcDir := filepath.Join(tmpDir, skillPath)
	if err := os.MkdirAll(filepath.Dir(destDir), 0o755); err != nil {
		return err
	}
	return copyDir(srcDir, destDir)
}

// fullCloneExtract clones the full repo and extracts the skill directory.
func fullCloneExtract(repoURL, skillPath, destDir string) error {
	tmpDir, err := os.MkdirTemp("", "agent-core-skill-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmpDir)

	cmd := exec.Command("git", "clone", "--depth=1", repoURL, tmpDir)
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("git clone: %w", err)
	}

	srcDir := filepath.Join(tmpDir, skillPath)
	if _, err := os.Stat(srcDir); err != nil {
		return fmt.Errorf("skill path %s not found in repo", skillPath)
	}

	return copyDir(srcDir, destDir)
}

// copyDir recursively copies a directory.
func copyDir(src, dst string) error {
	return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		relPath, _ := filepath.Rel(src, path)
		dstPath := filepath.Join(dst, relPath)

		if info.IsDir() {
			return os.MkdirAll(dstPath, info.Mode())
		}

		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(dstPath, data, info.Mode())
	})
}

// toRawGitHub converts github.com/owner/repo to raw.githubusercontent.com/owner/repo/main.
func toRawGitHub(source string) string {
	u := strings.TrimSuffix(source, ".git")
	u = strings.TrimPrefix(u, "https://")
	u = strings.TrimPrefix(u, "http://")

	if strings.HasPrefix(u, "github.com/") {
		parts := strings.SplitN(u, "/", 4)
		if len(parts) >= 3 {
			return fmt.Sprintf("https://raw.githubusercontent.com/%s/%s/main", parts[1], parts[2])
		}
	}
	return "https://" + u
}

// toHTTPS converts github.com/owner/repo to https://github.com/owner/repo.git.
func toHTTPS(source string) string {
	u := strings.TrimPrefix(source, "https://")
	u = strings.TrimPrefix(u, "http://")
	if !strings.HasSuffix(u, ".git") {
		u += ".git"
	}
	return "https://" + u
}

// Package skill handles loading, parsing, and managing skills.
package skill

// Skill is a loaded skill — metadata from frontmatter + instructions from body.
type Skill struct {
	Name         string
	Version      string
	Description  string
	Author       string
	Tags         []string
	Emoji        string
	Always       bool               // inject full instructions even in compact mode
	Requires     Requirements
	Install      []InstallSpec
	Config       map[string]ConfigOption
	Instructions string             // the markdown body (after frontmatter)
	Dir          string             // path to skill directory on disk
	Tools        []ToolDef          // tool schemas loaded from tools/*.json
}

// Requirements declares what a skill needs to function.
type Requirements struct {
	Bins    []string `yaml:"bins"`
	AnyBins []string `yaml:"any_bins"`
	Env     []string `yaml:"env"`
}

// InstallSpec describes how to install a dependency.
type InstallSpec struct {
	ID      string   `yaml:"id"`
	Kind    string   `yaml:"kind"`    // brew | shell | node | go | uv | download
	Formula string   `yaml:"formula"` // for brew
	Command string   `yaml:"command"` // for shell
	Package string   `yaml:"package"` // for node/go/uv
	OS      []string `yaml:"os"`      // restrict to these OSes
	Label   string   `yaml:"label"`
}

// ConfigOption describes a skill configuration parameter.
type ConfigOption struct {
	Type        string   `yaml:"type"`
	Default     any      `yaml:"default"`
	Enum        []string `yaml:"enum,omitempty"`
	Description string   `yaml:"description"`
}

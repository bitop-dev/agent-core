package skill

// Loader reads skill directories and parses SKILL.md frontmatter + body.
type Loader struct {
	dirs []string // directories to scan for skills
}

// NewLoader creates a skill loader that scans the given directories.
func NewLoader(dirs ...string) *Loader {
	return &Loader{dirs: dirs}
}

// TODO: LoadAll — scan dirs, parse SKILL.md, check eligibility, return loaded skills
// TODO: ParseFrontmatter — extract YAML front matter from SKILL.md
// TODO: CheckEligibility — verify bins/env requirements
// TODO: BuildSnapshot — merge skills into system prompt fragment + tool registrations

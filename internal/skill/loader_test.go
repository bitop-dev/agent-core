package skill

import (
	"strings"
	"testing"
)

func TestParseSkillMD(t *testing.T) {
	input := `---
name: test_skill
version: 1.0.0
description: "A test skill"
author: test-author
tags: [test, example]
emoji: 🧪
always: false

requires:
  bins: [python3]
  env: [TEST_KEY]

config:
  backend:
    type: string
    default: default_backend
---

# Test Skill

This is the instruction body.

## Usage

Use this skill for testing.
`
	skill, err := ParseSkillMD([]byte(input))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if skill.Name != "test_skill" {
		t.Errorf("name = %q, want %q", skill.Name, "test_skill")
	}
	if skill.Version != "1.0.0" {
		t.Errorf("version = %q, want %q", skill.Version, "1.0.0")
	}
	if skill.Emoji != "🧪" {
		t.Errorf("emoji = %q, want %q", skill.Emoji, "🧪")
	}
	if skill.Author != "test-author" {
		t.Errorf("author = %q, want %q", skill.Author, "test-author")
	}
	if len(skill.Tags) != 2 || skill.Tags[0] != "test" {
		t.Errorf("tags = %v, want [test example]", skill.Tags)
	}
	if len(skill.Requires.Bins) != 1 || skill.Requires.Bins[0] != "python3" {
		t.Errorf("requires.bins = %v, want [python3]", skill.Requires.Bins)
	}
	if len(skill.Requires.Env) != 1 || skill.Requires.Env[0] != "TEST_KEY" {
		t.Errorf("requires.env = %v, want [TEST_KEY]", skill.Requires.Env)
	}
	if !strings.Contains(skill.Instructions, "# Test Skill") {
		t.Errorf("instructions should contain markdown body, got: %s", skill.Instructions[:50])
	}
	if !strings.Contains(skill.Instructions, "Use this skill for testing.") {
		t.Errorf("instructions should contain full body")
	}
}

func TestParseSkillMD_NoFrontmatter(t *testing.T) {
	_, err := ParseSkillMD([]byte("# Just markdown"))
	if err == nil {
		t.Fatal("expected error for missing frontmatter")
	}
}

func TestParseSkillMD_UnclosedFrontmatter(t *testing.T) {
	_, err := ParseSkillMD([]byte("---\nname: broken\n# no closing"))
	if err == nil {
		t.Fatal("expected error for unclosed frontmatter")
	}
}

func TestBuildSystemPromptFragment(t *testing.T) {
	skills := []*Skill{
		{
			Name:         "web_search",
			Emoji:        "🔍",
			Description:  "Search the web",
			Instructions: "## When to Use\nSearch when you need current facts.",
		},
		{
			Name:         "summarize",
			Emoji:        "📝",
			Description:  "Summarize text",
			Instructions: "Lead with the conclusion.",
		},
	}

	fragment := BuildSystemPromptFragment(skills)

	if !strings.Contains(fragment, "🔍 web_search") {
		t.Error("fragment should contain web_search with emoji")
	}
	if !strings.Contains(fragment, "📝 summarize") {
		t.Error("fragment should contain summarize with emoji")
	}
	if !strings.Contains(fragment, "Search when you need current facts.") {
		t.Error("fragment should contain web_search instructions")
	}
	if !strings.Contains(fragment, "Lead with the conclusion.") {
		t.Error("fragment should contain summarize instructions")
	}
}

func TestBuildSystemPromptFragment_Empty(t *testing.T) {
	fragment := BuildSystemPromptFragment(nil)
	if fragment != "" {
		t.Errorf("empty skills should produce empty fragment, got %q", fragment)
	}
}

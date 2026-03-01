package builtin

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/bitop-dev/agent-core/internal/tool"
)

type grepArgs struct {
	Pattern   string `json:"pattern"`
	Path      string `json:"path"`
	Recursive bool   `json:"recursive,omitempty"`
	Context   int    `json:"context,omitempty"`
}

type grepTool struct{}

func NewGrep() tool.Tool { return &grepTool{} }

func (t *grepTool) Definition() tool.Definition {
	return tool.Definition{
		Name:        "grep",
		Description: "Search for a regex pattern in files. Returns matching lines with file path and line number.",
		InputSchema: json.RawMessage(`{
			"type": "object",
			"properties": {
				"pattern": {
					"type": "string",
					"description": "Regular expression pattern to search for"
				},
				"path": {
					"type": "string",
					"description": "File or directory to search in"
				},
				"recursive": {
					"type": "boolean",
					"description": "Search directories recursively (default: true)"
				},
				"context": {
					"type": "integer",
					"description": "Number of context lines before and after each match (default: 0)"
				}
			},
			"required": ["pattern", "path"]
		}`),
	}
}

func (t *grepTool) Execute(ctx context.Context, input json.RawMessage) (tool.Result, error) {
	var args grepArgs
	if err := json.Unmarshal(input, &args); err != nil {
		return tool.Result{Content: fmt.Sprintf("invalid arguments: %v", err), IsError: true}, nil
	}
	if args.Pattern == "" {
		return tool.Result{Content: "pattern is required", IsError: true}, nil
	}
	if args.Path == "" {
		args.Path = "."
	}

	re, err := regexp.Compile(args.Pattern)
	if err != nil {
		return tool.Result{Content: fmt.Sprintf("invalid regex: %v", err), IsError: true}, nil
	}

	var matches []string
	const maxMatches = 200

	info, err := os.Stat(args.Path)
	if err != nil {
		return tool.Result{Content: fmt.Sprintf("cannot access path: %v", err), IsError: true}, nil
	}

	if !info.IsDir() {
		matches = grepFile(args.Path, re, args.Context, maxMatches)
	} else {
		filepath.Walk(args.Path, func(path string, fi os.FileInfo, err error) error {
			if err != nil || fi.IsDir() {
				return nil
			}
			if len(matches) >= maxMatches {
				return filepath.SkipAll
			}
			remaining := maxMatches - len(matches)
			matches = append(matches, grepFile(path, re, args.Context, remaining)...)
			return nil
		})
	}

	if len(matches) == 0 {
		return tool.Result{Content: "no matches found"}, nil
	}

	result := strings.Join(matches, "\n")
	if len(matches) >= maxMatches {
		result += fmt.Sprintf("\n... (showing first %d matches)", maxMatches)
	}
	return tool.Result{Content: result}, nil
}

func grepFile(path string, re *regexp.Regexp, ctxLines, maxMatches int) []string {
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 256*1024), 256*1024)

	var allLines []string
	for scanner.Scan() {
		allLines = append(allLines, scanner.Text())
	}

	var matches []string
	for i, line := range allLines {
		if len(matches) >= maxMatches {
			break
		}
		if re.MatchString(line) {
			if ctxLines > 0 {
				start := i - ctxLines
				if start < 0 {
					start = 0
				}
				end := i + ctxLines + 1
				if end > len(allLines) {
					end = len(allLines)
				}
				for j := start; j < end; j++ {
					prefix := " "
					if j == i {
						prefix = ">"
					}
					matches = append(matches, fmt.Sprintf("%s%s:%d: %s", prefix, path, j+1, allLines[j]))
				}
				matches = append(matches, "---")
			} else {
				matches = append(matches, fmt.Sprintf("%s:%d: %s", path, i+1, line))
			}
		}
	}
	return matches
}

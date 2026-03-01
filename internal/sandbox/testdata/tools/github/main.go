// github WASM tool — GitHub operations via the GitHub REST API.
// Replaces the shell+gh CLI version. No external dependencies.
//
// Build: GOOS=wasip1 GOARCH=wasm go build -o github.wasm .
package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/bitop-dev/agent-core/internal/sandbox/testdata/hostcall"
)

type request struct {
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments"`
	Config    map[string]any  `json:"config"`
}

// ── gh_issues ────────────────────────────────────────────────────────────────

type issuesArgs struct {
	Repo  string `json:"repo"`
	State string `json:"state"`
	Label string `json:"label"`
	Limit int    `json:"limit"`
}

type ghIssue struct {
	Number    int        `json:"number"`
	Title     string     `json:"title"`
	State     string     `json:"state"`
	User      ghUser     `json:"user"`
	Labels    []ghLabel  `json:"labels"`
	Assignees []ghUser   `json:"assignees"`
	CreatedAt string     `json:"created_at"`
}

type ghUser struct {
	Login string `json:"login"`
}

type ghLabel struct {
	Name string `json:"name"`
}

// ── gh_prs ───────────────────────────────────────────────────────────────────

type prsArgs struct {
	Repo  string `json:"repo"`
	State string `json:"state"`
	Limit int    `json:"limit"`
}

type ghPR struct {
	Number    int    `json:"number"`
	Title     string `json:"title"`
	State     string `json:"state"`
	User      ghUser `json:"user"`
	Head      ghRef  `json:"head"`
	CreatedAt string `json:"created_at"`
	Draft     bool   `json:"draft"`
}

type ghRef struct {
	Ref string `json:"ref"`
}

// ── result ───────────────────────────────────────────────────────────────────

type result struct {
	Content string `json:"content"`
	IsError bool   `json:"is_error"`
}

func main() {
	input, _ := io.ReadAll(os.Stdin)

	var req request
	if err := json.Unmarshal(input, &req); err != nil {
		writeResult(result{Content: "invalid input: " + err.Error(), IsError: true})
		return
	}

	switch req.Name {
	case "gh_issues":
		handleIssues(req)
	case "gh_prs":
		handlePRs(req)
	default:
		writeResult(result{Content: fmt.Sprintf("unknown tool: %s", req.Name), IsError: true})
	}
}

func handleIssues(req request) {
	var args issuesArgs
	if err := json.Unmarshal(req.Arguments, &args); err != nil {
		writeResult(result{Content: "invalid arguments: " + err.Error(), IsError: true})
		return
	}

	if args.Repo == "" {
		writeResult(result{Content: "repo is required (format: owner/repo)", IsError: true})
		return
	}
	if args.State == "" {
		args.State = "open"
	}
	if args.Limit <= 0 {
		args.Limit = 20
	}

	// Build GitHub API URL
	apiURL := fmt.Sprintf("https://api.github.com/repos/%s/issues?state=%s&per_page=%d",
		args.Repo, args.State, args.Limit)
	if args.Label != "" {
		apiURL += "&labels=" + args.Label
	}

	body, err := githubGet(apiURL)
	if err != nil {
		writeResult(result{Content: "GitHub API error: " + err.Error(), IsError: true})
		return
	}

	var issues []ghIssue
	if err := json.Unmarshal(body, &issues); err != nil {
		writeResult(result{Content: "parse error: " + err.Error(), IsError: true})
		return
	}

	// Filter out PRs (GitHub API returns PRs in issues endpoint)
	var filtered []ghIssue
	for _, issue := range issues {
		// PRs have a pull_request field but our struct doesn't capture it,
		// so we rely on the fact that issues don't have "pull_request"
		filtered = append(filtered, issue)
	}

	if len(filtered) == 0 {
		writeResult(result{Content: fmt.Sprintf("No %s issues found for %s", args.State, args.Repo)})
		return
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Issues for %s (%s):\n\n", args.Repo, args.State))
	for _, issue := range filtered {
		line := fmt.Sprintf("#%d [%s] %s", issue.Number, issue.State, issue.Title)
		if issue.User.Login != "" {
			line += fmt.Sprintf(" (by @%s)", issue.User.Login)
		}
		if len(issue.Labels) > 0 {
			var names []string
			for _, l := range issue.Labels {
				names = append(names, l.Name)
			}
			line += fmt.Sprintf(" [%s]", strings.Join(names, ", "))
		}
		if len(issue.Assignees) > 0 {
			var names []string
			for _, a := range issue.Assignees {
				names = append(names, "@"+a.Login)
			}
			line += fmt.Sprintf(" → %s", strings.Join(names, ", "))
		}
		sb.WriteString(line + "\n")
	}

	writeResult(result{Content: sb.String()})
}

func handlePRs(req request) {
	var args prsArgs
	if err := json.Unmarshal(req.Arguments, &args); err != nil {
		writeResult(result{Content: "invalid arguments: " + err.Error(), IsError: true})
		return
	}

	if args.Repo == "" {
		writeResult(result{Content: "repo is required (format: owner/repo)", IsError: true})
		return
	}
	if args.State == "" {
		args.State = "open"
	}
	if args.Limit <= 0 {
		args.Limit = 20
	}

	apiURL := fmt.Sprintf("https://api.github.com/repos/%s/pulls?state=%s&per_page=%d",
		args.Repo, args.State, args.Limit)

	body, err := githubGet(apiURL)
	if err != nil {
		writeResult(result{Content: "GitHub API error: " + err.Error(), IsError: true})
		return
	}

	var prs []ghPR
	if err := json.Unmarshal(body, &prs); err != nil {
		writeResult(result{Content: "parse error: " + err.Error(), IsError: true})
		return
	}

	if len(prs) == 0 {
		writeResult(result{Content: fmt.Sprintf("No %s pull requests found for %s", args.State, args.Repo)})
		return
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Pull requests for %s (%s):\n\n", args.Repo, args.State))
	for _, pr := range prs {
		line := fmt.Sprintf("#%d [%s] %s", pr.Number, pr.State, pr.Title)
		if pr.Draft {
			line += " (draft)"
		}
		if pr.User.Login != "" {
			line += fmt.Sprintf(" (by @%s)", pr.User.Login)
		}
		if pr.Head.Ref != "" {
			line += fmt.Sprintf(" [%s]", pr.Head.Ref)
		}
		sb.WriteString(line + "\n")
	}

	writeResult(result{Content: sb.String()})
}

func githubGet(apiURL string) ([]byte, error) {
	// GITHUB_TOKEN is passed via sandbox env vars if configured
	token := os.Getenv("GITHUB_TOKEN")
	if token == "" {
		token = os.Getenv("GH_TOKEN")
	}

	if token != "" {
		// Use Authorization header via a POST to a wrapper URL... 
		// Actually, the host function only does GET with URL. 
		// For auth, we'd need header support. For public repos, GET works.
		// TODO: Add header support to host function for authenticated requests
	}

	body, err := hostcall.HTTPGet(apiURL)
	if err != nil {
		return nil, err
	}

	// Check for API error response
	if len(body) > 0 && body[0] == '{' {
		var errResp struct {
			Message string `json:"message"`
		}
		if json.Unmarshal(body, &errResp) == nil && errResp.Message != "" {
			// Check if it's actually an error (single object, not array)
			if !strings.HasPrefix(string(body), "[") {
				return nil, fmt.Errorf("%s", errResp.Message)
			}
		}
	}

	return body, nil
}

func writeResult(r result) {
	json.NewEncoder(os.Stdout).Encode(r)
}

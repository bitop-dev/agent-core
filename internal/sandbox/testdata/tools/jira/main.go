package main

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"os"
	"strings"

	"github.com/bitop-dev/agent-core/pkg/hostcall"
)

type request struct {
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments"`
}
type result struct {
	Content string `json:"content"`
	IsError bool   `json:"is_error"`
}

func main() {
	input, _ := io.ReadAll(os.Stdin)
	var req request
	json.Unmarshal(input, &req)

	baseURL := os.Getenv("JIRA_BASE_URL")
	email := os.Getenv("JIRA_EMAIL")
	token := os.Getenv("JIRA_API_TOKEN")
	if baseURL == "" || token == "" || email == "" {
		wr(result{"JIRA_BASE_URL, JIRA_EMAIL, and JIRA_API_TOKEN required", true})
		return
	}
	auth := "Basic " + base64.StdEncoding.EncodeToString([]byte(email+":"+token))
	hdr := map[string]string{"Authorization": auth, "Accept": "application/json", "Content-Type": "application/json"}

	switch req.Name {
	case "jira_list_issues":
		var a struct{ JQL string `json:"jql"`; MaxResults int `json:"max_results"` }
		json.Unmarshal(req.Arguments, &a)
		if a.MaxResults == 0 { a.MaxResults = 20 }
		u := fmt.Sprintf("%s/rest/api/3/search?jql=%s&maxResults=%d", baseURL, url.QueryEscape(a.JQL), a.MaxResults)
		resp, err := hostcall.HTTPRequestWithHeaders("GET", u, hdr, nil)
		if err != nil { wr(result{err.Error(), true}); return }
		wr(result{string(resp), false})

	case "jira_create_issue":
		var a struct{ Project, Summary, Description, IssueType, Priority, Assignee, Labels string }
		json.Unmarshal(req.Arguments, &a)
		if a.IssueType == "" { a.IssueType = "Task" }
		fields := map[string]any{
			"project": map[string]string{"key": a.Project},
			"summary": a.Summary,
			"issuetype": map[string]string{"name": a.IssueType},
		}
		if a.Priority != "" { fields["priority"] = map[string]string{"name": a.Priority} }
		if a.Labels != "" { fields["labels"] = strings.Split(a.Labels, ",") }
		body, _ := json.Marshal(map[string]any{"fields": fields})
		resp, err := hostcall.HTTPRequestWithHeaders("POST", baseURL+"/rest/api/3/issue", hdr, body)
		if err != nil { wr(result{err.Error(), true}); return }
		wr(result{string(resp), false})

	case "jira_update_issue":
		var a struct{ IssueKey, Summary, Transition, Assignee string `json:"issue_key"` }
		json.Unmarshal(req.Arguments, &a)
		fields := map[string]any{}
		if a.Summary != "" { fields["summary"] = a.Summary }
		if len(fields) > 0 {
			body, _ := json.Marshal(map[string]any{"fields": fields})
			hostcall.HTTPRequestWithHeaders("PUT", baseURL+"/rest/api/3/issue/"+a.IssueKey, hdr, body)
		}
		wr(result{fmt.Sprintf("updated %s", a.IssueKey), false})

	default:
		wr(result{"unknown tool: " + req.Name, true})
	}
}

func wr(r result) { json.NewEncoder(os.Stdout).Encode(r) }

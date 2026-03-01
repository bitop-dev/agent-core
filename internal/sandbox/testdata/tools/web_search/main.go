// web_search WASM tool — Search the web via DuckDuckGo.
//
// Build: GOOS=wasip1 GOARCH=wasm go build -o web_search.wasm .
package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"os"
	"strings"

	"github.com/bitop-dev/agent-core/internal/sandbox/testdata/hostcall"
)

type request struct {
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments"`
	Config    map[string]any  `json:"config"`
}

type searchArgs struct {
	Query      string `json:"query"`
	MaxResults int    `json:"max_results"`
}

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

	var args searchArgs
	if err := json.Unmarshal(req.Arguments, &args); err != nil {
		writeResult(result{Content: "invalid arguments: " + err.Error(), IsError: true})
		return
	}

	if args.Query == "" {
		writeResult(result{Content: "query is required", IsError: true})
		return
	}

	maxResults := args.MaxResults
	if maxResults <= 0 {
		maxResults = 10
		// Check config default
		if cfg, ok := req.Config["max_results"]; ok {
			if v, ok := cfg.(float64); ok && v > 0 {
				maxResults = int(v)
			}
		}
	}

	results, err := searchDDG(args.Query, maxResults)
	if err != nil {
		writeResult(result{Content: "search failed: " + err.Error(), IsError: true})
		return
	}

	if len(results) == 0 {
		writeResult(result{Content: "no results found for: " + args.Query})
		return
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Search results for: %s\n\n", args.Query))
	for i, r := range results {
		sb.WriteString(fmt.Sprintf("%d. %s\n   %s\n   %s\n\n", i+1, r.Title, r.URL, r.Snippet))
	}

	writeResult(result{Content: sb.String()})
}

type searchResult struct {
	Title   string
	URL     string
	Snippet string
}

func searchDDG(query string, maxResults int) ([]searchResult, error) {
	searchURL := fmt.Sprintf("https://html.duckduckgo.com/html/?q=%s", url.QueryEscape(query))
	body, err := hostcall.HTTPGet(searchURL)
	if err != nil {
		return nil, fmt.Errorf("http get: %w", err)
	}
	return parseDDGHTML(string(body), maxResults), nil
}

func parseDDGHTML(html string, maxResults int) []searchResult {
	var results []searchResult
	remaining := html

	for len(results) < maxResults {
		linkIdx := strings.Index(remaining, `class="result__a"`)
		if linkIdx < 0 {
			break
		}
		remaining = remaining[linkIdx:]

		href := extractAttr(remaining, "href")
		title := extractTagText(remaining, "a")

		snippetIdx := strings.Index(remaining, `class="result__snippet"`)
		snippet := ""
		if snippetIdx >= 0 {
			snippet = extractTagText(remaining[snippetIdx:], "a")
			if snippet == "" {
				snippet = extractTagText(remaining[snippetIdx:], "span")
			}
		}

		cleanURL := cleanDDGURL(href)

		if title != "" && cleanURL != "" {
			results = append(results, searchResult{
				Title:   cleanText(title),
				URL:     cleanURL,
				Snippet: cleanText(snippet),
			})
		}

		nextIdx := strings.Index(remaining[1:], `class="result__a"`)
		if nextIdx < 0 {
			break
		}
		remaining = remaining[nextIdx+1:]
	}

	return results
}

func extractAttr(html, attr string) string {
	key := attr + `="`
	idx := strings.Index(html, key)
	if idx < 0 {
		return ""
	}
	start := idx + len(key)
	end := strings.Index(html[start:], `"`)
	if end < 0 {
		return ""
	}
	return html[start : start+end]
}

func extractTagText(html, tag string) string {
	closeIdx := strings.Index(html, ">")
	if closeIdx < 0 {
		return ""
	}
	start := closeIdx + 1
	endTag := fmt.Sprintf("</%s>", tag)
	endIdx := strings.Index(html[start:], endTag)
	if endIdx < 0 {
		return ""
	}
	return html[start : start+endIdx]
}

func cleanDDGURL(rawURL string) string {
	if strings.Contains(rawURL, "uddg=") {
		parsed, err := url.Parse(rawURL)
		if err == nil {
			if uddg := parsed.Query().Get("uddg"); uddg != "" {
				return uddg
			}
		}
	}
	if strings.HasPrefix(rawURL, "http") {
		return rawURL
	}
	return ""
}

func cleanText(s string) string {
	var b strings.Builder
	inTag := false
	for _, r := range s {
		if r == '<' {
			inTag = true
			continue
		}
		if r == '>' {
			inTag = false
			continue
		}
		if !inTag {
			b.WriteRune(r)
		}
	}
	return strings.Join(strings.Fields(strings.TrimSpace(b.String())), " ")
}

func writeResult(r result) {
	json.NewEncoder(os.Stdout).Encode(r)
}

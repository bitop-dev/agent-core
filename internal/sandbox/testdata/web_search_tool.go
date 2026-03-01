// web_search_tool is a WASM tool that searches the web via DuckDuckGo.
// It uses the agent_host.http_request host function for network access.
//
// Build: GOOS=wasip1 GOARCH=wasm go build -o web_search_tool.wasm ./web_search_tool.go
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

type searchResult struct {
	Title   string `json:"title"`
	URL     string `json:"url"`
	Snippet string `json:"snippet"`
}

type toolResult struct {
	Content string `json:"content"`
	IsError bool   `json:"is_error"`
}

func main() {
	input, _ := io.ReadAll(os.Stdin)

	var req request
	if err := json.Unmarshal(input, &req); err != nil {
		writeResult(toolResult{Content: "invalid input: " + err.Error(), IsError: true})
		return
	}

	var args searchArgs
	if err := json.Unmarshal(req.Arguments, &args); err != nil {
		writeResult(toolResult{Content: "invalid arguments: " + err.Error(), IsError: true})
		return
	}

	if args.Query == "" {
		writeResult(toolResult{Content: "query is required", IsError: true})
		return
	}

	if args.MaxResults <= 0 {
		args.MaxResults = 5
	}

	results, err := searchDDG(args.Query, args.MaxResults)
	if err != nil {
		writeResult(toolResult{Content: "search failed: " + err.Error(), IsError: true})
		return
	}

	if len(results) == 0 {
		writeResult(toolResult{Content: "no results found for: " + args.Query})
		return
	}

	// Format results
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Search results for: %s\n\n", args.Query))
	for i, r := range results {
		sb.WriteString(fmt.Sprintf("%d. %s\n   %s\n   %s\n\n", i+1, r.Title, r.URL, r.Snippet))
	}

	writeResult(toolResult{Content: sb.String()})
}

// searchDDG queries DuckDuckGo's HTML search and extracts results.
func searchDDG(query string, maxResults int) ([]searchResult, error) {
	// Use DuckDuckGo's HTML-only endpoint (no JS required)
	searchURL := fmt.Sprintf("https://html.duckduckgo.com/html/?q=%s", url.QueryEscape(query))

	body, err := hostcall.HTTPGet(searchURL)
	if err != nil {
		return nil, fmt.Errorf("http get: %w", err)
	}

	return parseDDGHTML(string(body), maxResults), nil
}

// parseDDGHTML extracts search results from DuckDuckGo's HTML response.
// This is a simple parser that looks for result patterns without a full HTML parser.
func parseDDGHTML(html string, maxResults int) []searchResult {
	var results []searchResult

	// DuckDuckGo HTML results are in <a class="result__a" ...> tags
	// Each result has: result__a (title+link), result__snippet (description)
	remaining := html

	for len(results) < maxResults {
		// Find result link
		linkIdx := strings.Index(remaining, `class="result__a"`)
		if linkIdx < 0 {
			break
		}
		remaining = remaining[linkIdx:]

		// Extract href
		href := extractAttr(remaining, "href")
		// Extract title text
		title := extractTagText(remaining, "a")

		// Find snippet
		snippetIdx := strings.Index(remaining, `class="result__snippet"`)
		snippet := ""
		if snippetIdx >= 0 {
			snippet = extractTagText(remaining[snippetIdx:], "a")
			if snippet == "" {
				snippet = extractTagText(remaining[snippetIdx:], "span")
			}
		}

		// Clean up the URL — DDG wraps URLs in a redirect
		cleanURL := cleanDDGURL(href)

		if title != "" && cleanURL != "" {
			results = append(results, searchResult{
				Title:   cleanText(title),
				URL:     cleanURL,
				Snippet: cleanText(snippet),
			})
		}

		// Advance past this result
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
	// Find the closing > of the opening tag
	closeIdx := strings.Index(html, ">")
	if closeIdx < 0 {
		return ""
	}
	start := closeIdx + 1

	// Find the closing tag
	endTag := fmt.Sprintf("</%s>", tag)
	endIdx := strings.Index(html[start:], endTag)
	if endIdx < 0 {
		return ""
	}

	return html[start : start+endIdx]
}

func cleanDDGURL(rawURL string) string {
	// DDG wraps URLs: //duckduckgo.com/l/?uddg=ENCODED_URL&rut=...
	if strings.Contains(rawURL, "uddg=") {
		parsed, err := url.Parse(rawURL)
		if err == nil {
			if uddg := parsed.Query().Get("uddg"); uddg != "" {
				return uddg
			}
		}
	}
	// Direct URL
	if strings.HasPrefix(rawURL, "http") {
		return rawURL
	}
	return ""
}

func cleanText(s string) string {
	// Strip HTML tags
	var result strings.Builder
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
			result.WriteRune(r)
		}
	}
	// Clean whitespace
	text := result.String()
	text = strings.TrimSpace(text)
	// Collapse multiple spaces/newlines
	parts := strings.Fields(text)
	return strings.Join(parts, " ")
}

func writeResult(r toolResult) {
	json.NewEncoder(os.Stdout).Encode(r)
}

// web_fetch WASM tool — Fetch a URL and extract readable content as markdown.
//
// Build: GOOS=wasip1 GOARCH=wasm go build -o web_fetch.wasm .
package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"regexp"
	"strings"

	"github.com/bitop-dev/agent-core/internal/sandbox/testdata/hostcall"
)

type request struct {
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments"`
	Config    map[string]any  `json:"config"`
}

type fetchArgs struct {
	URL      string `json:"url"`
	MaxChars int    `json:"max_chars"`
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

	var args fetchArgs
	if err := json.Unmarshal(req.Arguments, &args); err != nil {
		writeResult(result{Content: "invalid arguments: " + err.Error(), IsError: true})
		return
	}

	if args.URL == "" {
		writeResult(result{Content: "url is required", IsError: true})
		return
	}

	if !strings.HasPrefix(args.URL, "http://") && !strings.HasPrefix(args.URL, "https://") {
		writeResult(result{Content: "invalid URL scheme — must start with http:// or https://", IsError: true})
		return
	}

	maxChars := args.MaxChars
	if maxChars <= 0 {
		maxChars = 20000
		if cfg, ok := req.Config["max_chars"]; ok {
			if v, ok := cfg.(float64); ok && v > 0 {
				maxChars = int(v)
			}
		}
	}

	body, err := hostcall.HTTPGet(args.URL)
	if err != nil {
		writeResult(result{Content: "fetch failed: " + err.Error(), IsError: true})
		return
	}

	html := string(body)
	markdown := htmlToMarkdown(html)

	if markdown == "" {
		writeResult(result{Content: "page returned no readable content"})
		return
	}

	if len(markdown) > maxChars {
		markdown = markdown[:maxChars] + fmt.Sprintf("\n\n[Truncated at %d characters]", maxChars)
	}

	writeResult(result{Content: markdown})
}

// htmlToMarkdown converts HTML to readable markdown text.
// Uses regex-based extraction (no external dependencies).
func htmlToMarkdown(html string) string {
	// Remove script, style, nav, footer, header, aside, svg, form
	removePatterns := []string{
		`(?is)<script[^>]*>.*?</script>`,
		`(?is)<style[^>]*>.*?</style>`,
		`(?is)<nav[^>]*>.*?</nav>`,
		`(?is)<footer[^>]*>.*?</footer>`,
		`(?is)<header[^>]*>.*?</header>`,
		`(?is)<aside[^>]*>.*?</aside>`,
		`(?is)<svg[^>]*>.*?</svg>`,
		`(?is)<form[^>]*>.*?</form>`,
		`(?is)<noscript[^>]*>.*?</noscript>`,
		`(?is)<iframe[^>]*>.*?</iframe>`,
	}

	for _, pat := range removePatterns {
		re := regexp.MustCompile(pat)
		html = re.ReplaceAllString(html, "")
	}

	// Try to extract main content area
	mainContent := extractMainContent(html)
	if mainContent != "" {
		html = mainContent
	}

	var sb strings.Builder

	// Convert headings
	for level := 1; level <= 6; level++ {
		tag := fmt.Sprintf("h%d", level)
		prefix := strings.Repeat("#", level)
		re := regexp.MustCompile(fmt.Sprintf(`(?is)<%s[^>]*>(.*?)</%s>`, tag, tag))
		html = re.ReplaceAllStringFunc(html, func(match string) string {
			text := cleanInnerText(re.FindStringSubmatch(match)[1])
			if text != "" {
				return fmt.Sprintf("\n%s %s\n", prefix, text)
			}
			return ""
		})
	}

	// Convert paragraphs
	reP := regexp.MustCompile(`(?is)<p[^>]*>(.*?)</p>`)
	html = reP.ReplaceAllStringFunc(html, func(match string) string {
		text := cleanInnerText(reP.FindStringSubmatch(match)[1])
		if text != "" {
			return "\n" + text + "\n"
		}
		return ""
	})

	// Convert list items
	reLi := regexp.MustCompile(`(?is)<li[^>]*>(.*?)</li>`)
	html = reLi.ReplaceAllStringFunc(html, func(match string) string {
		text := cleanInnerText(reLi.FindStringSubmatch(match)[1])
		if text != "" {
			return "- " + text + "\n"
		}
		return ""
	})

	// Convert code blocks
	rePre := regexp.MustCompile(`(?is)<pre[^>]*>(.*?)</pre>`)
	html = rePre.ReplaceAllStringFunc(html, func(match string) string {
		text := stripTags(rePre.FindStringSubmatch(match)[1])
		text = strings.TrimSpace(text)
		if text != "" {
			return "\n```\n" + text + "\n```\n"
		}
		return ""
	})

	// Convert links
	reA := regexp.MustCompile(`(?is)<a[^>]*href="(https?://[^"]*)"[^>]*>(.*?)</a>`)
	html = reA.ReplaceAllStringFunc(html, func(match string) string {
		parts := reA.FindStringSubmatch(match)
		if len(parts) >= 3 {
			text := cleanInnerText(parts[2])
			href := parts[1]
			if text != "" {
				return fmt.Sprintf("[%s](%s)", text, href)
			}
		}
		return ""
	})

	// Strip remaining HTML tags
	html = stripTags(html)

	// Decode common HTML entities
	html = strings.ReplaceAll(html, "&amp;", "&")
	html = strings.ReplaceAll(html, "&lt;", "<")
	html = strings.ReplaceAll(html, "&gt;", ">")
	html = strings.ReplaceAll(html, "&quot;", `"`)
	html = strings.ReplaceAll(html, "&#x27;", "'")
	html = strings.ReplaceAll(html, "&#39;", "'")
	html = strings.ReplaceAll(html, "&nbsp;", " ")

	// Clean up whitespace
	lines := strings.Split(html, "\n")
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed != "" {
			sb.WriteString(trimmed)
			sb.WriteString("\n")
		}
	}

	text := sb.String()
	// Collapse multiple blank lines
	reBlank := regexp.MustCompile(`\n{3,}`)
	text = reBlank.ReplaceAllString(text, "\n\n")

	return strings.TrimSpace(text)
}

func extractMainContent(html string) string {
	// Try common content containers
	patterns := []string{
		`(?is)<main[^>]*>(.*?)</main>`,
		`(?is)<article[^>]*>(.*?)</article>`,
		`(?is)<div[^>]*(?:class|id)="[^"]*(?:content|main|article|post)[^"]*"[^>]*>(.*?)</div>`,
	}
	for _, pat := range patterns {
		re := regexp.MustCompile(pat)
		if m := re.FindStringSubmatch(html); len(m) >= 2 {
			return m[1]
		}
	}
	// Fallback: try body
	reBody := regexp.MustCompile(`(?is)<body[^>]*>(.*?)</body>`)
	if m := reBody.FindStringSubmatch(html); len(m) >= 2 {
		return m[1]
	}
	return ""
}

func cleanInnerText(html string) string {
	text := stripTags(html)
	return strings.Join(strings.Fields(strings.TrimSpace(text)), " ")
}

func stripTags(html string) string {
	re := regexp.MustCompile(`<[^>]+>`)
	return re.ReplaceAllString(html, "")
}

func writeResult(r result) {
	json.NewEncoder(os.Stdout).Encode(r)
}

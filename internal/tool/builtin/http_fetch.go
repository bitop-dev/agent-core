package builtin

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/bitop-dev/agent-core/internal/tool"
)

type httpFetchArgs struct {
	URL     string            `json:"url"`
	Method  string            `json:"method,omitempty"`
	Headers map[string]string `json:"headers,omitempty"`
	Body    string            `json:"body,omitempty"`
}

type httpFetchTool struct{}

func NewHTTPFetch() tool.Tool { return &httpFetchTool{} }

func (t *httpFetchTool) Definition() tool.Definition {
	return tool.Definition{
		Name:        "http_fetch",
		Description: "Make an HTTP request. Returns status code, headers, and body. Supports GET and POST.",
		InputSchema: json.RawMessage(`{
			"type": "object",
			"properties": {
				"url": {
					"type": "string",
					"description": "URL to fetch"
				},
				"method": {
					"type": "string",
					"description": "HTTP method (default: GET)",
					"enum": ["GET", "POST", "PUT", "PATCH", "DELETE"]
				},
				"headers": {
					"type": "object",
					"description": "Request headers",
					"additionalProperties": {"type": "string"}
				},
				"body": {
					"type": "string",
					"description": "Request body (for POST/PUT/PATCH)"
				}
			},
			"required": ["url"]
		}`),
	}
}

func (t *httpFetchTool) Execute(ctx context.Context, input json.RawMessage) (tool.Result, error) {
	var args httpFetchArgs
	if err := json.Unmarshal(input, &args); err != nil {
		return tool.Result{Content: fmt.Sprintf("invalid arguments: %v", err), IsError: true}, nil
	}
	if args.URL == "" {
		return tool.Result{Content: "url is required", IsError: true}, nil
	}
	if args.Method == "" {
		args.Method = "GET"
	}

	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	var bodyReader io.Reader
	if args.Body != "" {
		bodyReader = strings.NewReader(args.Body)
	}

	req, err := http.NewRequestWithContext(ctx, args.Method, args.URL, bodyReader)
	if err != nil {
		return tool.Result{Content: fmt.Sprintf("invalid request: %v", err), IsError: true}, nil
	}

	for k, v := range args.Headers {
		req.Header.Set(k, v)
	}

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return tool.Result{Content: fmt.Sprintf("request failed: %v", err), IsError: true}, nil
	}
	defer resp.Body.Close()

	// Read body with size limit
	const maxBody = 512 * 1024
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxBody))
	if err != nil {
		return tool.Result{Content: fmt.Sprintf("read body failed: %v", err), IsError: true}, nil
	}

	result := fmt.Sprintf("HTTP %d %s\n\n%s", resp.StatusCode, resp.Status, string(body))
	return tool.Result{Content: result, IsError: resp.StatusCode >= 400}, nil
}

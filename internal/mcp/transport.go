package mcp

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os/exec"
	"strings"
	"sync"
	"time"
)

// Transport is the interface for MCP communication.
type Transport interface {
	// SendRecv sends a request and returns the response.
	// For notifications (ID == nil), returns an empty response.
	SendRecv(req Request) (Response, error)
	// Close shuts down the transport.
	Close() error
}

// ─── Stdio Transport ─────────────────────────────────────────────────────────

// StdioTransport spawns a local process and communicates via stdin/stdout.
type StdioTransport struct {
	mu     sync.Mutex
	cmd    *exec.Cmd
	stdin  io.WriteCloser
	reader *bufio.Reader
}

// NewStdioTransport spawns the process and returns a transport.
func NewStdioTransport(command string, args []string, env map[string]string) (*StdioTransport, error) {
	cmd := exec.Command(command, args...)

	// Set environment
	if len(env) > 0 {
		cmd.Env = cmd.Environ()
		for k, v := range env {
			cmd.Env = append(cmd.Env, fmt.Sprintf("%s=%s", k, v))
		}
	}

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("stdin pipe: %w", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("stdout pipe: %w", err)
	}
	// Inherit stderr so server logs are visible
	cmd.Stderr = nil

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start process: %w", err)
	}

	return &StdioTransport{
		cmd:    cmd,
		stdin:  stdin,
		reader: bufio.NewReaderSize(stdout, 4*1024*1024), // 4 MB buffer
	}, nil
}

func (t *StdioTransport) SendRecv(req Request) (Response, error) {
	t.mu.Lock()
	defer t.mu.Unlock()

	line, err := json.Marshal(req)
	if err != nil {
		return Response{}, fmt.Errorf("marshal request: %w", err)
	}

	if _, err := t.stdin.Write(append(line, '\n')); err != nil {
		return Response{}, fmt.Errorf("write to stdin: %w", err)
	}

	// Notifications don't get a response
	if req.ID == nil {
		return Response{JSONRPC: JSONRPCVersion}, nil
	}

	respLine, err := t.reader.ReadBytes('\n')
	if err != nil {
		return Response{}, fmt.Errorf("read from stdout: %w", err)
	}

	var resp Response
	if err := json.Unmarshal(respLine, &resp); err != nil {
		return Response{}, fmt.Errorf("unmarshal response: %w (raw: %s)", err, string(respLine))
	}
	return resp, nil
}

func (t *StdioTransport) Close() error {
	t.stdin.Close()
	return t.cmd.Process.Kill()
}

// ─── HTTP Transport ──────────────────────────────────────────────────────────

// HTTPTransport uses HTTP POST for MCP communication.
// Works with Streamable HTTP servers (MCP 2025-03-26+) and simple HTTP endpoints.
type HTTPTransport struct {
	url     string
	client  *http.Client
	headers map[string]string
	mu      sync.Mutex
}

// NewHTTPTransport creates an HTTP transport for the given URL.
func NewHTTPTransport(url string, headers map[string]string) *HTTPTransport {
	return &HTTPTransport{
		url: url,
		client: &http.Client{
			Timeout: 120 * time.Second,
		},
		headers: headers,
	}
}

func (t *HTTPTransport) SendRecv(req Request) (Response, error) {
	t.mu.Lock()
	defer t.mu.Unlock()

	body, err := json.Marshal(req)
	if err != nil {
		return Response{}, fmt.Errorf("marshal request: %w", err)
	}

	httpReq, err := http.NewRequest("POST", t.url, bytes.NewReader(body))
	if err != nil {
		return Response{}, fmt.Errorf("create request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "application/json, text/event-stream")
	for k, v := range t.headers {
		httpReq.Header.Set(k, v)
	}

	resp, err := t.client.Do(httpReq)
	if err != nil {
		return Response{}, fmt.Errorf("http request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return Response{}, fmt.Errorf("MCP server returned HTTP %d: %s", resp.StatusCode, string(bodyBytes))
	}

	// Notifications don't expect a response
	if req.ID == nil {
		return Response{JSONRPC: JSONRPCVersion}, nil
	}

	// Check if response is SSE or plain JSON
	ct := resp.Header.Get("Content-Type")
	if strings.Contains(strings.ToLower(ct), "text/event-stream") {
		return readFirstSSEResponse(resp.Body)
	}

	// Plain JSON response
	respBody, err := io.ReadAll(io.LimitReader(resp.Body, 4*1024*1024))
	if err != nil {
		return Response{}, fmt.Errorf("read response: %w", err)
	}

	var rpcResp Response
	if err := json.Unmarshal(respBody, &rpcResp); err != nil {
		return Response{}, fmt.Errorf("unmarshal response: %w (raw: %s)", err, string(respBody))
	}
	return rpcResp, nil
}

func (t *HTTPTransport) Close() error {
	return nil
}

// readFirstSSEResponse reads SSE data and extracts the first JSON-RPC response.
func readFirstSSEResponse(r io.Reader) (Response, error) {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 4*1024*1024), 4*1024*1024)

	var dataLines []string

	for scanner.Scan() {
		line := strings.TrimRight(scanner.Text(), "\r")

		if line == "" {
			// End of event — process accumulated data
			if len(dataLines) > 0 {
				data := strings.Join(dataLines, "\n")
				dataLines = nil

				data = strings.TrimSpace(data)
				if data == "" {
					continue
				}

				var resp Response
				if err := json.Unmarshal([]byte(data), &resp); err == nil {
					return resp, nil
				}
			}
			continue
		}

		if strings.HasPrefix(line, ":") {
			continue // comment
		}

		if strings.HasPrefix(line, "data:") {
			d := strings.TrimPrefix(line, "data:")
			d = strings.TrimPrefix(d, " ")
			dataLines = append(dataLines, d)
		}
	}

	// Try any remaining data
	if len(dataLines) > 0 {
		data := strings.TrimSpace(strings.Join(dataLines, "\n"))
		var resp Response
		if err := json.Unmarshal([]byte(data), &resp); err == nil {
			return resp, nil
		}
	}

	return Response{}, fmt.Errorf("no JSON-RPC response found in SSE stream")
}

package output

import (
	"fmt"
	"io"
	"strings"

	"github.com/bitop-dev/agent-core/internal/agent"
)

// TextRenderer renders events as human-readable colored terminal output.
type TextRenderer struct {
	out io.Writer // main output (stdout) — text deltas
	err io.Writer // status output (stderr) — tool calls, metadata
}

// NewTextRenderer creates a text renderer writing to the given streams.
func NewTextRenderer(out, errOut io.Writer) *TextRenderer {
	return &TextRenderer{out: out, err: errOut}
}

func (r *TextRenderer) Render(event agent.RunEvent) {
	switch event.Type {
	case agent.EventTextDelta:
		if data, ok := event.Data.(agent.TextDeltaData); ok {
			fmt.Fprint(r.out, data.Text)
		}

	case agent.EventToolCallStart:
		if data, ok := event.Data.(agent.ToolCallStartData); ok {
			fmt.Fprintf(r.err, "\033[36m⚙ %s\033[0m(%s)\n", data.ToolName, truncate(data.Arguments, 100))
		}

	case agent.EventToolCallEnd:
		if data, ok := event.Data.(agent.ToolCallEndData); ok {
			if data.IsError {
				fmt.Fprintf(r.err, "\033[31m✗ %s: %s\033[0m\n", data.ToolName, truncate(data.Content, 200))
			} else {
				fmt.Fprintf(r.err, "\033[32m✓ %s\033[0m (%s)\n", data.ToolName, truncate(data.Content, 100))
			}
		}

	case agent.EventContextCompact:
		fmt.Fprintf(r.err, "\033[33m⟳ compacting conversation history...\033[0m\n")

	case agent.EventLoopDetected:
		fmt.Fprintf(r.err, "\033[33m⚠ loop detected: %v\033[0m\n", event.Data)

	case agent.EventApprovalDenied:
		if data, ok := event.Data.(agent.ToolCallStartData); ok {
			fmt.Fprintf(r.err, "\033[31m⊘ %s denied\033[0m\n", data.ToolName)
		}

	case agent.EventDeferredAction:
		fmt.Fprintf(r.err, "\033[33m↻ deferred action detected, nudging...\033[0m\n")

	case agent.EventHeartbeat:
		fmt.Fprintf(r.err, "\033[90m♡ safety heartbeat\033[0m\n")

	case agent.EventError:
		fmt.Fprintf(r.err, "\033[31merror: %v\033[0m\n", event.Data)

	case agent.EventAgentEnd:
		if data, ok := event.Data.(agent.AgentEndData); ok {
			fmt.Fprintf(r.err, "\n\033[90m--- %s | %d turns | %dms ---\033[0m\n",
				data.StopReason, data.TotalTurns, data.DurationMs)
		}
	}
}

func (r *TextRenderer) Flush() {}

func truncate(s string, max int) string {
	s = strings.ReplaceAll(s, "\n", " ")
	if len(s) > max {
		return s[:max] + "…"
	}
	return s
}

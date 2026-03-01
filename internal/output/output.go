// Package output handles rendering agent events to the terminal or files.
package output

import (
	"github.com/bitop-dev/agent-core/internal/agent"
)

// Renderer consumes agent RunEvents and renders them to output.
type Renderer interface {
	Render(event agent.RunEvent)
	Flush()
}

// Package session manages conversation sessions with JSONL persistence.
package session

import (
	"github.com/bitop-dev/agent-core/internal/provider"
)

// Session holds the conversation state for a multi-turn chat.
type Session struct {
	ID       string
	Messages []provider.Message
	Metadata map[string]string
}

// New creates a new empty session.
func New(id string) *Session {
	return &Session{
		ID:       id,
		Messages: nil,
		Metadata: make(map[string]string),
	}
}

// Append adds a message to the session history.
func (s *Session) Append(msg provider.Message) {
	s.Messages = append(s.Messages, msg)
}

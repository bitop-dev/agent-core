// Package session manages conversation sessions with JSONL persistence.
package session

import (
	"crypto/rand"
	"encoding/hex"
	"time"

	"github.com/bitop-dev/agent-core/internal/provider"
)

// Session holds the conversation state for a multi-turn chat.
type Session struct {
	ID        string
	CreatedAt time.Time
	Messages  []provider.Message
	Metadata  map[string]string
}

// New creates a new empty session.
func New(id string) *Session {
	return &Session{
		ID:        id,
		CreatedAt: time.Now(),
		Messages:  nil,
		Metadata:  make(map[string]string),
	}
}

// Append adds a message to the session history.
func (s *Session) Append(msg provider.Message) {
	s.Messages = append(s.Messages, msg)
}

// GenerateID creates a short random session ID.
func GenerateID() string {
	b := make([]byte, 4)
	rand.Read(b)
	return hex.EncodeToString(b)
}

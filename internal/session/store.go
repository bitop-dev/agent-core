package session

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/bitop-dev/agent-core/internal/provider"
)

// Store persists sessions to disk as JSONL files.
// Sessions are stored at ~/.agent-core/sessions/{id}.jsonl
type Store struct {
	dir string
}

// sessionFile is the JSONL format — one line per message.
type sessionLine struct {
	Timestamp time.Time              `json:"ts"`
	Role      string                 `json:"role"`
	Content   []provider.ContentBlock `json:"content"`
}

// sessionMeta is stored as the first line of the JSONL file.
type sessionMeta struct {
	ID        string    `json:"id"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	Model     string    `json:"model,omitempty"`
	Agent     string    `json:"agent,omitempty"`
}

// NewStore creates a session store at the given directory.
// Creates the directory if it doesn't exist.
func NewStore(dir string) (*Store, error) {
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("create session dir: %w", err)
	}
	return &Store{dir: dir}, nil
}

// DefaultDir returns the default session directory (~/.agent-core/sessions/).
func DefaultDir() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".agent-core", "sessions")
}

// Save writes a session's messages to disk as JSONL.
func (s *Store) Save(sess *Session) error {
	path := s.path(sess.ID)
	f, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("create session file: %w", err)
	}
	defer f.Close()

	w := bufio.NewWriter(f)

	// Write metadata as first line
	meta := sessionMeta{
		ID:        sess.ID,
		CreatedAt: sess.CreatedAt,
		UpdatedAt: time.Now(),
		Model:     sess.Metadata["model"],
		Agent:     sess.Metadata["agent"],
	}
	metaLine, _ := json.Marshal(meta)
	fmt.Fprintf(w, "#meta:%s\n", metaLine)

	// Write each message as a JSONL line
	for _, msg := range sess.Messages {
		line := sessionLine{
			Timestamp: time.Now(),
			Role:      string(msg.Role),
			Content:   msg.Content,
		}
		data, err := json.Marshal(line)
		if err != nil {
			continue
		}
		w.Write(data)
		w.WriteByte('\n')
	}

	return w.Flush()
}

// Load reads a session from disk.
func (s *Store) Load(id string) (*Session, error) {
	path := s.path(id)
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("session %q not found", id)
		}
		return nil, fmt.Errorf("open session: %w", err)
	}
	defer f.Close()

	sess := New(id)
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 1024*1024), 1024*1024)

	for scanner.Scan() {
		line := scanner.Text()

		// Skip metadata line
		if strings.HasPrefix(line, "#meta:") {
			metaJSON := strings.TrimPrefix(line, "#meta:")
			var meta sessionMeta
			if json.Unmarshal([]byte(metaJSON), &meta) == nil {
				sess.CreatedAt = meta.CreatedAt
				if meta.Model != "" {
					sess.Metadata["model"] = meta.Model
				}
				if meta.Agent != "" {
					sess.Metadata["agent"] = meta.Agent
				}
			}
			continue
		}

		if line == "" {
			continue
		}

		var sl sessionLine
		if err := json.Unmarshal([]byte(line), &sl); err != nil {
			continue
		}

		sess.Messages = append(sess.Messages, provider.Message{
			Role:    provider.Role(sl.Role),
			Content: sl.Content,
		})
	}

	return sess, scanner.Err()
}

// Exists checks if a session exists on disk.
func (s *Store) Exists(id string) bool {
	_, err := os.Stat(s.path(id))
	return err == nil
}

// List returns all session IDs, sorted by modification time (newest first).
func (s *Store) List() ([]SessionInfo, error) {
	entries, err := os.ReadDir(s.dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var infos []SessionInfo
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".jsonl") {
			continue
		}
		id := strings.TrimSuffix(entry.Name(), ".jsonl")
		info, err := entry.Info()
		if err != nil {
			continue
		}
		infos = append(infos, SessionInfo{
			ID:       id,
			Modified: info.ModTime(),
			Size:     info.Size(),
		})
	}

	sort.Slice(infos, func(i, j int) bool {
		return infos[i].Modified.After(infos[j].Modified)
	})

	return infos, nil
}

// Delete removes a session from disk.
func (s *Store) Delete(id string) error {
	return os.Remove(s.path(id))
}

func (s *Store) path(id string) string {
	return filepath.Join(s.dir, id+".jsonl")
}

// SessionInfo is a summary of a session for listing.
type SessionInfo struct {
	ID       string
	Modified time.Time
	Size     int64
}

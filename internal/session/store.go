package session

// Store persists sessions to disk as JSONL files.
// Sessions are stored at ~/.agent-core/sessions/{id}.jsonl
type Store struct {
	dir string // base directory for session files
}

// NewStore creates a session store at the given directory.
func NewStore(dir string) *Store {
	return &Store{dir: dir}
}

// TODO: Save, Load, List, Delete methods

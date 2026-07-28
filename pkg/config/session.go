package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"
)

// ReencryptAllSessions rewrites every session file under SessionsDir from
// oldKey to newKey (either may be nil for plaintext). Used when the user
// toggles storage encryption on/off or changes their passphrase (FEAT-16);
// safe to retry — see the equivalent doc on portfolio.ReencryptAllPortfolios
// for the idempotent-retry algorithm this mirrors.
func ReencryptAllSessions(oldKey, newKey []byte) error {
	dir, err := SessionsDir()
	if err != nil {
		return err
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return fmt.Errorf("config: ReencryptAllSessions: %w", err)
	}
	for _, e := range entries {
		if e.IsDir() || filepath.Ext(e.Name()) != ".json" {
			continue
		}
		path := filepath.Join(dir, e.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("config: ReencryptAllSessions: read %s: %w", e.Name(), err)
		}
		var s Session
		if err := UnmarshalEncryptable(data, &s, oldKey); err != nil {
			if newKey == nil {
				return fmt.Errorf("config: ReencryptAllSessions: %s: %w", e.Name(), err)
			}
			if err := UnmarshalEncryptable(data, &s, newKey); err != nil {
				return fmt.Errorf("config: ReencryptAllSessions: %s: %w", e.Name(), err)
			}
			continue // already migrated in a prior partial run
		}
		out, err := MarshalEncryptable(&s, newKey)
		if err != nil {
			return fmt.Errorf("config: ReencryptAllSessions: marshal %s: %w", e.Name(), err)
		}
		if err := WriteFileAtomic(path, out, 0o600); err != nil {
			return fmt.Errorf("config: ReencryptAllSessions: write %s: %w", e.Name(), err)
		}
	}
	return nil
}

// Session is a single agent-chat session with its own history and metadata.
type Session struct {
	ID          string     `json:"id"`
	PortfolioID string     `json:"portfolio_id,omitempty"` // "" = global session
	Title       string     `json:"title"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
	History     []ChatTurn `json:"history"`
}

// SessionsDir returns the directory where session files are stored.
func SessionsDir() (string, error) {
	p, err := Path()
	if err != nil {
		return "", fmt.Errorf("config: SessionsDir: %w", err)
	}
	return filepath.Join(filepath.Dir(p), "sessions"), nil
}

// NewSession creates a new session with a timestamp-based ID (with 16 bits
// of random entropy so two creations in the same second don't collide — see
// issue #32) and the placeholder title "new_session".
func NewSession() *Session {
	now := time.Now()
	return &Session{
		ID:        NewTimestampID(),
		Title:     "new_session",
		CreatedAt: now,
		UpdatedAt: now,
		History:   nil,
	}
}

// SaveSession writes a session to disk as a JSON file. On first save (the
// destination doesn't exist yet) it uses O_EXCL semantics so a stale or
// colliding ID never silently overwrites an existing session; on subsequent
// saves (updating an existing session) it overwrites as today. See
// issue #32.
func SaveSession(s *Session) error {
	dir, err := SessionsDir()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("config: SaveSession: mkdir: %w", err)
	}
	data, err := MarshalEncryptable(s, StorageKey())
	if err != nil {
		return fmt.Errorf("config: SaveSession: marshal: %w", err)
	}
	p := filepath.Join(dir, s.ID+".json")
	_, statErr := os.Stat(p)
	switch {
	case statErr == nil:
		if err := WriteFileAtomic(p, data, 0o600); err != nil {
			return fmt.Errorf("config: SaveSession: write: %w", err)
		}
	case errors.Is(statErr, os.ErrNotExist):
		if err := WriteFileNew(p, data, 0o600); err != nil {
			return fmt.Errorf("config: SaveSession: write: %w", err)
		}
	default:
		return fmt.Errorf("config: SaveSession: stat: %w", statErr)
	}
	return nil
}

// LoadSession reads a session from disk by its ID. Returns nil and no error if
// the file does not exist.
func LoadSession(id string) (*Session, error) {
	dir, err := SessionsDir()
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(filepath.Join(dir, id+".json"))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("config: LoadSession: %w", err)
	}
	var s Session
	if err := UnmarshalEncryptable(data, &s, StorageKey()); err != nil {
		return nil, fmt.Errorf("config: LoadSession: unmarshal: %w", err)
	}
	return &s, nil
}

// ListSessions returns all global sessions (PortfolioID == "") from disk sorted
// by UpdatedAt descending (most recent first).
func ListSessions() ([]*Session, error) {
	return listSessionsWhere(func(s *Session) bool { return s.PortfolioID == "" })
}

// ListPortfolioSessions returns all sessions belonging to the given portfolio,
// sorted by UpdatedAt descending (most recent first).
func ListPortfolioSessions(portfolioID string) ([]*Session, error) {
	return listSessionsWhere(func(s *Session) bool { return s.PortfolioID == portfolioID })
}

func listSessionsWhere(keep func(*Session) bool) ([]*Session, error) {
	dir, err := SessionsDir()
	if err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("config: ListSessions: %w", err)
	}
	var sessions []*Session
	for _, e := range entries {
		if e.IsDir() || filepath.Ext(e.Name()) != ".json" {
			continue
		}
		id := e.Name()[:len(e.Name())-5] // strip .json
		s, err := LoadSession(id)
		if err != nil || s == nil {
			continue
		}
		if keep(s) {
			sessions = append(sessions, s)
		}
	}
	sort.Slice(sessions, func(i, j int) bool {
		return sessions[i].UpdatedAt.After(sessions[j].UpdatedAt)
	})
	return sessions, nil
}

// DeleteSession removes a session file from disk.
func DeleteSession(id string) error {
	dir, err := SessionsDir()
	if err != nil {
		return err
	}
	p := filepath.Join(dir, id+".json")
	if err := os.Remove(p); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("config: DeleteSession: %w", err)
	}
	return nil
}

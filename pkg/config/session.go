package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"
)

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

// NewSession creates a new session with a timestamp-based ID and the
// placeholder title "new_session".
func NewSession() *Session {
	now := time.Now()
	return &Session{
		ID:        now.Format("20060102-150405"),
		Title:     "new_session",
		CreatedAt: now,
		UpdatedAt: now,
		History:   nil,
	}
}

// SaveSession writes a session to disk as a JSON file.
func SaveSession(s *Session) error {
	dir, err := SessionsDir()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("config: SaveSession: mkdir: %w", err)
	}
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return fmt.Errorf("config: SaveSession: marshal: %w", err)
	}
	p := filepath.Join(dir, s.ID+".json")
	if err := os.WriteFile(p, data, 0o600); err != nil {
		return fmt.Errorf("config: SaveSession: write: %w", err)
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
	if err := json.Unmarshal(data, &s); err != nil {
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

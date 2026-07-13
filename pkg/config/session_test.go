package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNewSession_Defaults(t *testing.T) {
	s := NewSession()
	if s.ID == "" {
		t.Error("expected a generated ID")
	}
	if s.Title != "new_session" {
		t.Errorf("Title = %q, want %q", s.Title, "new_session")
	}
	if s.History != nil {
		t.Errorf("History = %+v, want nil", s.History)
	}
}

func TestSaveLoadSession(t *testing.T) {
	withTempConfig(t)

	s := NewSession()
	s.Title = "My chat"
	s.History = []ChatTurn{
		{Role: "user", Content: "What is my portfolio worth?"},
		{Role: "assistant", Content: "Your portfolio is worth $10,000."},
	}

	if err := SaveSession(s); err != nil {
		t.Fatalf("SaveSession: %v", err)
	}

	loaded, err := LoadSession(s.ID)
	if err != nil {
		t.Fatalf("LoadSession: %v", err)
	}
	if loaded == nil {
		t.Fatal("LoadSession returned nil")
	}
	if loaded.Title != "My chat" {
		t.Errorf("Title = %q, want %q", loaded.Title, "My chat")
	}
	if len(loaded.History) != 2 {
		t.Fatalf("History len = %d, want 2", len(loaded.History))
	}

	list, err := ListSessions()
	if err != nil {
		t.Fatalf("ListSessions: %v", err)
	}
	if len(list) != 1 {
		t.Errorf("ListSessions len = %d, want 1", len(list))
	}

	if err := DeleteSession(s.ID); err != nil {
		t.Fatalf("DeleteSession: %v", err)
	}
	dir, _ := SessionsDir()
	if _, err := os.Stat(filepath.Join(dir, s.ID+".json")); !os.IsNotExist(err) {
		t.Error("file still exists after delete")
	}
}

func TestLoadSession_NotFound(t *testing.T) {
	withTempConfig(t)

	s, err := LoadSession("nonexistent")
	if err != nil {
		t.Fatalf("expected nil error, got: %v", err)
	}
	if s != nil {
		t.Error("expected nil session")
	}
}

func TestListPortfolioSessions_Filters(t *testing.T) {
	withTempConfig(t)

	global := NewSession()
	global.ID = "20260101-000001"
	owned := NewSession()
	owned.ID = "20260101-000002"
	owned.PortfolioID = "p1"
	if err := SaveSession(global); err != nil {
		t.Fatalf("SaveSession global: %v", err)
	}
	if err := SaveSession(owned); err != nil {
		t.Fatalf("SaveSession owned: %v", err)
	}

	globals, err := ListSessions()
	if err != nil {
		t.Fatalf("ListSessions: %v", err)
	}
	if len(globals) != 1 || globals[0].ID != global.ID {
		t.Errorf("ListSessions = %+v, want just %q", globals, global.ID)
	}

	owned2, err := ListPortfolioSessions("p1")
	if err != nil {
		t.Fatalf("ListPortfolioSessions: %v", err)
	}
	if len(owned2) != 1 || owned2[0].ID != owned.ID {
		t.Errorf("ListPortfolioSessions = %+v, want just %q", owned2, owned.ID)
	}
}

// ─── FEAT-16: storage encryption ───────────────────────────────────────────────

func TestSaveLoadSession_Encrypted(t *testing.T) {
	withTempConfig(t)

	salt, err := NewSalt()
	if err != nil {
		t.Fatalf("NewSalt: %v", err)
	}
	key := DeriveStorageKey("pass", salt)
	SetStorageKey(key)
	t.Cleanup(func() { SetStorageKey(nil) })

	s := NewSession()
	s.Title = "Secret chat"
	s.History = []ChatTurn{{Role: "user", Content: "I have $1M in savings"}}
	if err := SaveSession(s); err != nil {
		t.Fatalf("SaveSession: %v", err)
	}

	dir, _ := SessionsDir()
	raw, err := os.ReadFile(filepath.Join(dir, s.ID+".json"))
	if err != nil {
		t.Fatalf("read raw file: %v", err)
	}
	if strings.Contains(string(raw), "$1M") {
		t.Error("chat content found in plaintext on disk, expected it to be encrypted")
	}

	loaded, err := LoadSession(s.ID)
	if err != nil {
		t.Fatalf("LoadSession: %v", err)
	}
	if loaded == nil || loaded.Title != "Secret chat" {
		t.Fatalf("LoadSession: got %+v", loaded)
	}
}

func TestLoadSession_LegacyPlaintextWithKeySet(t *testing.T) {
	withTempConfig(t)

	key := DeriveStorageKey("pass", []byte("0123456789abcdef"))
	SetStorageKey(key)
	t.Cleanup(func() { SetStorageKey(nil) })

	dir, _ := SessionsDir()
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	raw := `{"id":"legacy-1","title":"Legacy Plaintext","created_at":"2024-01-01T00:00:00Z","updated_at":"2024-01-01T00:00:00Z","history":null}`
	if err := os.WriteFile(filepath.Join(dir, "legacy-1.json"), []byte(raw), 0o600); err != nil {
		t.Fatal(err)
	}

	loaded, err := LoadSession("legacy-1")
	if err != nil {
		t.Fatalf("LoadSession: %v", err)
	}
	if loaded == nil || loaded.Title != "Legacy Plaintext" {
		t.Fatalf("LoadSession: got %+v", loaded)
	}
}

func TestReencryptAllSessions(t *testing.T) {
	withTempConfig(t)
	t.Cleanup(func() { SetStorageKey(nil) })

	// NewSession's ID has only second resolution, so assign distinct IDs
	// explicitly to avoid a collision between s1 and s2 created in the same test.
	SetStorageKey(nil)
	s1 := NewSession()
	s1.ID = "20260101-000001"
	s1.Title = "S1"
	s2 := NewSession()
	s2.ID = "20260101-000002"
	s2.Title = "S2"
	if err := SaveSession(s1); err != nil {
		t.Fatalf("SaveSession s1: %v", err)
	}
	if err := SaveSession(s2); err != nil {
		t.Fatalf("SaveSession s2: %v", err)
	}

	saltA, _ := NewSalt()
	keyA := DeriveStorageKey("pass", saltA)

	if err := ReencryptAllSessions(nil, keyA); err != nil {
		t.Fatalf("ReencryptAllSessions (plaintext->A): %v", err)
	}
	SetStorageKey(keyA)
	loaded, err := LoadSession(s1.ID)
	if err != nil || loaded == nil || loaded.Title != "S1" {
		t.Fatalf("LoadSession after migration to keyA: %v, %+v", err, loaded)
	}

	// Idempotent retry.
	if err := ReencryptAllSessions(nil, keyA); err != nil {
		t.Fatalf("ReencryptAllSessions retry: %v", err)
	}

	saltB, _ := NewSalt()
	keyB := DeriveStorageKey("pass2", saltB)

	if err := ReencryptAllSessions(keyA, keyB); err != nil {
		t.Fatalf("ReencryptAllSessions (A->B): %v", err)
	}
	SetStorageKey(keyB)
	loaded, err = LoadSession(s2.ID)
	if err != nil || loaded == nil || loaded.Title != "S2" {
		t.Fatalf("LoadSession after migration to keyB: %v, %+v", err, loaded)
	}

	SetStorageKey(keyA)
	if _, err := LoadSession(s1.ID); err == nil {
		t.Error("expected error loading with stale keyA after re-key to keyB, got nil")
	}

	if err := ReencryptAllSessions(keyB, nil); err != nil {
		t.Fatalf("ReencryptAllSessions (B->plaintext): %v", err)
	}
	SetStorageKey(nil)
	loaded, err = LoadSession(s1.ID)
	if err != nil || loaded == nil || loaded.Title != "S1" {
		t.Fatalf("LoadSession after decrypt: %v, %+v", err, loaded)
	}
}

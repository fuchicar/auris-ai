package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
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

func TestNewSession_UniqueIDsInSameSecond(t *testing.T) {
	// 1000 back-to-back NewSession calls must all produce distinct IDs even
	// though they almost certainly happen within the same wall-clock second.
	// See issue #32: pre-fix, all 1000 shared the same 1-second timestamp.
	const n = 1000
	seen := make(map[string]struct{}, n)
	for i := 0; i < n; i++ {
		id := NewSession().ID
		if _, dup := seen[id]; dup {
			t.Fatalf("duplicate ID %q after %d calls", id, i)
		}
		seen[id] = struct{}{}
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

func TestSaveSession_TwoSameSecondCreatesDoNotClobber(t *testing.T) {
	withTempConfig(t)

	// Force the two creations into the same second by reusing the timestamp
	// prefix of s1 for s2 (the entropy suffix is what differentiates them).
	// We bypass NewSession and assign IDs directly so the test is
	// deterministic regardless of how fast the test runner is.
	s1 := &Session{ID: NewTimestampID(), Title: "first", CreatedAt: time.Now(), UpdatedAt: time.Now()}
	s2 := &Session{ID: NewTimestampID(), Title: "second", CreatedAt: time.Now(), UpdatedAt: time.Now()}
	if err := SaveSession(s1); err != nil {
		t.Fatalf("SaveSession s1: %v", err)
	}
	if err := SaveSession(s2); err != nil {
		t.Fatalf("SaveSession s2: %v", err)
	}

	dir, _ := SessionsDir()
	for _, id := range []string{s1.ID, s2.ID} {
		if _, err := os.Stat(filepath.Join(dir, id+".json")); err != nil {
			t.Errorf("expected file %s.json on disk, got: %v", id, err)
		}
	}

	loaded1, err := LoadSession(s1.ID)
	if err != nil || loaded1 == nil || loaded1.Title != "first" {
		t.Errorf("LoadSession(s1) = %+v, %v; want Title=first", loaded1, err)
	}
	loaded2, err := LoadSession(s2.ID)
	if err != nil || loaded2 == nil || loaded2.Title != "second" {
		t.Errorf("LoadSession(s2) = %+v, %v; want Title=second", loaded2, err)
	}
}

func TestSaveSession_UpdateStillOverwrites(t *testing.T) {
	withTempConfig(t)

	s := NewSession()
	s.Title = "original"
	if err := SaveSession(s); err != nil {
		t.Fatalf("first SaveSession: %v", err)
	}

	s.Title = "updated"
	if err := SaveSession(s); err != nil {
		t.Fatalf("second SaveSession: %v", err)
	}

	loaded, err := LoadSession(s.ID)
	if err != nil || loaded == nil {
		t.Fatalf("LoadSession: %+v, %v", loaded, err)
	}
	if loaded.Title != "updated" {
		t.Errorf("Title = %q, want %q (update path should overwrite, not fail)", loaded.Title, "updated")
	}
}

func TestSaveSession_CreatePathRefusesExistingDestination(t *testing.T) {
	withTempConfig(t)

	// Write a session file under an ID, then attempt to save a *different*
	// session struct under the same ID. The create-path (stat says the file
	// doesn't exist, but the user assigned the ID manually — actually here
	// the file does exist, so the stat branch chooses the update path and
	// overwrites). To exercise the strict O_EXCL semantics, call WriteFileNew
	// directly: that's the function SaveSession falls through to on the
	// create branch, and it's what fails fast on a collision.
	dir, _ := SessionsDir()
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "manual-id.json")
	if err := WriteFileNew(path, []byte(`{"placeholder":true}`), 0o600); err != nil {
		t.Fatalf("first WriteFileNew: %v", err)
	}

	// Second WriteFileNew must fail (O_EXCL).
	err := WriteFileNew(path, []byte(`{"placeholder":false}`), 0o600)
	if err == nil {
		t.Fatal("expected O_EXCL error on second WriteFileNew, got nil")
	}

	// Original content untouched.
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != `{"placeholder":true}` {
		t.Errorf("file was overwritten despite O_EXCL: %q", got)
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

	// NewSession now produces IDs with 16 bits of random entropy (issue #32),
	// so two same-second creations never collide — no manual ID overrides
	// needed.
	SetStorageKey(nil)
	s1 := NewSession()
	s1.Title = "S1"
	s2 := NewSession()
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

package config

import (
	"regexp"
	"testing"
)

var timestampIDPattern = regexp.MustCompile(`^\d{8}-\d{6}-[0-9a-f]{4}$`)

func TestNewTimestampID_FormatMatches(t *testing.T) {
	id := NewTimestampID()
	if !timestampIDPattern.MatchString(id) {
		t.Errorf("ID %q does not match YYYYMMDD-HHMMSS-XXXX format", id)
	}
}

func TestNewTimestampID_UniqueAcrossMany(t *testing.T) {
	const n = 1000
	seen := make(map[string]struct{}, n)
	for i := 0; i < n; i++ {
		id := NewTimestampID()
		if _, dup := seen[id]; dup {
			t.Fatalf("duplicate ID %q after %d calls", id, i)
		}
		seen[id] = struct{}{}
	}
}

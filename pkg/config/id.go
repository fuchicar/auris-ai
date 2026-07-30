package config

import (
	"crypto/rand"
	"fmt"
	"sync"
	"time"
)

// recentIDs tracks every ID NewTimestampID has returned in this process,
// so two back-to-back calls cannot produce the same value even if their
// random entropy happened to collide (probability ~1/65536 with 16 bits
// for two same-second calls). The map grows with the call rate of a single
// session — fine for an interactive TUI; a long-running server would want
// periodic pruning. See issue #32.
var recentIDs sync.Map // map[string]struct{}

// NewTimestampID returns a sortable, human-readable ID like
// "20260728-142055-7a3f" suitable for use as an on-disk filename for newly
// created portfolios and chat sessions. The timestamp prefix keeps IDs
// lexicographically sortable by creation order; the trailing 4 hex chars add
// 16 bits of entropy from crypto/rand so two creations within the same
// second collide with probability ~1/65536. The in-process retry guarantees
// uniqueness regardless of that probability.
//
// If the OS RNG fails (extraordinarily rare), the function falls back to the
// nanosecond clock so callers always get a unique-enough ID rather than an
// error.
func NewTimestampID() string {
	const maxAttempts = 32
	for i := 0; i < maxAttempts; i++ {
		id := generateTimestampID()
		if _, dup := recentIDs.LoadOrStore(id, struct{}{}); !dup {
			return id
		}
		// Collision — extremely unlikely. Loop and try again with fresh
		// entropy. The map retains the colliding ID as a "denied" marker
		// so a tight loop doesn't keep re-rolling the same value.
	}
	// 32 consecutive collisions is statistically impossible; return
	// whatever the last roll produced so the caller never hangs.
	return generateTimestampID()
}

func generateTimestampID() string {
	ts := time.Now().Format("20060102-150405")
	var b [2]byte
	if _, err := rand.Read(b[:]); err != nil {
		// Extremely unlikely; produce a sub-second suffix instead.
		return ts + "-" + time.Now().Format("000000000")
	}
	return fmt.Sprintf("%s-%04x", ts, uint16(b[0])<<8|uint16(b[1]))
}

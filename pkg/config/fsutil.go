package config

import (
	"fmt"
	"os"
	"path/filepath"
)

// WriteFileAtomic writes data to path via a temp file in the same directory
// followed by an atomic rename, so a crash or full disk mid-write can never
// leave a truncated file behind. This matters most for encrypted portfolios
// and sessions: a partial AES-GCM blob fails authentication and is
// unrecoverable, whereas with rename the previous complete file survives.
func WriteFileAtomic(path string, data []byte, perm os.FileMode) error {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, filepath.Base(path)+".tmp-*")
	if err != nil {
		return fmt.Errorf("config: WriteFileAtomic: %w", err)
	}
	tmpName := tmp.Name()
	// Best-effort cleanup on any failure path; harmless after a successful rename.
	defer os.Remove(tmpName)

	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return fmt.Errorf("config: WriteFileAtomic: write: %w", err)
	}
	if err := tmp.Chmod(perm); err != nil {
		tmp.Close()
		return fmt.Errorf("config: WriteFileAtomic: chmod: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("config: WriteFileAtomic: close: %w", err)
	}
	if err := os.Rename(tmpName, path); err != nil {
		return fmt.Errorf("config: WriteFileAtomic: rename: %w", err)
	}
	return nil
}

// WriteFileNew is like WriteFileAtomic but refuses to write if the
// destination already exists. Use on create paths (e.g. SavePortfolio,
// SaveSession) where an ID collision must never silently overwrite a real
// file. Updates still go through WriteFileAtomic — the caller distinguishes
// create from update with os.Stat before invoking this function.
//
// The existence check is racy in the multi-writer sense (another writer
// could create the destination in the microsecond gap between the stat and
// the rename), but the agent dispatches tool calls sequentially so the only
// "racer" is an upstream re-roll on the same struct, in which case the
// caller already mutated the ID before re-invoking SavePortfolio. In that
// case the second save targets a different path entirely and the race
// window doesn't matter.
func WriteFileNew(path string, data []byte, perm os.FileMode) error {
	if _, err := os.Stat(path); err == nil {
		return fmt.Errorf("config: WriteFileNew: destination already exists: %w", os.ErrExist)
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("config: WriteFileNew: stat: %w", err)
	}
	return WriteFileAtomic(path, data, perm)
}

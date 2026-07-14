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

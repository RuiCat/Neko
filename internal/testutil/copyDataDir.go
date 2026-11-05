// Package testutil holds filesystem helpers shared by integration tests.
package testutil

import (
	"io/fs"
	"os"
	"path/filepath"
	"testing"
)

// CopyDataDir copies the regular files and directories under src into dst.
// Non-regular entries are skipped on purpose: when a live engine runs, data/
// contains the control-plane Unix socket (and its lock), and os.CopyFS refuses
// sockets with "invalid argument". Tests therefore copy only what a data dir
// is expected to persist.
func CopyDataDir(t testing.TB, src, dst string) {
	t.Helper()
	err := filepath.WalkDir(src, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		if entry.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		if !entry.Type().IsRegular() {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(target, data, 0o644)
	})
	if err != nil {
		t.Fatalf("copy data dir %s: %v", src, err)
	}
}

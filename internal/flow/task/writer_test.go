// writer_test.go reproduces the two C4 judgments as Go assertions: exactly one file
// under internal/flow + scripts/rules assigns the task state rows, and exactly one
// file under internal declares the K4 completion type.
package task

import (
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// repoRoot walks up from the test directory until it finds the module root, so the
// scan sees the same tree the C4 shell judgment sees.
func repoRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatalf("no go.mod above %s", dir)
		}
		dir = parent
	}
}

// filesAssigningPath returns every file under dirs that assigns path, matching the
// C4 shell shape: the path, optional blanks, then '='.
func filesAssigningPath(t *testing.T, dirs []string, path string) []string {
	t.Helper()
	found := map[string]struct{}{}
	for _, dir := range dirs {
		err := filepath.WalkDir(dir, func(name string, entry fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if entry.IsDir() {
				if entry.Name() == ".git" {
					return filepath.SkipDir
				}
				return nil
			}
			raw, readErr := os.ReadFile(name)
			if readErr != nil {
				return readErr
			}
			for _, line := range strings.Split(string(raw), "\n") {
				if assignsPath(line, path) {
					found[name] = struct{}{}
					break
				}
			}
			return nil
		})
		if err != nil {
			t.Fatalf("scan %s: %v", dir, err)
		}
	}
	return sortedKeys(found)
}

// assignsPath reports whether one line assigns the given contract path.
func assignsPath(line, path string) bool {
	rest := line
	for {
		index := strings.Index(rest, path)
		if index < 0 {
			return false
		}
		after := strings.TrimLeft(rest[index+len(path):], " \t")
		if strings.HasPrefix(after, "=") {
			return true
		}
		rest = rest[index+len(path):]
	}
}

// sortedKeys returns the map keys in a stable order.
func sortedKeys(values map[string]struct{}) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

// TestTaskFieldsHaveSingleWriter asserts the C4 single-writer rule: the two task
// state rows are assigned by exactly one file, and that file is this package.
func TestTaskFieldsHaveSingleWriter(t *testing.T) {
	root := repoRoot(t)
	dirs := []string{filepath.Join(root, "internal", "flow"), filepath.Join(root, "scripts", "rules")}
	writers := map[string]struct{}{}
	for _, path := range []string{PATH_TASK_CURRENT, PATH_TASK_PROGRESS} {
		for _, file := range filesAssigningPath(t, dirs, path) {
			writers[file] = struct{}{}
		}
	}
	keys := sortedKeys(writers)
	if len(keys) != 1 {
		t.Fatalf("C4 wants exactly 1 task state writer under internal/flow + scripts/rules, got %d: %v", len(keys), keys)
	}
	packageDir := filepath.Join(root, "internal", "flow", "task") + string(filepath.Separator)
	if !strings.HasPrefix(keys[0], packageDir) {
		t.Fatalf("task state rows are written outside internal/flow/task: %s", keys[0])
	}
}

// TestCompletionTypeDeclaredOnce asserts the C4 K4 rule: the settlement completion
// type is declared by exactly one file under internal.
func TestCompletionTypeDeclaredOnce(t *testing.T) {
	root := repoRoot(t)
	// The two halves are assembled at run time so this test file itself cannot match
	// the declaration pattern it looks for.
	typePrefix := "ty" + "pe "
	completionWord := "Comp" + "letion"
	found := map[string]struct{}{}
	err := filepath.WalkDir(filepath.Join(root, "internal"), func(name string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			if entry.Name() == ".git" {
				return filepath.SkipDir
			}
			return nil
		}
		raw, readErr := os.ReadFile(name)
		if readErr != nil {
			return readErr
		}
		for _, line := range strings.Split(string(raw), "\n") {
			index := strings.Index(line, typePrefix)
			if index < 0 {
				continue
			}
			if strings.Contains(line[index+len(typePrefix):], completionWord) {
				found[name] = struct{}{}
				break
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("scan internal: %v", err)
	}
	keys := sortedKeys(found)
	if len(keys) != 1 {
		t.Fatalf("C4 wants exactly 1 completion type declaration, got %d: %v", len(keys), keys)
	}
}

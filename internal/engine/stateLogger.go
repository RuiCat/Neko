package engine

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// StateLogger appends tick results to daily JSONL files and prunes old ones.
type StateLogger struct {
	mu       sync.Mutex
	dir      string
	keepDays int
}

// NewStateLogger creates a logger rooted at dir (usually <dataDir>/logs).
func NewStateLogger(dir string, keepDays int) *StateLogger {
	if keepDays <= 0 {
		keepDays = 7
	}
	return &StateLogger{dir: dir, keepDays: keepDays}
}

// Log appends one tick result as a JSON line. snapshots (entity id → key
// values) is optional and mirrors Python's per-tick key-state snapshot.
func (l *StateLogger) Log(result TickResult, scope string, snapshots map[string]map[string]any) error {
	if len(result.Transitions) == 0 && len(result.Conflicts) == 0 {
		return nil
	}
	payload := map[string]any{
		"tick_id": result.TickID, "status": result.Status, "scope": scope,
		"transitions": result.Transitions, "conflicts": result.Conflicts,
		"hint": result.ReplyHint,
	}
	if len(snapshots) > 0 {
		payload["snapshots"] = snapshots
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if err := os.MkdirAll(l.dir, 0o755); err != nil {
		return err
	}
	path := filepath.Join(l.dir, "state_"+time.Now().Format("2006-01-02")+".log")
	file, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer file.Close()
	if _, err := file.Write(append(raw, '\n')); err != nil {
		return err
	}
	l.pruneLocked()
	return nil
}

func (l *StateLogger) pruneLocked() {
	entries, err := os.ReadDir(l.dir)
	if err != nil {
		return
	}
	cutoff := time.Now().AddDate(0, 0, -l.keepDays)
	var stale []string
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasPrefix(name, "state_") || !strings.HasSuffix(name, ".log") {
			continue
		}
		datePart := strings.TrimSuffix(strings.TrimPrefix(name, "state_"), ".log")
		date, err := time.Parse("2006-01-02", datePart)
		if err != nil {
			continue
		}
		if date.Before(cutoff) {
			stale = append(stale, filepath.Join(l.dir, name))
		}
	}
	sort.Strings(stale)
	for _, path := range stale {
		_ = os.Remove(path)
	}
}

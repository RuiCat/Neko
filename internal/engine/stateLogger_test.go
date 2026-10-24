package engine

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestStateLoggerWritesDailyJSONL(t *testing.T) {
	dir := t.TempDir()
	logger := NewStateLogger(dir, 7)
	result := TickResult{
		TickID: "t1", Status: "ok",
		Transitions: []Transition{{EntityID: "猫猫", RuleID: "r1"}},
		Conflicts:   []Conflict{},
		ReplyHint:   "done",
	}
	if err := logger.Log(result, "*", map[string]map[string]any{
		"猫猫": {"体力": float64(90), "身体状态": "活跃"},
	}); err != nil {
		t.Fatalf("log: %v", err)
	}
	name := "state_" + time.Now().Format("2006-01-02") + ".log"
	raw, err := os.ReadFile(filepath.Join(dir, name))
	if err != nil {
		t.Fatalf("read log: %v", err)
	}
	if !strings.Contains(string(raw), `"tick_id":"t1"`) || !strings.Contains(string(raw), `"scope":"*"`) {
		t.Fatalf("unexpected log line: %s", raw)
	}
	if !strings.Contains(string(raw), `"snapshots":{"猫猫":{"体力":90`) {
		t.Fatalf("snapshot missing: %s", raw)
	}
}

func TestStateLoggerPrunesOldFiles(t *testing.T) {
	dir := t.TempDir()
	old := filepath.Join(dir, "state_2000-01-01.log")
	if err := os.WriteFile(old, []byte("{}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	logger := NewStateLogger(dir, 1)
	if err := logger.Log(TickResult{
		TickID: "t2", Status: "ok",
		Transitions: []Transition{{EntityID: "猫猫", RuleID: "r1"}},
	}, "*", nil); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(old); !os.IsNotExist(err) {
		t.Fatalf("old log was not pruned: %v", err)
	}
}

func TestStateLoggerSkipsEmptyTick(t *testing.T) {
	dir := t.TempDir()
	logger := NewStateLogger(dir, 7)
	if err := logger.Log(TickResult{TickID: "empty", Status: "ok"}, "*", nil); err != nil {
		t.Fatalf("log: %v", err)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("empty tick wrote %d files", len(entries))
	}
}

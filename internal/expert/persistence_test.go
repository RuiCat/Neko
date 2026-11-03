// persistence_test.go pins the round-trip and the fail-loud contract for a
// corrupt store.
package expert

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestSaveLoadRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "experts.json")
	source := NewDefaultRegistry(8)
	source.Remember("事件记忆", Entry{SnapshotID: "evt_1", Content: "昨晚的约定", Weight: 0.7})

	if err := SaveFile(path, source); err != nil {
		t.Fatal(err)
	}
	target := NewDefaultRegistry(8)
	if err := LoadFile(path, target); err != nil {
		t.Fatal(err)
	}
	expert, ok := target.Get("事件记忆")
	if !ok {
		t.Fatal("缺少事件专家")
	}
	fragments, err := expert.Compensate(context.TODO(), Input{})
	if err != nil || len(fragments) != 1 || fragments[0].SourceSnapshotID != "evt_1" {
		t.Fatalf("恢复失败：%v,%v", fragments, err)
	}
}

func TestLoadMissingFileIsEmpty(t *testing.T) {
	registry := NewDefaultRegistry(8)
	if err := LoadFile(filepath.Join(t.TempDir(), "missing.json"), registry); err != nil {
		t.Fatalf("缺失文件不应报错：%v", err)
	}
}

func TestLoadCorruptFileFails(t *testing.T) {
	path := filepath.Join(t.TempDir(), "experts.json")
	if err := os.WriteFile(path, []byte("{oops"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := LoadFile(path, NewDefaultRegistry(8)); err == nil {
		t.Fatal("坏文件应报错")
	}
}

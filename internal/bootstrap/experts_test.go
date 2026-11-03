// experts_test.go pins the runtime load/save wiring for expert contexts.
package bootstrap

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/RuiCat/Neko/internal/expert"
)

func TestRuntimeLoadsAndPersistsExperts(t *testing.T) {
	dataDir := t.TempDir()
	path := filepath.Join(dataDir, "experts.json")
	seed := expert.NewDefaultRegistry(8)
	seed.Remember("事件记忆", expert.Entry{SnapshotID: "evt_seed", Content: "种子记忆", Weight: 0.8})
	if err := expert.SaveFile(path, seed); err != nil {
		t.Fatal(err)
	}
	runtime, err := New(Options{DataDir: dataDir, ScriptsDir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	loaded, ok := runtime.Experts.Get("事件记忆")
	if !ok {
		t.Fatal("缺少事件专家")
	}
	fragments, err := loaded.Compensate(context.Background(), expert.Input{})
	if err != nil || len(fragments) != 1 || fragments[0].SourceSnapshotID != "evt_seed" {
		t.Fatalf("未加载种子记忆：%v,%v", fragments, err)
	}
	runtime.Experts.Remember("事件记忆", expert.Entry{SnapshotID: "evt_new", Content: "新记忆", Weight: 0.9})
	if err := runtime.Close(); err != nil {
		t.Fatal(err)
	}
	reloaded := expert.NewDefaultRegistry(8)
	if err := expert.LoadFile(path, reloaded); err != nil {
		t.Fatal(err)
	}
	recall, _ := reloaded.Get("事件记忆")
	fragments, _ = recall.Compensate(context.Background(), expert.Input{})
	if len(fragments) != 2 {
		t.Fatalf("Close 未持久化新记忆：%d 条", len(fragments))
	}
}

// setPersistence_test.go pins the v2 round trip and the v1 backup migration.
package expert

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// TestSetSaveLoadRoundTrip keeps every entity pool separate on disk.
func TestSetSaveLoadRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "experts.json")
	source := NewDefaultSet(8)
	source.Remember("猫猫", "事件记忆", Entry{SnapshotID: "evt_cat", SessionID: "s-1", Content: "猫猫的约定", Weight: 0.7})
	source.Remember("梦梦", "事件记忆", Entry{SnapshotID: "evt_meng", Content: "梦梦的约定", Weight: 0.6})
	if err := SaveSetFile(path, source); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	payload := persistedSet{}
	if err := json.Unmarshal(raw, &payload); err != nil {
		t.Fatal(err)
	}
	if payload.Version != PERSIST_VERSION || len(payload.Entities) != 2 {
		t.Fatalf("v2 文件形状错误：%+v", payload)
	}
	target := NewDefaultSet(8)
	if err := LoadSetFile(path, target); err != nil {
		t.Fatal(err)
	}
	for entity, snapshot := range map[string]string{"猫猫": "evt_cat", "梦梦": "evt_meng"} {
		memory, ok := target.RegistryOf(entity).Get("事件记忆")
		if !ok {
			t.Fatalf("%s 缺少事件专家", entity)
		}
		fragments, err := memory.Compensate(context.TODO(), Input{})
		if err != nil || len(fragments) != 1 || fragments[0].SourceSnapshotID != snapshot {
			t.Fatalf("%s 恢复失败：%v,%v", entity, fragments, err)
		}
	}
}

// TestLoadSetFileMigratesV1ToBackup proves the old global pool is preserved
// beside the store and never guessed into an entity.
func TestLoadSetFileMigratesV1ToBackup(t *testing.T) {
	path := filepath.Join(t.TempDir(), "experts.json")
	legacy := persistedRegistry{
		Version: LEGACY_PERSIST_VERSION,
		Experts: map[string][]Entry{"事件记忆": {{SnapshotID: "evt_old", Content: "旧池", Weight: 0.8}}},
	}
	raw, err := json.Marshal(legacy)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, raw, 0o644); err != nil {
		t.Fatal(err)
	}
	set := NewDefaultSet(8)
	if err := LoadSetFile(path, set); err != nil {
		t.Fatal(err)
	}
	if entities := set.Entities(); len(entities) != 0 {
		t.Fatalf("v1 无法归属实体，应按空状态启动：%v", entities)
	}
	backup, err := os.ReadFile(path + ".v1.bak")
	if err != nil {
		t.Fatalf("缺少 v1 备份：%v", err)
	}
	if !bytes.Equal(backup, raw) {
		t.Fatal("v1 备份内容与原文件不一致")
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("原文件应保留给后续 Save 覆盖：%v", err)
	}
}

// TestLoadSetFileRejectsUnknownVersion keeps a future store fail-loud.
func TestLoadSetFileRejectsUnknownVersion(t *testing.T) {
	path := filepath.Join(t.TempDir(), "experts.json")
	if err := os.WriteFile(path, []byte(`{"version":9,"entities":{}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := LoadSetFile(path, NewDefaultSet(8)); err == nil {
		t.Fatal("未知版本应报错")
	}
}

// TestLoadSetFileMissingFileIsEmpty matches the historical first-run behavior.
func TestLoadSetFileMissingFileIsEmpty(t *testing.T) {
	set := NewDefaultSet(8)
	if err := LoadSetFile(filepath.Join(t.TempDir(), "missing.json"), set); err != nil {
		t.Fatalf("缺失文件不应报错：%v", err)
	}
	if entities := set.Entities(); len(entities) != 0 {
		t.Fatalf("缺失文件应为空状态：%v", entities)
	}
}

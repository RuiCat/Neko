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

func TestDropInjectedEchoesRemovesPushAndAssistantOnlyEntries(t *testing.T) {
	registry := NewDefaultRegistry(8)
	remember := func(id, snapshot, content string) {
		registry.Remember(id, Entry{SnapshotID: snapshot, Content: content, Weight: 0.7})
	}
	remember("事件记忆", "evt_1", "用户: 在吗？ | 助手: 在的喵")
	remember("事件记忆", "evt_2", "用户: [astrbox-observation] 猫猫 · 2 处变化 | 助手: 好")
	remember("事件记忆", "evt_3", "用户: [astrbox-compensation] 猫猫 补偿包 | 助手: 唔")
	remember("事件记忆", "evt_4", "助手: （睡沉了，尾巴松开了）")
	remember("表达偏好", "evt_5", "用户: 摸摸头 | 助手: 唔……好")

	if dropped := DropInjectedEchoes(registry); dropped != 3 {
		t.Fatalf("应清掉 3 条回声，得到 %d", dropped)
	}
	event, ok := registry.Get("事件记忆")
	if !ok {
		t.Fatal("缺少事件专家")
	}
	entries := event.(*RecallExpert).Entries()
	if len(entries) != 1 || entries[0].SnapshotID != "evt_1" {
		t.Fatalf("真实对话应保留，回声应清掉：%+v", entries)
	}
	expression, _ := registry.Get("表达偏好")
	if len(expression.(*RecallExpert).Entries()) != 1 {
		t.Fatal("其他专家的真实条目不应被误删")
	}
}

// candidates_test.go pins append/list semantics and the missing-file case.
package candidates

import (
	"path/filepath"
	"testing"
	"time"
)

func TestAppendAndList(t *testing.T) {
	store := NewStore(filepath.Join(t.TempDir(), "candidates.jsonl"))
	if _, err := store.Append(Record{EntityID: "猫猫", Path: "状态.信任值", Proposed: 0.3}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Append(Record{EntityID: "猫猫", Path: "状态.新字段", Status: STATUS_REJECTED}); err != nil {
		t.Fatal(err)
	}
	pending, err := store.List(STATUS_PENDING)
	if err != nil {
		t.Fatal(err)
	}
	if len(pending) != 1 || pending[0].Path != "状态.信任值" || pending[0].ID == "" {
		t.Fatalf("待审候选错误：%+v", pending)
	}
	all, err := store.List("")
	if err != nil || len(all) != 2 {
		t.Fatalf("全部候选 %d,%v", len(all), err)
	}
}

func TestAppendRejectsMissingFields(t *testing.T) {
	store := NewStore(filepath.Join(t.TempDir(), "candidates.jsonl"))
	if _, err := store.Append(Record{Path: "状态.情绪"}); err == nil {
		t.Fatal("缺少 entity_id 应报错")
	}
	if _, err := store.Append(Record{EntityID: "猫猫"}); err == nil {
		t.Fatal("缺少 path 应报错")
	}
}

func TestListMissingFileIsEmpty(t *testing.T) {
	store := NewStore(filepath.Join(t.TempDir(), "missing.jsonl"))
	records, err := store.List("")
	if err != nil || len(records) != 0 {
		t.Fatalf("缺失文件应返回空：%v,%v", records, err)
	}
}

func TestResolveFoldsLatestState(t *testing.T) {
	store := NewStore(filepath.Join(t.TempDir(), "candidates.jsonl"))
	created, err := store.Append(Record{EntityID: "猫猫", Path: "状态.信任值"})
	if err != nil {
		t.Fatal(err)
	}
	resolved, err := store.Resolve(created.ID, STATUS_APPROVED, "人工确认")
	if err != nil {
		t.Fatal(err)
	}
	if resolved.Status != STATUS_APPROVED {
		t.Fatalf("处置状态 %q", resolved.Status)
	}
	pending, _ := store.List(STATUS_PENDING)
	if len(pending) != 0 {
		t.Fatalf("已处置仍出现在 pending：%v", pending)
	}
	approved, _ := store.List(STATUS_APPROVED)
	if len(approved) != 1 || approved[0].ID != created.ID {
		t.Fatalf("approved 列表错误：%v", approved)
	}
	if approved[0].Reason != "人工确认" {
		t.Fatalf("备注未保存：%q", approved[0].Reason)
	}
}

func TestResolveRejectsBadInput(t *testing.T) {
	store := NewStore(filepath.Join(t.TempDir(), "candidates.jsonl"))
	if _, err := store.Resolve("missing", STATUS_APPROVED, ""); err == nil {
		t.Fatal("未知 ID 应报错")
	}
	created, _ := store.Append(Record{EntityID: "猫猫", Path: "状态.情绪"})
	if _, err := store.Resolve(created.ID, "pending", ""); err == nil {
		t.Fatal("非法状态应报错")
	}
}

func TestExpireOldPending(t *testing.T) {
	store := NewStore(filepath.Join(t.TempDir(), "candidates.jsonl"))
	if _, err := store.Append(Record{EntityID: "猫猫", Path: "状态.信任值"}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Append(Record{EntityID: "猫猫", Path: "状态.新字段", Status: STATUS_APPROVED}); err != nil {
		t.Fatal(err)
	}
	expired, err := store.Expire(time.Now().Add(time.Hour))
	if err != nil || expired != 1 {
		t.Fatalf("过期数 %d,%v", expired, err)
	}
	pending, _ := store.List(STATUS_PENDING)
	if len(pending) != 0 {
		t.Fatalf("pending 未清空：%v", pending)
	}
	rejected, _ := store.List(STATUS_REJECTED)
	if len(rejected) != 1 || rejected[0].Reason != "expired" {
		t.Fatalf("过期记录错误：%v", rejected)
	}
}

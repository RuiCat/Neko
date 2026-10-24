package diagnostics

import (
	"fmt"
	"sync"
	"testing"
)

func TestStoreDeduplicates(t *testing.T) {
	store := New(16)
	store.Add(Record{Code: "script.apply.failed", Severity: SEVERITY_ERROR, Script: "a", Message: "boom"})
	store.Add(Record{Code: "script.apply.failed", Severity: SEVERITY_ERROR, Script: "a", Message: "boom"})
	if store.Len() != 1 {
		t.Fatalf("len = %d, want 1", store.Len())
	}
	items := store.Snapshot(Filter{}, 0)
	if len(items) != 1 || items[0].Count != 2 {
		t.Fatalf("items = %+v", items)
	}
}

func TestStoreEvictsOldest(t *testing.T) {
	store := New(2)
	for index := 0; index < 3; index++ {
		store.Add(Record{Code: "tick.rule.error", Severity: SEVERITY_ERROR, Message: fmt.Sprintf("e%d", index)})
	}
	if store.Len() != 2 {
		t.Fatalf("len = %d, want 2", store.Len())
	}
	items := store.Snapshot(Filter{}, 0)
	for _, item := range items {
		if item.Message == "e0" {
			t.Fatal("oldest record was not evicted")
		}
	}
	if summary := store.Summary(Filter{}); summary.Overflow != 1 {
		t.Fatalf("overflow = %d, want 1", summary.Overflow)
	}
}

func TestStoreClearByScript(t *testing.T) {
	store := New(16)
	store.Add(Record{Code: "script.apply.failed", Severity: SEVERITY_ERROR, Script: "a", Message: "boom"})
	store.Add(Record{Code: "script.apply.failed", Severity: SEVERITY_ERROR, Script: "b", Message: "boom"})
	store.ClearByScript("a")
	items := store.Snapshot(Filter{}, 0)
	if len(items) != 1 || items[0].Script != "b" {
		t.Fatalf("items = %+v", items)
	}
}

func TestForEntityScope(t *testing.T) {
	store := New(16)
	store.Add(Record{Code: "tick.conflict.pending", Severity: SEVERITY_WARNING, Entity: "猫猫", Message: "conflict"})
	store.Add(Record{Code: "script.apply.failed", Severity: SEVERITY_ERROR, Script: "a", Message: "global"})
	store.Add(Record{Code: "script.apply.warning", Severity: SEVERITY_INFO, Script: "b", Message: "unrelated"})
	scope := store.ForEntity("猫猫", "NPC:猫猫", 3)
	if scope.Summary.Error != 1 || scope.Summary.Warning != 1 || scope.Summary.Info != 0 {
		t.Fatalf("summary = %+v", scope.Summary)
	}
	if len(scope.Items) != 2 {
		t.Fatalf("items = %+v", scope.Items)
	}
	if scope.Items[0].Severity != SEVERITY_ERROR {
		t.Fatalf("error must sort first: %+v", scope.Items)
	}
}

func TestForEntityTruncates(t *testing.T) {
	store := New(16)
	for index := 0; index < 5; index++ {
		store.Add(Record{Code: "tick.conflict.pending", Severity: SEVERITY_WARNING, Entity: "猫猫", Message: fmt.Sprintf("c%d", index)})
	}
	scope := store.ForEntity("猫猫", "", 2)
	if len(scope.Items) != 2 || !scope.Truncated {
		t.Fatalf("scope = %+v", scope)
	}
}

func TestStoreConcurrentAdd(t *testing.T) {
	store := New(64)
	var group sync.WaitGroup
	for worker := 0; worker < 8; worker++ {
		group.Add(1)
		go func(worker int) {
			defer group.Done()
			for index := 0; index < 20; index++ {
				store.Add(Record{Code: "tick.rule.error", Severity: SEVERITY_ERROR, Message: fmt.Sprintf("w%d-%d", worker, index)})
				_ = store.Snapshot(Filter{}, 5)
			}
		}(worker)
	}
	group.Wait()
	if store.Len() > 64 {
		t.Fatalf("len = %d exceeds capacity", store.Len())
	}
}

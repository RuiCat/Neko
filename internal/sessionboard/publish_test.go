// publish_test.go covers the script publish channel: a system-owned global
// region, one key per entity, unchanged rewrites and the validation limits.
package sessionboard

import "testing"

// TestPublishWritesSystemRegion proves one publish lands in the shared region.
func TestPublishWritesSystemRegion(t *testing.T) {
	store := newTestStore(t)
	result, err := store.Publish(PublishRequest{Key: "猫猫", Value: "猫猫想主人了"})
	if err != nil {
		t.Fatalf("publish: %v", err)
	}
	if !result.Changed || result.Version != 1 {
		t.Fatalf("result = %+v, 期望 changed=true version=1", result)
	}
	read, err := store.Read(ReadQuery{Scope: SCOPE_GLOBAL})
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	region := findRegion(t, read.Board, PUBLISH_REGION)
	if region.Owner.Kind != OWNER_SYSTEM || region.Owner.ID != PUBLISH_REGION {
		t.Fatalf("owner = %+v, 期望 system/%s", region.Owner, PUBLISH_REGION)
	}
	if len(region.Entries) != 1 {
		t.Fatalf("entries = %d, 期望 1", len(region.Entries))
	}
	entry := region.Entries[0]
	if entry.Key != "猫猫" || entry.To != "猫猫想主人了" || entry.By != PUBLISH_BY {
		t.Fatalf("entry = %+v", entry)
	}
}

// TestPublishUnchangedAndValidation proves no-op rewrites and the error matrix.
func TestPublishUnchangedAndValidation(t *testing.T) {
	store := newTestStore(t)
	if _, err := store.Publish(PublishRequest{Key: "猫猫", Value: "第一句"}); err != nil {
		t.Fatalf("publish: %v", err)
	}
	same, err := store.Publish(PublishRequest{Key: "猫猫", Value: "第一句"})
	if err != nil {
		t.Fatalf("republish: %v", err)
	}
	if same.Changed {
		t.Fatalf("相同消息不应产生变化: %+v", same)
	}
	if _, err := store.Publish(PublishRequest{Key: " ", Value: "x"}); err == nil {
		t.Fatal("空 key 应被拒绝")
	}
	if _, err := store.Publish(PublishRequest{Key: "猫猫", Value: string(make([]byte, MAX_VALUE_BYTES+1))}); err == nil {
		t.Fatal("超长 value 应被拒绝")
	}
}

// findRegion returns the named region or fails the test.
func findRegion(t *testing.T, board Board, id string) Region {
	t.Helper()
	for _, region := range board.Regions {
		if region.ID == id {
			return region
		}
	}
	t.Fatalf("找不到区域 %q", id)
	return Region{}
}

// snapshot_test.go pins the canonical digest and the flattened state shape.
package hardstate

import (
	"strings"
	"testing"
	"time"
)

func sampleData() map[string]map[string]any {
	return map[string]map[string]any{
		"猫猫": {
			"状态": map[string]any{
				"情绪": "平静",
				"身体": map[string]any{"体力": 90},
			},
			"标签": []any{"成年"},
		},
		"场景": {"状态": map[string]any{"地点": "客厅"}},
	}
}

func testIDs() IDs {
	return IDs{SnapshotID: "evt_20261101_0001", TurnID: "evt_20261101_0001", TickID: "t1"}
}

func TestCaptureFlattensOnlyTheStateGroup(t *testing.T) {
	snapshot, err := Capture(sampleData(), DefaultSchema(), testIDs(), time.Date(2026, 11, 1, 12, 0, 0, 0, time.Local))
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"猫猫.状态.情绪", "猫猫.状态.身体.体力", "场景.状态.地点"}
	if len(snapshot.State) != len(want) {
		t.Fatalf("状态字段数 %d，期望 %d：%v", len(snapshot.State), len(want), snapshot.State)
	}
	for _, key := range want {
		if _, ok := snapshot.State[key]; !ok {
			t.Fatalf("缺少扁平路径 %q：%v", key, snapshot.State)
		}
	}
	if _, leaked := snapshot.State["猫猫.标签"]; leaked {
		t.Fatal("非状态组字段不应进入快照")
	}
	if !strings.HasPrefix(snapshot.Digest, "sha256:") {
		t.Fatalf("digest 格式错误: %q", snapshot.Digest)
	}
}

func TestDigestIsStableAndSensitive(t *testing.T) {
	schema := DefaultSchema()
	first, err := Capture(sampleData(), schema, testIDs(), time.Unix(0, 0))
	if err != nil {
		t.Fatal(err)
	}
	second, err := Capture(sampleData(), schema, testIDs(), time.Unix(999, 0))
	if err != nil {
		t.Fatal(err)
	}
	if first.Digest != second.Digest {
		t.Fatalf("同状态不同时间 digest 应相等: %s vs %s", first.Digest, second.Digest)
	}
	changed := sampleData()
	changed["猫猫"]["状态"].(map[string]any)["情绪"] = "烦躁"
	third, err := Capture(changed, schema, testIDs(), time.Unix(0, 0))
	if err != nil {
		t.Fatal(err)
	}
	if third.Digest == first.Digest {
		t.Fatal("状态变化后 digest 必须变化")
	}
}

func TestSchemaEntitiesFilterAndValidation(t *testing.T) {
	snapshot, err := Capture(sampleData(), DefaultSchema().WithEntities("猫猫"), testIDs(), time.Unix(0, 0))
	if err != nil {
		t.Fatal(err)
	}
	for key := range snapshot.State {
		if !strings.HasPrefix(key, "猫猫.") {
			t.Fatalf("实体过滤失效: %q", key)
		}
	}
	if err := (Schema{Version: 0, Group: "状态"}).Validate(); err == nil {
		t.Fatal("版本 0 应被拒绝")
	}
	if err := (Schema{Version: 1, Group: "状态.身体"}).Validate(); err == nil {
		t.Fatal("含点的状态组应被拒绝")
	}
}

func TestCaptureRejectsMissingSnapshotID(t *testing.T) {
	if _, err := Capture(sampleData(), DefaultSchema(), IDs{}, time.Unix(0, 0)); err == nil {
		t.Fatal("缺少快照 ID 应被拒绝")
	}
}

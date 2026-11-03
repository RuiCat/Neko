// annotations_test.go pins the annotation loader and its validation.
package annotations

import (
	"os"
	"path/filepath"
	"testing"
)

func writeFile(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "labels.jsonl")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestLoadAndValidate(t *testing.T) {
	path := writeFile(t, `{"turn_id":"evt_1","used_fragments":[0],"cites":[{"event_id":"evt_0","kind":"event"}],"annotator":"lead","created_at":"2026-11-03"}`)
	index, err := LoadDir(filepath.Dir(path))
	if err != nil {
		t.Fatal(err)
	}
	if len(index) != 1 || len(index["evt_1"]) != 1 || index["evt_1"][0].UsedFragments[0] != 0 {
		t.Fatalf("加载错误：%v", index)
	}
}

func TestMultipleAnnotatorsAllowed(t *testing.T) {
	first := `{"turn_id":"evt_1","used_fragments":[0],"annotator":"a","created_at":"2026-11-03"}`
	second := `{"turn_id":"evt_1","used_fragments":[0],"annotator":"b","created_at":"2026-11-03"}`
	path := writeFile(t, first+"\n"+second+"\n")
	index, err := LoadDir(filepath.Dir(path))
	if err != nil {
		t.Fatalf("不同标注者应允许：%v", err)
	}
	if len(index["evt_1"]) != 2 {
		t.Fatalf("标注者数 %d", len(index["evt_1"]))
	}
}

func TestDuplicateTurnRejected(t *testing.T) {
	line := `{"turn_id":"evt_1","used_fragments":[],"annotator":"lead","created_at":"2026-11-03"}`
	path := writeFile(t, line+"\n"+line+"\n")
	if _, err := LoadDir(filepath.Dir(path)); err == nil {
		t.Fatal("同一标注者重复标注应报错")
	}
}

func TestValidateRejectsBadFields(t *testing.T) {
	cases := []Record{
		{TurnID: "", Annotator: "a", CreatedAt: "2026-11-03"},
		{TurnID: "evt_1", Annotator: "", CreatedAt: "2026-11-03"},
		{TurnID: "evt_1", Annotator: "a", CreatedAt: ""},
		{TurnID: "evt_1", Annotator: "a", CreatedAt: "2026-11-03", UsedFragments: []int{-1}},
		{TurnID: "evt_1", Annotator: "a", CreatedAt: "2026-11-03", Cites: []Citation{{EventID: ""}}},
	}
	for index, record := range cases {
		if err := Validate(record); err == nil {
			t.Fatalf("用例 %d 应被拒绝：%+v", index, record)
		}
	}
}

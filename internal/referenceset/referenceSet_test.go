// referenceSet_test.go pins the validator and verifies the shipped v0 anchor.
package referenceset

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

const validEntry = `{"id":"rs-001","scenario":"试探","context":"室内·夜间·两人","expected_reactions":["说不清楚","提出一起查证"],"forbidden_reactions":["编造答案"],"author":"lead","created_at":"2026-11-01","locked":true}`

func writeTempSet(t *testing.T, lines ...string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "set.jsonl")
	content := ""
	for _, line := range lines {
		content += line + "\n"
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestLoadAndValidate(t *testing.T) {
	path := writeTempSet(t, validEntry)
	entries, err := LoadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := Validate(entries); err != nil {
		t.Fatal(err)
	}
}

func TestValidateRejectsUnlockedAndDuplicate(t *testing.T) {
	unlocked := `{"id":"rs-001","scenario":"x","context":"y","expected_reactions":["a","b"],"forbidden_reactions":["c"],"author":"lead","created_at":"2026-11-01","locked":false}`
	if err := Validate(mustLoad(t, unlocked)); err == nil {
		t.Fatal("未锁定条目应被拒绝")
	}
	duplicated := validEntry + "\n" + validEntry
	if err := Validate(mustLoad(t, duplicated)); err == nil {
		t.Fatal("重复 id 应被拒绝")
	}
}

func TestValidateRejectsTooFewReactions(t *testing.T) {
	short := `{"id":"rs-002","scenario":"x","context":"y","expected_reactions":["a"],"forbidden_reactions":["c"],"author":"lead","created_at":"2026-11-01","locked":true}`
	if err := Validate(mustLoad(t, short)); err == nil {
		t.Fatal("期望反应少于下限应被拒绝")
	}
}

func TestShippedReferenceSetMatchesHash(t *testing.T) {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("无法定位测试文件")
	}
	root := filepath.Join(filepath.Dir(file), "..", "..")
	setPath := filepath.Join(root, "fixtures", "reference-set", "v0.jsonl")
	hashPath := filepath.Join(root, "fixtures", "reference-set", "v0.sha256")
	entries, err := LoadFile(setPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := Validate(entries); err != nil {
		t.Fatal(err)
	}
	if len(entries) != 30 {
		t.Fatalf("v0 场景数 %d，期望 30", len(entries))
	}
	if err := Verify(setPath, hashPath); err != nil {
		t.Fatal(err)
	}
}

func mustLoad(t *testing.T, line string) []Entry {
	t.Helper()
	path := writeTempSet(t, line)
	entries, err := LoadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return entries
}

package compiler

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"testing"

	"github.com/RuiCat/Neko/internal/neko"
)

// TestRuleIDsMatchPython compares the rule ids produced by the Go compiler with
// the ids recorded by the Python implementation in scripts.json.
func TestRuleIDsMatchPython(t *testing.T) {
	scriptsBase := filepath.Join("..", "..", "scripts")
	goldenPath := filepath.Join("testdata", "python_rule_ids.json")
	// 这是从 Python 实现导出的对照数据，未随仓库分发（testdata/ 不存在）。
	// 干净 clone 里跳过；一旦有人补上该文件，下面的逐条比对照常严格执行。
	if _, statErr := os.Stat(goldenPath); errors.Is(statErr, fs.ErrNotExist) {
		t.Skipf("跳过：%s 不存在（Python 对照数据不随仓库分发）", goldenPath)
	}
	raw, err := os.ReadFile(goldenPath)
	if err != nil {
		t.Fatalf("read golden ids: %v", err)
	}
	var golden map[string][]string
	if err := json.Unmarshal(raw, &golden); err != nil {
		t.Fatalf("decode golden ids: %v", err)
	}
	names := make([]string, 0, len(golden))
	for name := range golden {
		names = append(names, name)
	}
	sort.Strings(names)

	mismatches := 0
	for _, name := range names {
		source, err := os.ReadFile(filepath.Join(scriptsBase, name+".neko"))
		if err != nil {
			continue
		}
		result := CompileScript(neko.Parse(string(source)))
		got := make([]string, 0, len(result.Rules))
		for _, rule := range result.Rules {
			got = append(got, rule.ID)
		}
		want := append([]string{}, golden[name]...)
		sort.Strings(got)
		sort.Strings(want)
		if len(got) != len(want) {
			mismatches++
			t.Logf("%s: count got=%d want=%d", name, len(got), len(want))
			continue
		}
		for index := range got {
			if got[index] != want[index] {
				mismatches++
				if mismatches <= 8 {
					t.Logf("%s: got %s want %s", name, got[index], want[index])
				}
				break
			}
		}
	}
	if mismatches > 0 {
		t.Fatalf("%d/%d scripts have mismatched rule ids", mismatches, len(names))
	}
}

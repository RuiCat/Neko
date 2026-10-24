package compiler

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"testing"

	"github.com/ruicat/astrbox/internal/neko"
)

// TestRuleIDsMatchPython compares the rule ids produced by the Go compiler with
// the ids recorded by the Python implementation in scripts.json.
func TestRuleIDsMatchPython(t *testing.T) {
	scriptsBase := filepath.Join("..", "..", "scripts")
	raw, err := os.ReadFile(filepath.Join("testdata", "python_rule_ids.json"))
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

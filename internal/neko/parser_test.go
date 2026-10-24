// parser_test.go tests the .neko lexer and parser against hand-written cases and
// the full scripts corpus.
package neko

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// corpusDir is the .neko script collection the corpus test walks.
const corpusDir = "../../scripts"

// astSummary is the JSON shape used to compare Go output with the Python parser.
type astSummary struct {
	DataDefs  int      `json:"data_defs"`
	RuleDefs  int      `json:"rule_defs"`
	EventDefs int      `json:"event_defs"`
	FuncDefs  int      `json:"func_defs"`
	Metadata  int      `json:"metadata"`
	RuleIDs   []string `json:"rule_ids"`
	Errors    int      `json:"errors"`
}

// summarize builds the comparable summary of one parse result.
func summarize(ast *ScriptAST) astSummary {
	ids := []string{}
	seen := map[string]bool{}
	for _, rule := range ast.RuleDefs {
		if rule.RuleID != "" && !seen[rule.RuleID] {
			seen[rule.RuleID] = true
			ids = append(ids, rule.RuleID)
		}
	}
	return astSummary{
		DataDefs:  len(ast.DataDefs),
		RuleDefs:  len(ast.RuleDefs),
		EventDefs: len(ast.EventDefs),
		FuncDefs:  len(ast.FuncDefs),
		Metadata:  len(ast.Metadata),
		RuleIDs:   ids,
		Errors:    len(ast.Errors),
	}
}

// TestScanBlocksKinds checks every block kind, its name and its start line.
func TestScanBlocksKinds(t *testing.T) {
	text := strings.Join([]string{
		`@Data NPC:猫猫 {`,
		`  "名字": "猫",`,
		`  "链接": "http://example.com//x"`,
		`}`,
		`#规则: $[NPC].a < 1 → $.b = 2`,
		`@事件:变化 { 条件: 变化量 > 0 }`,
		`@func 加(a: int) -> int { 返回值 }`,
		`#世界观: 猫猫世界`,
		`@规则[$[NPC]] { 条件: true`,
		`动作: $.a = 1`,
		`}`,
	}, "\n")
	blocks := ScanBlocks(text)
	want := []struct {
		kind string
		name string
		line int
	}{
		{"at_data", "NPC:猫猫", 1},
		{"hash_rule", "#规则: $[NPC].a < 1 → $.b = 2", 5},
		{"at_event", "变化", 6},
		{"at_func", "加(a: int) -> int", 7},
		{"meta", "#世界观: 猫猫世界", 8},
		{"at_rule", "[$[NPC]]", 9},
	}
	if len(blocks) != len(want) {
		t.Fatalf("got %d blocks, want %d: %+v", len(blocks), len(want), blocks)
	}
	for i, w := range want {
		if blocks[i].Kind != w.kind || blocks[i].Name != w.name || blocks[i].Line != w.line {
			t.Errorf("block %d = {%q %q %d}, want {%q %q %d}",
				i, blocks[i].Kind, blocks[i].Name, blocks[i].Line, w.kind, w.name, w.line)
		}
	}
	if !strings.Contains(blocks[0].Body, "http://example.com//x") {
		t.Errorf("string-embedded // was stripped from data body: %q", blocks[0].Body)
	}
}

// TestParseDataDef checks JSON data blocks, number typing and comment stripping.
func TestParseDataDef(t *testing.T) {
	ast := Parse("@Data 猫猫 {\n  \"名字\": \"猫\", // 名字\n  \"数\": 3\n}")
	if len(ast.Errors) != 0 {
		t.Fatalf("unexpected errors: %v", ast.Errors)
	}
	if len(ast.DataDefs) != 1 {
		t.Fatalf("got %d data defs, want 1", len(ast.DataDefs))
	}
	def := ast.DataDefs[0]
	if def.TypeName != "猫猫" || !def.IsEntity {
		t.Errorf("def = {%q entity=%v}", def.TypeName, def.IsEntity)
	}
	if def.Data["名字"] != "猫" {
		t.Errorf("名字 = %#v", def.Data["名字"])
	}
	if def.Data["数"] != int64(3) {
		t.Errorf("数 = %#v, want int64(3)", def.Data["数"])
	}
}

// TestParseHashRule checks the one-line rule form.
func TestParseHashRule(t *testing.T) {
	ast := Parse("#规则: $[NPC].身体.体力 < 20 → $.状态.疲劳 = true")
	if len(ast.Errors) != 0 {
		t.Fatalf("unexpected errors: %v", ast.Errors)
	}
	if len(ast.RuleDefs) != 1 {
		t.Fatalf("got %d rules, want 1", len(ast.RuleDefs))
	}
	rule := ast.RuleDefs[0]
	if len(rule.Conditions) != 1 || rule.Conditions[0] != "$[NPC].身体.体力 < 20" {
		t.Errorf("conditions = %#v", rule.Conditions)
	}
	if len(rule.Actions) != 1 {
		t.Fatalf("actions = %#v", rule.Actions)
	}
	action := rule.Actions[0]
	if action.Op != "set" || action.Target != "状态.疲劳" || action.Value != true {
		t.Errorf("action = %+v", action)
	}
	if rule.LineForm == nil || *rule.LineForm != "#规则: $[NPC].身体.体力 < 20 → $.状态.疲劳 = true" {
		t.Errorf("line form = %v", rule.LineForm)
	}
}

// TestParseRuleLegacyBody checks the key:value rule body form, including branches.
func TestParseRuleLegacyBody(t *testing.T) {
	text := strings.Join([]string{
		`@规则[$[NPC]] {`,
		`  优先级: 100`,
		`  状态: 身体.体力 < 10`,
		`  条件: $[NPC].身体.体力 < 20`,
		`  动作: $.状态.疲劳 = true`,
		`  分支: {`,
		`    $[NPC].身体.体力 < 5 → 状态 = 昏厥`,
		`    默认 → 状态 = 清醒`,
		`  }`,
		`}`,
	}, "\n")
	ast := Parse(text)
	if len(ast.Errors) != 0 {
		t.Fatalf("unexpected errors: %v", ast.Errors)
	}
	if len(ast.RuleDefs) != 1 {
		t.Fatalf("got %d rules, want 1", len(ast.RuleDefs))
	}
	rule := ast.RuleDefs[0]
	if rule.Priority == nil || *rule.Priority != 100 {
		t.Errorf("priority = %v", rule.Priority)
	}
	if rule.StateWhen == nil || *rule.StateWhen != "身体.体力 < 10" {
		t.Errorf("state_when = %v", rule.StateWhen)
	}
	if len(rule.Conditions) != 1 || rule.Conditions[0] != "$[NPC].身体.体力 < 20" {
		t.Errorf("conditions = %#v", rule.Conditions)
	}
	if len(rule.Actions) != 1 || rule.Actions[0].Target != "状态.疲劳" {
		t.Errorf("actions = %+v", rule.Actions)
	}
	if len(rule.Branches) != 2 {
		t.Fatalf("branches = %+v", rule.Branches)
	}
	if rule.Branches[0].Condition == nil || *rule.Branches[0].Condition != "$[NPC].身体.体力 < 5" {
		t.Errorf("branch 0 condition = %v", rule.Branches[0].Condition)
	}
	if got := rule.Branches[0].State["身体"]; got != "昏厥" {
		t.Errorf("branch 0 state = %#v", rule.Branches[0].State)
	}
	if rule.Branches[1].Condition != nil {
		t.Errorf("else branch condition = %v", rule.Branches[1].Condition)
	}
}

// TestParseRuleConditionForm checks `条件 { 动作 }` including in-sets and {e}.
func TestParseRuleConditionForm(t *testing.T) {
	text := strings.Join([]string{
		`@规则[$[NPC]] {`,
		`  $[NPC].行为.主人行为 in {无} ∨ $[NPC].标记.停止 == "停止触摸" {`,
		`    动作: $.状态.触发 = true`,
		`  }`,
		`}`,
	}, "\n")
	ast := Parse(text)
	if len(ast.Errors) != 0 {
		t.Fatalf("unexpected errors: %v", ast.Errors)
	}
	if len(ast.RuleDefs) != 1 {
		t.Fatalf("got %d rules, want 1", len(ast.RuleDefs))
	}
	rule := ast.RuleDefs[0]
	wantCond := `$[NPC].行为.主人行为 in {无} ∨ $[NPC].标记.停止 == "停止触摸"`
	if len(rule.Conditions) != 1 || rule.Conditions[0] != wantCond {
		t.Errorf("conditions = %#v, want %q", rule.Conditions, wantCond)
	}
	if len(rule.Actions) != 1 || rule.Actions[0].Target != "状态.触发" {
		t.Errorf("actions = %+v", rule.Actions)
	}
}

// TestParseMultiIf checks a block holding several comma-separated if branches.
func TestParseMultiIf(t *testing.T) {
	text := strings.Join([]string{
		`@规则[$[NPC]] {`,
		`  $[NPC].身体.体力 < 20 { $.状态.触发 = "低" },`,
		`  $[NPC].身体.体力 >= 80 { $.状态.触发 = "高" }`,
		`}`,
	}, "\n")
	ast := Parse(text)
	if len(ast.Errors) != 0 {
		t.Fatalf("unexpected errors: %v", ast.Errors)
	}
	if len(ast.RuleDefs) != 2 {
		t.Fatalf("got %d rules, want 2", len(ast.RuleDefs))
	}
	if got := ast.RuleDefs[0].Conditions[0]; got != "$[NPC].身体.体力 < 20" {
		t.Errorf("rule 0 condition = %q", got)
	}
	if got := ast.RuleDefs[1].Conditions[0]; got != "$[NPC].身体.体力 >= 80" {
		t.Errorf("rule 1 condition = %q", got)
	}
	if got := ast.RuleDefs[0].Actions[0].Value; got != "低" {
		t.Errorf("rule 0 action value = %#v", got)
	}
}

// TestParseMulticond checks @多规则 child-rule blocks.
func TestParseMulticond(t *testing.T) {
	text := strings.Join([]string{
		`@多规则[$[NPC]] {`,
		`  规则1: {`,
		`    条件: $[NPC].a > 1`,
		`    动作: $.b = 1`,
		`  },`,
		`  规则2: { 动作: $.c = 2 }`,
		`}`,
	}, "\n")
	ast := Parse(text)
	if len(ast.Errors) != 0 {
		t.Fatalf("unexpected errors: %v", ast.Errors)
	}
	if len(ast.RuleDefs) != 2 {
		t.Fatalf("got %d rules, want 2: %+v", len(ast.RuleDefs), ast.RuleDefs)
	}
	if len(ast.RuleDefs[0].Subjects) != 1 || ast.RuleDefs[0].Subjects[0].TypeName != "NPC" {
		t.Errorf("subjects = %+v", ast.RuleDefs[0].Subjects)
	}
	if len(ast.RuleDefs[0].Conditions) != 1 || ast.RuleDefs[0].Conditions[0] != "$[NPC].a > 1" {
		t.Errorf("rule 0 conditions = %#v", ast.RuleDefs[0].Conditions)
	}
	if len(ast.RuleDefs[1].Actions) != 1 || ast.RuleDefs[1].Actions[0].Target != "c" {
		t.Errorf("rule 1 actions = %+v", ast.RuleDefs[1].Actions)
	}
}

// TestParseRuleRange checks threshold-bucket @规则 blocks.
func TestParseRuleRange(t *testing.T) {
	text := strings.Join([]string{
		`@规则[$[NPC]]:$.身体.体温 {`,
		`  36: { 动作: $.状态.正常 = true },`,
		`  38.5: { 动作: $.状态.发烧 = true },`,
		`  =40: { 动作: $.状态.高烧 = true }`,
		`}`,
	}, "\n")
	blocks := ScanBlocks(text)
	if len(blocks) != 1 || blocks[0].Kind != "at_rule_range" {
		t.Fatalf("blocks = %+v", blocks)
	}
	ast := Parse(text)
	if len(ast.Errors) != 0 {
		t.Fatalf("unexpected errors: %v", ast.Errors)
	}
	if len(ast.RuleDefs) != 3 {
		t.Fatalf("got %d rules, want 3: %+v", len(ast.RuleDefs), ast.RuleDefs)
	}
	want := []string{
		"$[NPC].身体.体温 <= 36",
		"$[NPC].身体.体温 > 36 ∧ $[NPC].身体.体温 <= 38.5",
		"$[NPC].身体.体温 == 40",
	}
	for i, cond := range want {
		if got := ast.RuleDefs[i].Conditions[0]; got != cond {
			t.Errorf("rule %d condition = %q, want %q", i, got, cond)
		}
	}
}

// TestParseEventFuncMeta checks @事件, @func and metadata parsing.
func TestParseEventFuncMeta(t *testing.T) {
	text := strings.Join([]string{
		`@事件:状态变化 {`,
		`  条件: 变化量 > 0`,
		`  处理器: onStateChange`,
		`  标记 = 1`,
		`}`,
		`@func 加(a: int, b:int) -> int {`,
		`  返回值`,
		`}`,
		`#世界观: 猫猫世界`,
	}, "\n")
	ast := Parse(text)
	if len(ast.Errors) != 0 {
		t.Fatalf("unexpected errors: %v", ast.Errors)
	}
	if len(ast.EventDefs) != 1 {
		t.Fatalf("event defs = %+v", ast.EventDefs)
	}
	event := ast.EventDefs[0]
	if event.Name != "状态变化" || event.Condition != "变化量 > 0" || event.Handler != "onStateChange" {
		t.Errorf("event = %+v", event)
	}
	if len(event.Actions) != 1 || event.Actions[0] != "标记 = 1" {
		t.Errorf("event actions = %#v", event.Actions)
	}
	if len(ast.FuncDefs) != 1 {
		t.Fatalf("func defs = %+v", ast.FuncDefs)
	}
	fn := ast.FuncDefs[0]
	if fn.Name != "加" || fn.ReturnType != "int" || fn.Body != "\n  返回值\n" {
		t.Errorf("func = %+v", fn)
	}
	if len(fn.Params) != 2 || fn.Params[0] != (Param{Name: "a", Type: "int"}) || fn.Params[1] != (Param{Name: "b", Type: "int"}) {
		t.Errorf("params = %+v", fn.Params)
	}
	if len(ast.Metadata) != 1 || ast.Metadata[0].Key != "世界观" || ast.Metadata[0].Value != "猫猫世界" {
		t.Errorf("metadata = %+v", ast.Metadata)
	}
}

// TestFailSoft checks that one broken block leaves the following blocks parsed.
func TestFailSoft(t *testing.T) {
	text := strings.Join([]string{
		`@Data 坏 { 不是 JSON }`,
		`#规则: a → $.b = 1`,
		`@func 坏头 { x }`,
		`#规则: c → $.d = 2`,
	}, "\n")
	ast := Parse(text)
	if len(ast.Errors) != 2 {
		t.Fatalf("got %d errors, want 2: %v", len(ast.Errors), ast.Errors)
	}
	if len(ast.RuleDefs) != 2 {
		t.Errorf("got %d rules, want 2", len(ast.RuleDefs))
	}
	if len(ast.DataDefs) != 0 {
		t.Errorf("got %d data defs, want 0", len(ast.DataDefs))
	}
}

// TestParseScalar checks the scalar literal conversions.
func TestParseScalar(t *testing.T) {
	cases := []struct {
		raw  string
		want any
	}{
		{`"abc"`, "abc"},
		{`'x'`, "x"},
		{"true", true},
		{"false", false},
		{"12", int64(12)},
		{"-3.5", -3.5},
		{"1_000", int64(1000)},
		{"abc", "abc"},
		{`"`, ""},
	}
	for _, c := range cases {
		if got := parseScalar(c.raw); got != c.want {
			t.Errorf("parseScalar(%q) = %#v, want %#v", c.raw, got, c.want)
		}
	}
	obj := parseScalar(`{"a": 1}`).(map[string]any)
	if obj["a"] != int64(1) {
		t.Errorf("object value = %#v", obj["a"])
	}
	arr := parseScalar(`[1, 2]`).([]any)
	if len(arr) != 2 || arr[0] != int64(1) {
		t.Errorf("array = %#v", arr)
	}
}

// expectedVectors lists per-file definition counts produced by the Python reference.
var expectedVectors = map[string][5]int{
	"core/cat_core.neko":                    {2, 4, 0, 0, 0},
	"behavior/cat_behavior.neko":            {1, 0, 0, 0, 0},
	"r18/cat_r18_flows.neko":                {1, 0, 0, 0, 0},
	"core/cat_body_state.neko":              {1, 7, 3, 3, 0},
	"core/cat_core_v2.neko":                 {2, 3, 2, 3, 0},
	"behavior/cat_season.neko":              {1, 10, 0, 1, 0},
	"behavior/cat_daily.neko":               {1, 9, 0, 1, 0},
	"behavior/cat-repro-cycle-advance.neko": {0, 1, 0, 0, 0},
	"core/cat_anatomy_eversion.neko":        {0, 7, 0, 0, 0},
}

// knownErrorFiles are corpus scripts the Python parser also rejects (invalid
// JSON bodies); every other script must parse with an empty AST.Errors.
var knownErrorFiles = map[string]string{
	"behavior/cat-repro-cycle-advance.neko": "unquoted keys in the @Data JSON object",
	"core/cat_anatomy_eversion.neko":        "YAML-like @Data body instead of JSON",
}

// collectCorpus returns every .neko path under corpusDir, sorted.
func collectCorpus(t *testing.T) []string {
	t.Helper()
	if _, err := os.Stat(corpusDir); err != nil {
		t.Skipf("corpus unavailable: %v", err)
	}
	files := []string{}
	err := filepath.WalkDir(corpusDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() && strings.HasSuffix(path, ".neko") {
			files = append(files, path)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk corpus: %v", err)
	}
	return files
}

// TestCorpus parses every corpus script and checks counts against the Python run.
func TestCorpus(t *testing.T) {
	files := collectCorpus(t)
	if len(files) != 262 {
		t.Fatalf("corpus has %d files, want 262", len(files))
	}
	var total [5]int
	clean := 0
	for _, path := range files {
		rel, err := filepath.Rel(corpusDir, path)
		if err != nil {
			t.Fatalf("rel %s: %v", path, err)
		}
		rel = filepath.ToSlash(rel)
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		ast := Parse(string(raw))
		reason, known := knownErrorFiles[rel]
		if known {
			if len(ast.Errors) == 0 {
				t.Errorf("%s: expected a parse error (%s), got none", rel, reason)
			}
		} else {
			if len(ast.Errors) > 0 {
				t.Errorf("%s: unexpected parse error: %v", rel, ast.Errors)
			}
			clean++
			if len(ast.DataDefs) == 0 && len(ast.RuleDefs) == 0 {
				t.Errorf("%s: parsed but has no data_defs and no rule_defs", rel)
			}
		}
		got := [5]int{len(ast.DataDefs), len(ast.RuleDefs), len(ast.EventDefs), len(ast.FuncDefs), len(ast.Metadata)}
		if want, ok := expectedVectors[rel]; ok && got != want {
			t.Errorf("%s: counts %v, want %v", rel, got, want)
		}
		for i := range total {
			total[i] += got[i]
		}
	}
	if clean != 260 {
		t.Errorf("clean scripts = %d, want 260", clean)
	}
	wantTotal := [5]int{54, 1901, 5, 15, 0}
	if total != wantTotal {
		t.Errorf("corpus totals (data,rule,event,func,meta) = %v, want %v", total, wantTotal)
	}
}

// TestSummaryDump prints AST summaries when NEKO_SUMMARY_FILES is set, so the
// Go result can be compared field by field with the Python reference.
func TestSummaryDump(t *testing.T) {
	raw := os.Getenv("NEKO_SUMMARY_FILES")
	if raw == "" {
		t.Skip("set NEKO_SUMMARY_FILES to print AST summaries")
	}
	for _, rel := range strings.Split(raw, ",") {
		rel = strings.TrimSpace(rel)
		if rel == "" {
			continue
		}
		rawText, err := os.ReadFile(filepath.Join(corpusDir, rel))
		if err != nil {
			t.Fatalf("read %s: %v", rel, err)
		}
		payload, err := json.Marshal(summarize(Parse(string(rawText))))
		if err != nil {
			t.Fatalf("marshal %s: %v", rel, err)
		}
		fmt.Printf("SUMMARY\t%s\t%s\n", rel, payload)
	}
}

// parserDetail_test.go dumps a normalized value-level AST summary so the Go
// parser can be compared field by field with the Python reference parser.
package neko

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// detailSubject is one rule subject in the comparable dump.
type detailSubject struct {
	Var       any    `json:"var"`
	Kind      string `json:"kind"`
	TypeName  any    `json:"type_name"`
	Condition any    `json:"condition"`
	EntityID  any    `json:"entity_id"`
}

// detailAction is one action with its normalized value payload.
type detailAction struct {
	Op      string `json:"op"`
	Target  string `json:"target"`
	Subject any    `json:"subject"`
	Entity  any    `json:"entity"`
	Value   any    `json:"value"`
}

// detailBranch is one state-machine branch in the comparable dump.
type detailBranch struct {
	Condition any `json:"condition"`
	State     any `json:"state"`
}

// detailRule is one parsed rule in the comparable dump.
type detailRule struct {
	Subjects   []detailSubject `json:"subjects"`
	HeaderRaw  string          `json:"header_raw"`
	RuleID     any             `json:"rule_id"`
	StateWhen  any             `json:"state_when"`
	Priority   any             `json:"priority"`
	Conditions []string        `json:"conditions"`
	Actions    []detailAction  `json:"actions"`
	Branches   []detailBranch  `json:"branches"`
	LineForm   any             `json:"line_form"`
	Line       int             `json:"line"`
}

// detailData is one data definition in the comparable dump.
type detailData struct {
	TypeName string   `json:"type_name"`
	IsEntity bool     `json:"is_entity"`
	Keys     []string `json:"keys"`
	Data     any      `json:"data"`
	Line     int      `json:"line"`
}

// detailEvent is one event definition in the comparable dump.
type detailEvent struct {
	Name      string `json:"name"`
	Condition any    `json:"condition"`
	Handler   any    `json:"handler"`
	Actions   []any  `json:"actions"`
	Line      int    `json:"line"`
}

// detailFunc is one function definition in the comparable dump.
type detailFunc struct {
	Name       string   `json:"name"`
	Params     [][2]any `json:"params"`
	ReturnType any      `json:"return_type"`
	Body       string   `json:"body"`
	Line       int      `json:"line"`
}

// detailMeta is one metadata entry in the comparable dump.
type detailMeta struct {
	Key   string `json:"key"`
	Value string `json:"value"`
	Line  int    `json:"line"`
}

// detailAST is the whole comparable dump for one script.
type detailAST struct {
	DataDefs  []detailData  `json:"data_defs"`
	RuleDefs  []detailRule  `json:"rule_defs"`
	EventDefs []detailEvent `json:"event_defs"`
	FuncDefs  []detailFunc  `json:"func_defs"`
	Metadata  []detailMeta  `json:"metadata"`
}

// optionalString maps the empty string to nil so it matches Python's None.
func optionalString(s string) any {
	if s == "" {
		return nil
	}
	return s
}

// optionalInt maps zero to nil so it matches Python's None.
func optionalInt(v int) any {
	if v == 0 {
		return nil
	}
	return v
}

// detailValue normalizes an arbitrary scalar value for cross-language comparison.
func detailValue(v any) any {
	switch val := v.(type) {
	case nil, bool, string:
		return val
	case int:
		return []any{"num", float64(val)}
	case int64:
		return []any{"num", float64(val)}
	case float64:
		return []any{"num", val}
	case map[string]any:
		keys := make([]string, 0, len(val))
		for k := range val {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		items := make([]any, 0, len(keys))
		for _, k := range keys {
			items = append(items, []any{k, detailValue(val[k])})
		}
		return []any{"map", items}
	case []any:
		items := make([]any, 0, len(val))
		for _, item := range val {
			items = append(items, detailValue(item))
		}
		return []any{"list", items}
	default:
		return []any{"other", fmt.Sprint(val)}
	}
}

// detailOf converts a parsed AST into the comparable dump shape.
func detailOf(ast *ScriptAST) detailAST {
	dump := detailAST{
		DataDefs:  []detailData{},
		RuleDefs:  []detailRule{},
		EventDefs: []detailEvent{},
		FuncDefs:  []detailFunc{},
		Metadata:  []detailMeta{},
	}
	for _, def := range ast.DataDefs {
		keys := make([]string, 0, len(def.Data))
		for k := range def.Data {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		dump.DataDefs = append(dump.DataDefs, detailData{
			TypeName: def.TypeName, IsEntity: def.IsEntity, Keys: keys,
			Data: detailValue(def.Data), Line: def.Pos.Line,
		})
	}
	for _, rule := range ast.RuleDefs {
		subjects := []detailSubject{}
		for _, s := range rule.Subjects {
			subjects = append(subjects, detailSubject{
				Var: optionalString(s.Var), Kind: s.Kind,
				TypeName: optionalString(s.TypeName), Condition: optionalString(s.Condition),
				EntityID: optionalString(s.EntityID),
			})
		}
		actions := []detailAction{}
		for _, a := range rule.Actions {
			actions = append(actions, detailAction{
				Op: a.Op, Target: a.Target, Subject: optionalInt(a.Subject),
				Entity: optionalString(a.Entity), Value: detailValue(a.Value),
			})
		}
		branches := []detailBranch{}
		for _, b := range rule.Branches {
			var state any
			if b.State != nil {
				state = detailValue(b.State)
			}
			branch := detailBranch{State: state}
			if b.Condition != nil {
				branch.Condition = *b.Condition
			}
			branches = append(branches, branch)
		}
		conditions := rule.Conditions
		if conditions == nil {
			conditions = []string{}
		}
		var stateWhen any
		if rule.StateWhen != nil {
			stateWhen = *rule.StateWhen
		}
		var priority any
		if rule.Priority != nil {
			priority = *rule.Priority
		}
		var lineForm any
		if rule.LineForm != nil {
			lineForm = *rule.LineForm
		}
		dump.RuleDefs = append(dump.RuleDefs, detailRule{
			Subjects: subjects, HeaderRaw: rule.HeaderRaw,
			RuleID: optionalString(rule.RuleID), StateWhen: stateWhen, Priority: priority,
			Conditions: conditions, Actions: actions, Branches: branches,
			LineForm: lineForm, Line: rule.Pos.Line,
		})
	}
	for _, event := range ast.EventDefs {
		actions := event.Actions
		if actions == nil {
			actions = []any{}
		}
		dump.EventDefs = append(dump.EventDefs, detailEvent{
			Name: event.Name, Condition: optionalString(event.Condition),
			Handler: optionalString(event.Handler), Actions: actions, Line: event.Pos.Line,
		})
	}
	for _, fn := range ast.FuncDefs {
		params := [][2]any{}
		for _, p := range fn.Params {
			params = append(params, [2]any{p.Name, p.Type})
		}
		dump.FuncDefs = append(dump.FuncDefs, detailFunc{
			Name: fn.Name, Params: params, ReturnType: optionalString(fn.ReturnType),
			Body: fn.Body, Line: fn.Pos.Line,
		})
	}
	for _, meta := range ast.Metadata {
		dump.Metadata = append(dump.Metadata, detailMeta{Key: meta.Key, Value: meta.Value, Line: meta.Pos.Line})
	}
	return dump
}

// synthScripts covers block forms absent from the corpus (@多规则, threshold
// range rules, multi-if, legacy key:value rules and @描述/@data/@函数 variants).
var synthScripts = map[string]string{
	"multicond": "@多规则[$[NPC:猫猫]] {\n" +
		"  优先级: 300\n  id: multi-1\n  规则A: {\n    条件: $[NPC:猫猫].身体.体力 < 20\n" +
		"    动作: $.状态.触发 = \"低\"\n  },\n  规则B: {\n    动作: $.状态.触发 = \"高\"\n" +
		"    动作: $.计数 += 1\n  }\n}\n",
	"range": "@规则[$[NPC:猫猫]]:$.状态.情绪 {\n  =开心: { 动作: $.状态.情绪值 = 100 },\n" +
		"  >50: { 动作: $.状态.情绪值 += 1 },\n  30: { 动作: $.状态.情绪值 = 30 },\n" +
		"  20.5: { 动作: $.状态.情绪值 = 20.5 },\n  悲伤: { 动作: $.状态.情绪值 = 0 }\n}\n",
	"multiif": "@规则[$[NPC:猫猫]] {\n  优先级: 10\n  id: mi-1\n" +
		"  $[NPC:猫猫].身体.饱食 < 40 ∧ $[NPC:猫猫].状态.情绪 in {开心, 平静} {\n" +
		"    $.状态.触发 = \"饿\"\n  },\n  {e}.标记.疲劳 == true {\n" +
		"    动作: $.状态.触发 = \"累\"\n  },\n  $[NPC:猫猫].行为.主人行为 in {无} {\n" +
		"    $.状态.触发 = \"等\"\n  }\n}\n",
	"legacy": "@规则[$1[NPC.性别=女]，场景] {\n  id: legacy-1\n  优先级: 200\n" +
		"  状态: 身体.疲劳 > 80\n  条件: $1.身体.疲劳 > 90\n  条件: 场景.天气 == \"雨\"\n" +
		"  动作: $1.亲密 += 5\n  动作: 场景.事件 += {\"类型\": \"见面\"}\n  动作: $1.计数 -= 2\n" +
		"  动作: {e}.标记.疲劳 = true\n  分支: {\n    体力 < 10 → 状态 = 昏厥\n" +
		"    默认 → 状态.情绪 = \"平静\"\n  }\n}\n",
	"eventfunc": "@描述 模块 {\n  说明: 只是文档\n}\n@data 通用块 {\n  \"键\": 1\n}\n" +
		"@定义 定义块 {\n  \"键\": 2\n}\n@事件:心跳 {\n  条件: 变化量 > 0\n  处理器: onTick\n" +
		"  标记 = 1\n}\n@函数 加法(a: int, b: int) -> int {\n  返回 a + b\n}\n" +
		"#世界观: 合成样例\n#规则: $[NPC].a = 1 → $.b = 2\n",
}

// TestDetailDump prints value-level AST summaries when NEKO_DETAIL_FILES is set,
// and dumps the built-in synthetic scripts when NEKO_DETAIL_SYNTH=1.
func TestDetailDump(t *testing.T) {
	raw := os.Getenv("NEKO_DETAIL_FILES")
	if raw == "" && os.Getenv("NEKO_DETAIL_SYNTH") != "1" {
		t.Skip("set NEKO_DETAIL_FILES or NEKO_DETAIL_SYNTH to print value-level AST summaries")
	}
	if os.Getenv("NEKO_DETAIL_SYNTH") == "1" {
		names := make([]string, 0, len(synthScripts))
		for name := range synthScripts {
			names = append(names, name)
		}
		sort.Strings(names)
		for _, name := range names {
			text := synthScripts[name]
			quoted, err := json.Marshal(text)
			if err != nil {
				t.Fatalf("marshal synth %s: %v", name, err)
			}
			payload, err := json.Marshal(detailOf(Parse(text)))
			if err != nil {
				t.Fatalf("marshal detail %s: %v", name, err)
			}
			fmt.Printf("SYNTH\t%s\t%s\n", name, quoted)
			fmt.Printf("DETAIL\t%s\t%s\n", name, payload)
		}
	}
	for _, rel := range splitNonEmpty(raw) {
		path := filepath.Join(corpusDir, rel)
		if filepath.IsAbs(rel) {
			path = rel
		}
		rawText, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", rel, err)
		}
		payload, err := json.Marshal(detailOf(Parse(string(rawText))))
		if err != nil {
			t.Fatalf("marshal %s: %v", rel, err)
		}
		fmt.Printf("DETAIL\t%s\t%s\n", rel, payload)
	}
}

// splitNonEmpty splits a comma-separated list, dropping blank entries.
func splitNonEmpty(raw string) []string {
	out := []string{}
	for _, part := range strings.Split(raw, ",") {
		part = strings.TrimSpace(part)
		if part != "" {
			out = append(out, part)
		}
	}
	return out
}

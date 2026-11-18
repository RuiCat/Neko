// r18Extract.go turns the read-only event digest corpus into the C7 R18 rows.
// Every corpus event carries 阶段流程 (stage beats with dialogue), CG数 and
// 状态操作; only 阶段流程 is semantic data, so this file extracts the stages as
// steps and records the absent 效果/条件 fields as a gap instead of inventing
// effects to make the C7 structure check pass.
package task

import (
	"fmt"
	"os"
	"strings"

	"github.com/RuiCat/Neko/internal/flow/spec"
)

// Corpus field names read by this file. 效果 and 条件 belong to the kernel's
// 事件模板 shape (scripts/kernel/event/eventModel.neko) but no corpus event has them.
const (
	R18_FIELD_STAGES     = "阶段流程"
	R18_FIELD_STAGE      = "阶段"
	R18_FIELD_DIALOG     = "对话"
	R18_FIELD_EFFECTS    = "效果"
	R18_FIELD_CONDITIONS = "条件"
	R18_FIELD_ACTIONS    = "状态操作"
)

// r18Event is one extracted event plus the field presence the gap report needs, so
// the artifact can state exactly which C7 input the corpus does not carry.
type r18Event struct {
	flow          R18Flow
	hasEffects    bool
	hasConditions bool
	actionCount   int
}

// ExtractR18Flows parses the corpus and returns the artifact. Read, parse and
// structure errors are fatal: a partial artifact would hide a missing event.
func ExtractR18Flows(corpusPath string) (R18FlowsFile, error) {
	raw, err := os.ReadFile(corpusPath)
	if err != nil {
		return R18FlowsFile{}, fmt.Errorf("read r18 corpus: %w", err)
	}
	document := spec.ParseDocument(raw)
	if len(document.Issues) > 0 {
		issue := document.Issues[0]
		return R18FlowsFile{}, fmt.Errorf("parse r18 corpus: %d issue(s), first at line %d: %s",
			len(document.Issues), issue.Line, issue.Message)
	}
	entries, err := eventEntries(document)
	if err != nil {
		return R18FlowsFile{}, err
	}
	events := make([]r18Event, 0, len(entries))
	for _, entry := range entries {
		event, err := r18EventOf(entry, corpusPath)
		if err != nil {
			return R18FlowsFile{}, err
		}
		events = append(events, event)
	}
	flows := make([]R18Flow, 0, len(events))
	for _, event := range events {
		flows = append(flows, event.flow)
	}
	return R18FlowsFile{
		SchemaVersion: R18_SCHEMA_VERSION,
		Corpus:        corpusPath,
		Gaps:          r18Gaps(events),
		R18Flows:      flows,
	}, nil
}

// eventEntries returns the corpus events. The corpus holds exactly one 定义 block;
// accepting more would silently drop a whole event set.
func eventEntries(document spec.Document) ([]spec.Entry, error) {
	candidates := []spec.Block{}
	for _, block := range document.Blocks {
		if !block.Skipped && len(block.Entries) > 0 {
			candidates = append(candidates, block)
		}
	}
	if len(candidates) != 1 {
		return nil, fmt.Errorf("r18 corpus: want exactly 1 event block, got %d", len(candidates))
	}
	return candidates[0].Entries, nil
}

// r18EventOf extracts one corpus event. A present 效果 or 条件 value is refused
// instead of parsed: their shape is not frozen, so mapping them now would be a
// guess that the artifact cannot audit.
func r18EventOf(entry spec.Entry, corpusPath string) (r18Event, error) {
	if entry.Value.Kind != spec.VALUE_OBJECT {
		return r18Event{}, fmt.Errorf("r18 event %q: body is not an object", entry.Key)
	}
	stages, found := findEntry(entry.Value.Entries, R18_FIELD_STAGES)
	if !found {
		return r18Event{}, fmt.Errorf("r18 event %q: missing %s", entry.Key, R18_FIELD_STAGES)
	}
	steps, err := r18StepsOf(stages)
	if err != nil {
		return r18Event{}, fmt.Errorf("r18 event %q: %w", entry.Key, err)
	}
	_, hasEffects := findEntry(entry.Value.Entries, R18_FIELD_EFFECTS)
	if hasEffects {
		return r18Event{}, fmt.Errorf("r18 event %q: corpus carries %s whose shape is not frozen; refusing to guess", entry.Key, R18_FIELD_EFFECTS)
	}
	_, hasConditions := findEntry(entry.Value.Entries, R18_FIELD_CONDITIONS)
	if hasConditions {
		return r18Event{}, fmt.Errorf("r18 event %q: corpus carries %s whose shape is not frozen; refusing to guess", entry.Key, R18_FIELD_CONDITIONS)
	}
	actions := 0
	if value, ok := findEntry(entry.Value.Entries, R18_FIELD_ACTIONS); ok {
		actions = len(value.Items)
	}
	return r18Event{
		flow: R18Flow{
			ID:      entry.Key,
			Source:  spec.Source{File: corpusPath, Line: entry.KeyLine},
			Steps:   steps,
			Effects: []spec.EffectSpec{},
		},
		hasEffects:    hasEffects,
		hasConditions: hasConditions,
		actionCount:   actions,
	}, nil
}

// r18StepsOf expands 阶段流程 into one step per stage beat. The dialogue is joined
// with newlines so the corpus line structure survives without inventing prose.
func r18StepsOf(value spec.Value) ([]string, error) {
	if value.Kind != spec.VALUE_ARRAY {
		return nil, fmt.Errorf("%s is not an array", R18_FIELD_STAGES)
	}
	steps := make([]string, 0, len(value.Items))
	for index, item := range value.Items {
		step, err := r18StepOf(item)
		if err != nil {
			return nil, fmt.Errorf("%s[%d]: %w", R18_FIELD_STAGES, index, err)
		}
		steps = append(steps, step)
	}
	if len(steps) == 0 {
		return nil, fmt.Errorf("%s is empty", R18_FIELD_STAGES)
	}
	return steps, nil
}

// r18StepOf renders one stage beat as "<阶段>：<dialogue lines>".
func r18StepOf(item spec.Value) (string, error) {
	if item.Kind != spec.VALUE_OBJECT {
		return "", fmt.Errorf("stage beat is not an object")
	}
	stage, err := stringField(item.Entries, R18_FIELD_STAGE)
	if err != nil {
		return "", err
	}
	dialogue, err := stringArrayField(item.Entries, R18_FIELD_DIALOG)
	if err != nil {
		return "", err
	}
	if len(dialogue) == 0 {
		return stage, nil
	}
	return stage + "：" + strings.Join(dialogue, "\n"), nil
}

// r18Gaps states, with counts computed from the parsed corpus, which C7 inputs the
// corpus does not provide. The strings are stable across runs.
func r18Gaps(events []r18Event) []string {
	total := len(events)
	effects := 0
	conditions := 0
	actions := 0
	totalActions := 0
	for _, event := range events {
		if event.hasEffects {
			effects++
		}
		if event.hasConditions {
			conditions++
		}
		if event.actionCount > 0 {
			actions++
			totalActions += event.actionCount
		}
	}
	return []string{
		fmt.Sprintf("effects: %d/%d 事件有 效果 字段，因此每条 effects 为空；事件模板内核把 效果 与 阶段流程 分开（scripts/kernel/event/eventModel.neko），阶段流程 不能当作 effects，C7 的 effects 非空要求在本语料上无法由抽取满足", effects, total),
		fmt.Sprintf("conditions: %d/%d 事件有 条件 字段；C7 断言的 条件 在本语料无来源", conditions, total),
		fmt.Sprintf("状态操作: %d/%d 事件非空（共 %d 条）；该字段是序列化位置（docs/container-kernel-design.md:351），语义不可恢复，按 docs/legacy-extraction-plan.md 不导入", actions, total, totalActions),
	}
}

// findEntry returns the value of one object field.
func findEntry(entries []spec.Entry, key string) (spec.Value, bool) {
	for _, entry := range entries {
		if entry.Key == key {
			return entry.Value, true
		}
	}
	return spec.Value{}, false
}

// stringField reads one required string field.
func stringField(entries []spec.Entry, key string) (string, error) {
	value, ok := findEntry(entries, key)
	if !ok {
		return "", fmt.Errorf("missing %s", key)
	}
	if value.Kind != spec.VALUE_STRING {
		return "", fmt.Errorf("%s is not a string", key)
	}
	if strings.TrimSpace(value.Text) == "" {
		return "", fmt.Errorf("%s is empty", key)
	}
	return strings.TrimSpace(value.Text), nil
}

// stringArrayField reads one string array field; an absent field is an empty list
// because a stage beat may legitimately carry no dialogue.
func stringArrayField(entries []spec.Entry, key string) ([]string, error) {
	value, ok := findEntry(entries, key)
	if !ok {
		return nil, nil
	}
	if value.Kind != spec.VALUE_ARRAY {
		return nil, fmt.Errorf("%s is not an array", key)
	}
	items := make([]string, 0, len(value.Items))
	for _, item := range value.Items {
		if item.Kind != spec.VALUE_STRING {
			return nil, fmt.Errorf("%s holds a non-string item", key)
		}
		items = append(items, item.Text)
	}
	return items, nil
}

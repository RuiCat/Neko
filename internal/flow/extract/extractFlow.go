// extractFlow.go maps parsed @定义 entries onto FlowSpec records, effect
// occurrences and migration anchors. It knows the three legacy shapes used by
// P4: behavior templates, task template libraries and the R18 flow index.
package extract

import . "github.com/RuiCat/Neko/internal/flow/spec"

import (
	"fmt"
	"strconv"
	"strings"
)

type extractedDocument struct {
	Flows         []FlowSpec
	TaskTemplates []TaskTemplateSpec
	Records       []MigrationRecord
	Containers    []Container
	Skipped       []SkippedBlock
	Issues        []ParseIssue
	Unclassified  []UnclassifiedEffect
}

func extractDocument(file string, doc Document) extractedDocument {
	out := extractedDocument{
		Flows:         []FlowSpec{},
		TaskTemplates: []TaskTemplateSpec{},
		Records:       []MigrationRecord{},
		Containers:    []Container{},
		Skipped:       []SkippedBlock{},
		Issues:        []ParseIssue{},
		Unclassified:  []UnclassifiedEffect{},
	}
	for _, issue := range doc.Issues {
		out.Issues = append(out.Issues, ParseIssue{File: file, Line: issue.Line, Message: issue.Message})
	}
	for _, block := range doc.Blocks {
		if block.Directive != "定义" {
			out.Skipped = append(out.Skipped, SkippedBlock{
				File:      file,
				Line:      block.Line,
				Directive: "@" + block.Directive,
				Reason:    "legacy rule block; P4 executes on current kernel rules",
			})
			continue
		}
		out.Containers = append(out.Containers, Container{File: file, Line: block.Line, Name: block.Name})
		switch block.Name {
		case "行为模板":
			for _, entry := range block.Entries {
				flow, unclassified, issues := behaviorFlow(file, entry)
				out.Flows = append(out.Flows, flow)
				out.Unclassified = append(out.Unclassified, unclassified...)
				out.Issues = append(out.Issues, issues...)
				out.Records = append(out.Records, MigrationRecord{
					File: file, Line: entry.KeyLine, Key: entry.Key, Kind: "flow",
				})
			}
		case "任务模板库", "任务模板":
			for _, entry := range block.Entries {
				template, issues := taskTemplateSpec(file, entry)
				out.TaskTemplates = append(out.TaskTemplates, template)
				out.Issues = append(out.Issues, issues...)
				out.Records = append(out.Records, MigrationRecord{
					File: file, Line: entry.KeyLine, Key: entry.Key, Kind: "taskTemplate",
				})
			}
		case "性行为流程库":
			flows, records, issues := r18Flows(file, block)
			out.Flows = append(out.Flows, flows...)
			out.Records = append(out.Records, records...)
			out.Issues = append(out.Issues, issues...)
		case "角色基础":
			out.Skipped = append(out.Skipped, SkippedBlock{
				File: file, Line: block.Line, Directive: "@定义 NPC",
				Reason: "state schema definition; not a K1 flow or task template",
			})
		default:
			out.Skipped = append(out.Skipped, SkippedBlock{
				File: file, Line: block.Line, Directive: "@定义 " + block.Name,
				Reason: "unsupported definition for K1 extraction",
			})
		}
	}
	return out
}

func behaviorFlow(file string, entry Entry) (FlowSpec, []UnclassifiedEffect, []ParseIssue) {
	flow := FlowSpec{
		ID:       entry.Key,
		Source:   Source{File: file, Line: entry.KeyLine},
		Requires: []string{},
		Consume:  []string{},
		Produce:  []ItemSpec{},
		Steps:    []string{},
		Effects:  []EffectSpec{},
		Intents:  []IntentSpec{},
	}
	issues := []ParseIssue{}
	unclassified := []UnclassifiedEffect{}
	if entry.Value.Kind != VALUE_OBJECT {
		issues = append(issues, ParseIssue{
			File: file, Line: entry.KeyLine,
			Message: fmt.Sprintf("flow %q: body is not an object", entry.Key),
		})
		return flow, unclassified, issues
	}
	obj := newObjectView(entry.Value.Entries)
	if value, ok := obj.get("场景"); ok {
		if value.Value.Kind == VALUE_STRING {
			flow.Scene = value.Value.Text
		} else {
			issues = append(issues, ParseIssue{
				File: file, Line: value.Value.Line,
				Message: fmt.Sprintf("flow %q: 场景 is not a string", entry.Key),
			})
		}
	}
	flow.Requires = readStringArray(obj, "需要物品", entry.Key, file, &issues)
	flow.Consume = readStringArray(obj, "消耗", entry.Key, file, &issues)
	flow.Steps = readStringArray(obj, "步骤", entry.Key, file, &issues)
	cost, costIssues := readCost(obj, entry.Key, file)
	flow.Cost = cost
	issues = append(issues, costIssues...)
	produce, produceIssues := readProduce(obj, entry.Key, file)
	flow.Produce = produce
	issues = append(issues, produceIssues...)
	effects, unclassified, effectIssues := readEffects(obj, entry.Key, file)
	flow.Effects = effects
	issues = append(issues, effectIssues...)
	flow.Intents = deriveIntents(flow.ID, flow.Consume, flow.Produce)
	return flow, unclassified, issues
}

// deriveIntents turns a flow name into the kernel requests settlement writes.
// The rules are deliberately narrow and ordered: a trade name wins, then the
// interaction verb, and everything else stays empty instead of guessing.
//
//	买/购 -> trade buy, object = first consume/produce item
//	吃    -> interaction eat, target = first consume item
//	喝    -> interaction drink, target = first consume item
//	做/煮/洗/使用 -> interaction use, target = first consume item
func deriveIntents(flowID string, consume []string, produce []ItemSpec) []IntentSpec {
	firstConsume := ""
	if len(consume) > 0 {
		firstConsume = consume[0]
	}
	if strings.Contains(flowID, "买") || strings.Contains(flowID, "购") {
		object := firstConsume
		if object == "" && len(produce) > 0 {
			object = produce[0].Name
		}
		return []IntentSpec{{TradeSide: "买", Object: object}}
	}
	for _, verb := range []string{"吃", "喝", "使用", "做", "煮", "洗"} {
		if !strings.Contains(flowID, verb) {
			continue
		}
		action := verb
		if action == "做" || action == "煮" || action == "洗" {
			action = "使用"
		}
		return []IntentSpec{{Action: action, Target: firstConsume}}
	}
	return []IntentSpec{}
}

func r18Flows(file string, block Block) ([]FlowSpec, []MigrationRecord, []ParseIssue) {
	flows := []FlowSpec{}
	records := []MigrationRecord{}
	issues := []ParseIssue{}
	obj := newObjectView(block.Entries)
	index, ok := obj.get("流程索引")
	if !ok {
		issues = append(issues, ParseIssue{File: file, Line: block.Line, Message: "性行为流程库: missing 流程索引"})
		return flows, records, issues
	}
	if index.Value.Kind != VALUE_OBJECT {
		issues = append(issues, ParseIssue{File: file, Line: index.Value.Line, Message: "流程索引 is not an object"})
		return flows, records, issues
	}
	dialogs := map[string][]string{}
	if summary, ok := obj.get("流程对话摘要"); ok {
		if summary.Value.Kind == VALUE_OBJECT {
			for _, entry := range summary.Value.Entries {
				lines, err := stringArrayValue(entry.Value)
				if err != nil {
					issues = append(issues, ParseIssue{
						File: file, Line: entry.KeyLine,
						Message: fmt.Sprintf("流程对话摘要 %q: %v", entry.Key, err),
					})
					continue
				}
				dialogs[entry.Key] = lines
			}
		} else {
			issues = append(issues, ParseIssue{File: file, Line: summary.Value.Line, Message: "流程对话摘要 is not an object"})
		}
	}
	for _, entry := range index.Value.Entries {
		flow := FlowSpec{
			ID:       entry.Key,
			Source:   Source{File: file, Line: entry.KeyLine},
			Requires: []string{},
			Consume:  []string{},
			Produce:  []ItemSpec{},
			Steps:    []string{},
			Effects:  []EffectSpec{},
		}
		if lines, ok := dialogs[entry.Key]; ok {
			flow.Steps = lines
		} else {
			issues = append(issues, ParseIssue{
				File: file, Line: entry.KeyLine,
				Message: fmt.Sprintf("r18 flow %q: missing 流程对话摘要", entry.Key),
			})
		}
		if entry.Value.Kind == VALUE_OBJECT {
			meta := newObjectView(entry.Value.Entries)
			if stepCount, ok := meta.get("步骤"); ok && stepCount.Value.Kind != VALUE_NUMBER {
				issues = append(issues, ParseIssue{
					File: file, Line: stepCount.Value.Line,
					Message: fmt.Sprintf("r18 flow %q: 步骤 is not a number", entry.Key),
				})
			}
		}
		flow.Intents = deriveIntents(flow.ID, flow.Consume, flow.Produce)
		flows = append(flows, flow)
		records = append(records, MigrationRecord{
			File: file, Line: entry.KeyLine, Key: entry.Key, Kind: "r18Flow",
		})
	}
	return flows, records, issues
}

type objectView struct {
	entries []Entry
	index   map[string]Entry
}

func newObjectView(entries []Entry) objectView {
	index := make(map[string]Entry, len(entries))
	for _, entry := range entries {
		if _, exists := index[entry.Key]; !exists {
			index[entry.Key] = entry
		}
	}
	return objectView{entries: entries, index: index}
}

func (o objectView) get(key string) (Entry, bool) {
	entry, ok := o.index[key]
	return entry, ok
}

func readStringArray(obj objectView, key, flowID, file string, issues *[]ParseIssue) []string {
	entry, ok := obj.get(key)
	if !ok {
		return []string{}
	}
	if entry.Value.Kind != VALUE_ARRAY {
		*issues = append(*issues, ParseIssue{
			File: file, Line: entry.Value.Line,
			Message: fmt.Sprintf("flow %q: %s is not an array", flowID, key),
		})
		return []string{}
	}
	out := make([]string, 0, len(entry.Value.Items))
	for _, item := range entry.Value.Items {
		if item.Kind != VALUE_STRING {
			*issues = append(*issues, ParseIssue{
				File: file, Line: item.Line,
				Message: fmt.Sprintf("flow %q: %s contains a non-string item", flowID, key),
			})
			continue
		}
		out = append(out, item.Text)
	}
	return out
}

func readCost(obj objectView, flowID, file string) (int, []ParseIssue) {
	entry, ok := obj.get("花费")
	if !ok {
		return 0, []ParseIssue{}
	}
	if entry.Value.Kind != VALUE_NUMBER {
		return 0, []ParseIssue{{
			File: file, Line: entry.Value.Line,
			Message: fmt.Sprintf("flow %q: 花费 is not a number", flowID),
		}}
	}
	cost, err := strconv.Atoi(entry.Value.Text)
	if err != nil {
		return 0, []ParseIssue{{
			File: file, Line: entry.Value.Line,
			Message: fmt.Sprintf("flow %q: 花费 %q is not an integer", flowID, entry.Value.Text),
		}}
	}
	return cost, []ParseIssue{}
}

func readProduce(obj objectView, flowID, file string) ([]ItemSpec, []ParseIssue) {
	entry, ok := obj.get("产出")
	if !ok {
		return []ItemSpec{}, []ParseIssue{}
	}
	switch entry.Value.Kind {
	case VALUE_OBJECT:
		item, issues := itemFromObject(entry.Value, flowID, "产出", file)
		return []ItemSpec{item}, issues
	case VALUE_ARRAY:
		items := make([]ItemSpec, 0, len(entry.Value.Items))
		issues := []ParseIssue{}
		for _, value := range entry.Value.Items {
			item, itemIssues := itemFromObject(value, flowID, "产出", file)
			items = append(items, item)
			issues = append(issues, itemIssues...)
		}
		return items, issues
	default:
		return []ItemSpec{}, []ParseIssue{{
			File: file, Line: entry.Value.Line,
			Message: fmt.Sprintf("flow %q: 产出 is neither an object nor an array", flowID),
		}}
	}
}

func itemFromObject(value Value, flowID, field, file string) (ItemSpec, []ParseIssue) {
	item := ItemSpec{}
	if value.Kind != VALUE_OBJECT {
		return item, []ParseIssue{{
			File: file, Line: value.Line,
			Message: fmt.Sprintf("flow %q: %s item is not an object", flowID, field),
		}}
	}
	obj := newObjectView(value.Entries)
	issues := []ParseIssue{}
	assignString := func(key string, target *string) {
		entry, ok := obj.get(key)
		if !ok {
			return
		}
		if entry.Value.Kind != VALUE_STRING {
			issues = append(issues, ParseIssue{
				File: file, Line: entry.Value.Line,
				Message: fmt.Sprintf("flow %q: %s.%s is not a string", flowID, field, key),
			})
			return
		}
		*target = entry.Value.Text
	}
	assignString("类型", &item.Type)
	assignString("名称", &item.Name)
	assignString("描述", &item.Description)
	return item, issues
}

func readEffects(obj objectView, flowID, file string) ([]EffectSpec, []UnclassifiedEffect, []ParseIssue) {
	entry, ok := obj.get("效果")
	if !ok {
		return []EffectSpec{}, []UnclassifiedEffect{}, []ParseIssue{}
	}
	if entry.Value.Kind != VALUE_OBJECT {
		return []EffectSpec{}, []UnclassifiedEffect{}, []ParseIssue{{
			File: file, Line: entry.Value.Line,
			Message: fmt.Sprintf("flow %q: 效果 is not an object", flowID),
		}}
	}
	effects := make([]EffectSpec, 0, len(entry.Value.Entries))
	unclassified := []UnclassifiedEffect{}
	issues := []ParseIssue{}
	for _, effectEntry := range entry.Value.Entries {
		source := Source{File: file, Line: effectEntry.KeyLine}
		effect, rejected := classifyEffect(effectEntry.Key, effectEntry.Value, source)
		if rejected != nil {
			unclassified = append(unclassified, *rejected)
			continue
		}
		effects = append(effects, effect)
	}
	return effects, unclassified, issues
}

func stringArrayValue(value Value) ([]string, error) {
	if value.Kind != VALUE_ARRAY {
		return nil, fmt.Errorf("value is not an array")
	}
	out := make([]string, 0, len(value.Items))
	for _, item := range value.Items {
		if item.Kind != VALUE_STRING {
			return nil, fmt.Errorf("array contains a non-string item")
		}
		out = append(out, item.Text)
	}
	return out, nil
}

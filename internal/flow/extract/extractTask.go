// extractTask.go normalizes legacy task templates into TaskTemplateSpec. The
// library shape stores 阶段/条件/奖励/对话 as arrays/objects; cat_quest is a
// partial shape (single-string 阶段/条件/奖励, no 对话). Stages map 已激活 to
// 未激活 here per C4; missing example/dialog stay empty in this file and may be
// filled from the same-id library template in Generate, never invented.
package extract

import . "github.com/RuiCat/Neko/internal/flow/spec"

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

var (
	taskExpPattern      = regexp.MustCompile(`经验\s*([0-9]+)`)
	taskAffinityPattern = regexp.MustCompile(`好感度?\s*([0-9]+)`)
)

func taskTemplateSpec(file string, entry Entry) (TaskTemplateSpec, []ParseIssue) {
	spec := TaskTemplateSpec{
		ID:     entry.Key,
		Source: Source{File: file, Line: entry.KeyLine},
		Stages: []string{},
	}
	issues := []ParseIssue{}
	if entry.Value.Kind != VALUE_OBJECT {
		issues = append(issues, ParseIssue{
			File: file, Line: entry.KeyLine,
			Message: fmt.Sprintf("task template %q: body is not an object", entry.Key),
		})
		return spec, issues
	}
	obj := newObjectView(entry.Value.Entries)
	spec.Stages = readTaskStages(obj, entry.Key, file, &issues)
	if len(spec.Stages) == 0 {
		issues = append(issues, ParseIssue{
			File: file, Line: entry.KeyLine,
			Message: fmt.Sprintf("task template %q: no 阶段", entry.Key),
		})
	}
	spec.Condition = readTaskCondition(obj, entry.Key, file, &issues)
	if spec.Condition.Type == "" {
		issues = append(issues, ParseIssue{
			File: file, Line: entry.KeyLine,
			Message: fmt.Sprintf("task template %q: empty 条件", entry.Key),
		})
	}
	spec.Reward = readTaskReward(obj, entry.Key, file, &issues)
	spec.Dialog = readTaskDialog(obj, entry.Key, file, &issues)
	return spec, issues
}

func readTaskStages(obj objectView, templateID, file string, issues *[]ParseIssue) []string {
	entry, ok := obj.get("阶段")
	if !ok {
		return []string{}
	}
	stages := []string{}
	switch entry.Value.Kind {
	case VALUE_ARRAY:
		for _, item := range entry.Value.Items {
			if item.Kind != VALUE_STRING {
				*issues = append(*issues, ParseIssue{
					File: file, Line: item.Line,
					Message: fmt.Sprintf("task template %q: 阶段 contains a non-string item", templateID),
				})
				continue
			}
			stages = append(stages, normalizeTaskStage(item.Text))
		}
	case VALUE_STRING:
		stages = append(stages, normalizeTaskStage(entry.Value.Text))
	default:
		*issues = append(*issues, ParseIssue{
			File: file, Line: entry.Value.Line,
			Message: fmt.Sprintf("task template %q: 阶段 is neither an array nor a string", templateID),
		})
	}
	return stages
}

func normalizeTaskStage(stage string) string {
	if stage == "已激活" {
		return "未激活"
	}
	return stage
}

func readTaskCondition(obj objectView, templateID, file string, issues *[]ParseIssue) TaskCondition {
	entry, ok := obj.get("条件")
	if !ok {
		return TaskCondition{}
	}
	condition := TaskCondition{}
	switch entry.Value.Kind {
	case VALUE_OBJECT:
		conditionObj := newObjectView(entry.Value.Entries)
		condition.Type = readTaskString(conditionObj, "类型", templateID, "条件", file, issues)
		condition.Example = readTaskString(conditionObj, "示例", templateID, "条件", file, issues)
	case VALUE_STRING:
		condition.Type = entry.Value.Text
	default:
		*issues = append(*issues, ParseIssue{
			File: file, Line: entry.Value.Line,
			Message: fmt.Sprintf("task template %q: 条件 is neither an object nor a string", templateID),
		})
	}
	return condition
}

func readTaskReward(obj objectView, templateID, file string, issues *[]ParseIssue) TaskReward {
	entry, ok := obj.get("奖励")
	if !ok {
		return TaskReward{}
	}
	reward := TaskReward{}
	switch entry.Value.Kind {
	case VALUE_OBJECT:
		rewardObj := newObjectView(entry.Value.Entries)
		reward.Experience = readTaskInt(rewardObj, "经验", templateID, "奖励", file, issues)
		reward.Affinity = readTaskInt(rewardObj, "好感度", templateID, "奖励", file, issues)
	case VALUE_STRING:
		parsed, ok := parseTaskRewardString(entry.Value.Text)
		if !ok {
			*issues = append(*issues, ParseIssue{
				File: file, Line: entry.Value.Line,
				Message: fmt.Sprintf("task template %q: cannot parse 奖励 %q", templateID, entry.Value.Text),
			})
			return reward
		}
		reward = parsed
	default:
		*issues = append(*issues, ParseIssue{
			File: file, Line: entry.Value.Line,
			Message: fmt.Sprintf("task template %q: 奖励 is neither an object nor a string", templateID),
		})
	}
	return reward
}

func parseTaskRewardString(text string) (TaskReward, bool) {
	reward := TaskReward{}
	matched := false
	if groups := taskExpPattern.FindStringSubmatch(text); groups != nil {
		value, err := strconv.Atoi(groups[1])
		if err == nil {
			reward.Experience = value
			matched = true
		}
	}
	if groups := taskAffinityPattern.FindStringSubmatch(text); groups != nil {
		value, err := strconv.Atoi(groups[1])
		if err == nil {
			reward.Affinity = value
			matched = true
		}
	}
	return reward, matched
}

func readTaskDialog(obj objectView, templateID, file string, issues *[]ParseIssue) TaskDialog {
	entry, ok := obj.get("对话")
	if !ok {
		return TaskDialog{}
	}
	if entry.Value.Kind != VALUE_OBJECT {
		*issues = append(*issues, ParseIssue{
			File: file, Line: entry.Value.Line,
			Message: fmt.Sprintf("task template %q: 对话 is not an object", templateID),
		})
		return TaskDialog{}
	}
	dialogObj := newObjectView(entry.Value.Entries)
	return TaskDialog{
		Activate:   readTaskString(dialogObj, "激活", templateID, "对话", file, issues),
		InProgress: readTaskString(dialogObj, "进行中", templateID, "对话", file, issues),
		Complete:   readTaskString(dialogObj, "完成", templateID, "对话", file, issues),
	}
}

func readTaskString(obj objectView, key, templateID, parent, file string, issues *[]ParseIssue) string {
	entry, ok := obj.get(key)
	if !ok {
		return ""
	}
	if entry.Value.Kind != VALUE_STRING {
		*issues = append(*issues, ParseIssue{
			File: file, Line: entry.Value.Line,
			Message: fmt.Sprintf("task template %q: %s.%s is not a string", templateID, parent, key),
		})
		return ""
	}
	return entry.Value.Text
}

func readTaskInt(obj objectView, key, templateID, parent, file string, issues *[]ParseIssue) int {
	entry, ok := obj.get(key)
	if !ok {
		return 0
	}
	if entry.Value.Kind != VALUE_NUMBER {
		*issues = append(*issues, ParseIssue{
			File: file, Line: entry.Value.Line,
			Message: fmt.Sprintf("task template %q: %s.%s is not a number", templateID, parent, key),
		})
		return 0
	}
	value, err := strconv.Atoi(entry.Value.Text)
	if err != nil {
		*issues = append(*issues, ParseIssue{
			File: file, Line: entry.Value.Line,
			Message: fmt.Sprintf("task template %q: %s.%s %q is not an integer", templateID, parent, key, entry.Value.Text),
		})
		return 0
	}
	return value
}

func hasTaskDialog(dialog TaskDialog) bool {
	return strings.TrimSpace(dialog.Activate) != "" ||
		strings.TrimSpace(dialog.InProgress) != "" ||
		strings.TrimSpace(dialog.Complete) != ""
}

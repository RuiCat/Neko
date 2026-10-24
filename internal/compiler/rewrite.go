// Package compiler turns a parsed .neko AST into templates and compiled rules.
package compiler

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/ruicat/astrbox/internal/config"
	"github.com/ruicat/astrbox/internal/neko"
)

// BareFieldMap maps unqualified fields to their real group path. It mirrors the
// audit-driven table in the Python compiler.
var BareFieldMap = map[string]string{
	"适应": "身体.适应", "行为总数": "行为.互动次数", "外翻次数": "器官状态.外翻层级",
	"深度超限": "身体.深度超限", "力度超限": "身体.力度超限", "速度超限": "身体.速度超限",
	"跳跃层级": "身体.跳跃层级", "重复超限": "身体.重复超限",
	"受精后时间": "器官状态.生殖.受精后时间", "孕周": "器官状态.生殖.孕周",
	"主人行为": "行为.主人行为", "主人同场景": "行为.主人同场景", "傲娇倾向": "行为.傲娇倾向",
	"撒娇倾向": "行为.撒娇倾向", "服从倾向": "行为.服从倾向", "首次访问": "行为.首次访问",
	"安全感": "关系.安全感", "撒娇欲": "关系.撒娇欲", "好感度": "关系.好感度",
	"亲密": "关系.亲密", "信赖": "关系.信赖",
	"当前场景": "位置.场景", "可达": "位置.可达", "目标": "位置.目标",
	"好奇心": "探索.好奇心", "探索欲": "探索.好奇心",
	"疼痛": "状态.疼痛", "安全等级": "状态.安全等级",
	"伤害": "伤害.出血", "出血": "伤害.出血",
	"行为类型": "性行为.行为类型", "EL": "器官状态.外翻层级", "当前EL": "器官状态.外翻层级",
	"性行为阶段": "性行为.阶段", "性行为进行中": "性行为.进行中",
	"当前位置": "肉棒追踪.位置", "深度": "肉棒追踪.深度", "硬度": "肉棒追踪.硬度",
	"抽动频率": "肉棒追踪.抽动频率", "携带": "携带", "自称": "自称",
	"对主人称呼": "对主人称呼", "恐惧": "器官状态.R18G伤害.恐惧程度",
	"天气": "世界.环境.天气", "时间": "世界.状态.阶段",
	"体内段": "身体.体内段", "当前接触面": "器官状态.外翻过程.当前接触面",
	"持续轮次": "器官状态.外翻过程.持续轮次", "指定输卵管": "器官状态.外翻过程.指定输卵管",
	"停止分钟": "器官状态.外翻过程.停止分钟", "宫颈管径": "器官状态.基础生理.宫颈管径.value",
	"主人离开": "行为.主人离开", "R18互动": "性行为.进行中",
}

var (
	typeSubjectPattern = regexp.MustCompile(`^(\$\d*)\[([^\]]+)\]\.(.*)$`)
	typeOnlyPattern    = regexp.MustCompile(`^(\$\d*)\[([^\]]+)\]$`)
	multiRefPattern    = regexp.MustCompile(`^\$\d+\.`)
	typeDotPattern     = regexp.MustCompile(`\$\[[^\]]+\]\.`)
	bareFieldPattern   = regexp.MustCompile(`^([\p{L}\p{N}_]+(?:\.[\p{L}\p{N}_]+)*)\s*(>=|<=|!=|==|=|>|<|in|contains)\s*(.*)$`)
	rightFieldPattern  = regexp.MustCompile(`^[\p{Han}][\p{Han}\p{L}\p{N}_]*$`)
	pathRefPattern     = regexp.MustCompile(`^[\p{L}\p{N}_]+(?:\.[\p{L}\p{N}_]+)+$`)
	typedRefPattern    = regexp.MustCompile(`\$\[[^\]]+\]\.`)
	typedBarePattern   = regexp.MustCompile(`\$\[[^\]]+\]$`)

	statePrefixPattern = regexp.MustCompile(`^(?:\$\d*\.|\$\[[^\]]+\]\.)`)
)

// SplitLogical splits on top-level ∨ / ∧ and returns parts with their leading
// separator (empty string for the first).
func SplitLogical(text string) []struct {
	Part string
	Sep  string
} {
	parts := []struct {
		Part string
		Sep  string
	}{}
	depth, quote := 0, rune(0)
	var current []rune
	lastSep := ""
	for _, ch := range text {
		switch {
		case quote != 0:
			current = append(current, ch)
			if ch == quote {
				quote = 0
			}
		case ch == '"' || ch == '\'':
			quote = ch
			current = append(current, ch)
		case ch == '(':
			depth++
			current = append(current, ch)
		case ch == ')':
			if depth > 0 {
				depth--
			}
			current = append(current, ch)
		case depth == 0 && (ch == '∨' || ch == '∧'):
			parts = append(parts, struct {
				Part string
				Sep  string
			}{strings.TrimSpace(string(current)), lastSep})
			current = nil
			lastSep = string(ch)
		default:
			current = append(current, ch)
		}
	}
	parts = append(parts, struct {
		Part string
		Sep  string
	}{strings.TrimSpace(string(current)), lastSep})
	return parts
}

// RewriteCondition rewrites $-prefixed subjects to {e} and qualifies bare fields.
func RewriteCondition(cond string, subjects []neko.Subject) (string, []string) {
	extraWhen := []string{}
	if strings.TrimSpace(cond) == "" {
		return cond, extraWhen
	}
	text := strings.TrimSpace(cond)

	var primary *neko.Subject
	multi := false
	for i := range subjects {
		switch subjects[i].Kind {
		case "primary":
			primary = &subjects[i]
		case "multi":
			multi = true
		}
	}

	if parts := SplitLogical(text); len(parts) > 1 {
		var rewritten []string
		var separators []string
		for _, part := range parts {
			if strings.TrimSpace(part.Part) == "" {
				continue
			}
			value, extra := RewriteCondition(part.Part, subjects)
			rewritten = append(rewritten, value)
			separators = append(separators, part.Sep)
			extraWhen = append(extraWhen, extra...)
		}
		if len(rewritten) > 0 {
			result := rewritten[0]
			for i := 1; i < len(rewritten) && i < len(separators); i++ {
				result += " " + separators[i] + " " + rewritten[i]
			}
			return result, extraWhen
		}
	}

	if match := typeSubjectPattern.FindStringSubmatch(text); match != nil && (match[1] == "$" || match[1] == "") {
		typeCond, rest := match[2], match[3]
		if strings.Contains(typeCond, ".") && strings.Contains(typeCond, "=") {
			attr := typeCond[strings.Index(typeCond, ".")+1:]
			extraWhen = append(extraWhen, "{e}."+attr)
		}
		text = "{e}." + rest
		if !multi {
			text = typeDotPattern.ReplaceAllString(text, "{e}.")
			text = strings.ReplaceAll(text, "$.", "{e}.")
		}
		return text, extraWhen
	}

	if typeOnlyPattern.MatchString(text) {
		return "{e}", extraWhen
	}
	if strings.HasPrefix(text, "$.") {
		return strings.Replace(text, "$.", "{e}.", 1), extraWhen
	}
	if text == "$" {
		return "{e}", extraWhen
	}
	if multiRefPattern.MatchString(text) {
		return text, extraWhen
	}
	if strings.HasPrefix(text, "{e}.") {
		if !multi {
			text = typeDotPattern.ReplaceAllString(text, "{e}.")
			text = strings.ReplaceAll(text, "$.", "{e}.")
		}
		return text, extraWhen
	}

	first := strings.TrimSpace(strings.SplitN(text, ".", 2)[0])
	if config.Current().IsGlobalEntity(first) || (primary == nil && !multi) {
		if primary != nil && !multi {
			text = typeDotPattern.ReplaceAllString(text, "{e}.")
			text = strings.ReplaceAll(text, "$.", "{e}.")
		}
		return text, extraWhen
	}

	if match := bareFieldPattern.FindStringSubmatch(text); match != nil {
		field, op, value := match[1], match[2], match[3]
		if op == ">" || op == ">=" || op == "<" || op == "<=" {
			raw := strings.TrimSpace(value)
			if rightFieldPattern.MatchString(raw) && !strings.HasPrefix(raw, ".") && !isDigitStart(raw) {
				if mapped, ok := BareFieldMap[raw]; ok {
					value = "{e}." + config.Current().NormalizePath(mapped)
				} else {
					value = "{e}.身体." + raw
				}
			}
		}
		if strings.Contains(field, ".") {
			return fmt.Sprintf("{e}.%s %s %s", field, op, value), extraWhen
		}
		if mapped, ok := BareFieldMap[field]; ok {
			return fmt.Sprintf("{e}.%s %s %s", config.Current().NormalizePath(mapped), op, value), extraWhen
		}
		return fmt.Sprintf("{e}.%s.%s %s %s", config.Current().DefaultStateSubgroup, field, op, value), extraWhen
	}

	if strings.Contains(text, "(") {
		return RewritePrimaryRefs(text), extraWhen
	}
	return text, extraWhen
}

func isDigitStart(value string) bool {
	if value == "" {
		return false
	}
	ch := value[0]
	return ch >= '0' && ch <= '9'
}

// RewritePrimaryRefs replaces single-subject references with {e}.
func RewritePrimaryRefs(text string) string {
	text = typeDotPattern.ReplaceAllString(text, "{e}.")
	text = strings.ReplaceAll(text, "$.", "{e}.")
	return replaceBareDollar(text)
}

// RewriteStateWhen the 状态: line into a concrete condition.
func RewriteStateWhen(stateWhen *string) *string {
	if stateWhen == nil || strings.TrimSpace(*stateWhen) == "" {
		return nil
	}
	text := strings.TrimSpace(*stateWhen)
	if match := stateLineRegex().FindStringSubmatch(text); match != nil {
		group := match[1]
		if group == "" {
			group = config.Current().DefaultStateSubgroup
		}
		result := fmt.Sprintf("{e}.%s.%s = %s", config.Current().StateGroup, group, strings.TrimSpace(match[2]))
		return &result
	}
	text = statePrefixPattern.ReplaceAllString(text, "{e}.")
	if strings.HasPrefix(text, "$.") {
		text = "{e}." + text[2:]
	}
	return &text
}

// replaceBareDollar replaces a standalone "$" that is not part of "$1"/"$2".
func replaceBareDollar(text string) string {
	runes := []rune(text)
	var out []rune
	for i, ch := range runes {
		if ch != '$' {
			out = append(out, ch)
			continue
		}
		prevDigit := i > 0 && runes[i-1] >= '0' && runes[i-1] <= '9'
		nextDigit := i+1 < len(runes) && runes[i+1] >= '0' && runes[i+1] <= '9'
		if prevDigit || nextDigit {
			out = append(out, ch)
			continue
		}
		out = append(out, []rune("{e}")...)
	}
	return string(out)
}

// stateLineRegex builds the `状态: ...` condition matcher from the active config.
func stateLineRegex() *regexp.Regexp {
	group := regexp.QuoteMeta(config.Current().StateGroup)
	return regexp.MustCompile(`^(?:\$\d*\.|\$\[[^\]]+\]\.)?` + group + `\s*(?:\.([\p{L}\p{N}_]+))?\s*=\s*(.+)$`)
}

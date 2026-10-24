// Package safety ports the optional R18G word-list scan from the Python plugin.
// It is disabled by default (config safety_enabled=false), matching the legacy
// behaviour; when enabled it rejects scripts that contain flagged words.
package safety

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/ruicat/astrbox/internal/config"
	"github.com/ruicat/astrbox/internal/neko"
)

// Violation is one flagged block.
type Violation struct {
	Block string   `json:"block"`
	Words []string `json:"words"`
	Type  string   `json:"type"`
}

// Words is the direct-match list (ported verbatim from legacy safety.py).
var Words = []string{
	"性交", "做爱", "口交", "阴道", "阴茎", "肉棒", "阴蒂", "子宫",
	"高潮", "射精", "精液", "自慰", "性行为", "乳头", "乳交", "肛交",
	"插入", "抽插", "淫", "婬", "情欲", "性欲", "性感带", "G点",
	"阴茎插入", "射在里面", "内射", "体液交换",
	"快感", "兴奋度", "湿润度", "收缩状态", "成瘾度",
	"外翻", "翻出", "子宫颈", "卵巢", "受精", "怀孕",
	"支配", "调教", "束缚", "虐", "痛感",
}

var filePatterns = []*regexp.Regexp{
	regexp.MustCompile(`eversion`), regexp.MustCompile(`sexual`),
	regexp.MustCompile(`sex`), regexp.MustCompile(`penetrat`),
	regexp.MustCompile(`intercourse`), regexp.MustCompile(`nipple`),
	regexp.MustCompile(`clitor`), regexp.MustCompile(`vagina`),
	regexp.MustCompile(`penis`), regexp.MustCompile(`orgasm`),
}

// Enabled reports whether the scan is active (config safety_enabled).
func Enabled() bool { return config.Current().SafetyEnabled }

// CheckText returns the flagged words in text; context is the script name.
func CheckText(text, context string) []string {
	if !Enabled() || text == "" {
		return nil
	}
	lower := strings.ToLower(text)
	var hits []string
	for _, word := range Words {
		if strings.Contains(lower, strings.ToLower(word)) {
			hits = append(hits, word)
		}
	}
	if context != "" {
		lowerContext := strings.ToLower(context)
		for _, pattern := range filePatterns {
			if pattern.MatchString(lowerContext) {
				hits = append(hits, "文件名命中: "+context)
				break
			}
		}
	}
	return hits
}

// CheckAST scans rules, events, functions and data type names.
func CheckAST(ast *neko.ScriptAST) []Violation {
	if !Enabled() {
		return nil
	}
	var violations []Violation
	for _, rule := range ast.RuleDefs {
		var texts []string
		texts = append(texts, rule.Conditions...)
		for _, action := range rule.Actions {
			texts = append(texts, action.Target+"="+valueText(action.Value))
		}
		for _, branch := range rule.Branches {
			if branch.Condition != nil {
				texts = append(texts, *branch.Condition)
			}
			for _, op := range branch.Ops {
				texts = append(texts, op.Target+"="+valueText(op.Value))
			}
		}
		if hits := CheckText(strings.Join(texts, " "), ""); len(hits) > 0 {
			violations = append(violations, Violation{Block: "@规则 " + rule.HeaderRaw, Words: hits, Type: "rule"})
		}
	}
	for _, event := range ast.EventDefs {
		if hits := CheckText(event.Name+" "+event.Condition+" "+event.Handler, ""); len(hits) > 0 {
			violations = append(violations, Violation{Block: "@事件 " + event.Name, Words: hits, Type: "event"})
		}
	}
	for _, function := range ast.FuncDefs {
		if hits := CheckText(function.Name+" "+function.Body, ""); len(hits) > 0 {
			violations = append(violations, Violation{Block: "@函数 " + function.Name, Words: hits, Type: "func"})
		}
	}
	for _, data := range ast.DataDefs {
		if hits := CheckText(data.TypeName, ""); len(hits) > 0 {
			violations = append(violations, Violation{Block: "@Data " + data.TypeName, Words: hits, Type: "data"})
		}
	}
	return violations
}

// CheckScriptText scans the raw script text and its name.
func CheckScriptText(text, name string) []Violation {
	if !Enabled() {
		return nil
	}
	if hits := CheckText(text, name); len(hits) > 0 {
		block := name
		if block == "" {
			block = "(全文)"
		}
		return []Violation{{Block: block, Words: hits, Type: "text"}}
	}
	return nil
}

func valueText(value any) string {
	if value == nil {
		return ""
	}
	text := strings.TrimSpace(fmt.Sprintf("%v", value))
	return strings.TrimSpace(strings.ReplaceAll(text, "\n", " "))
}

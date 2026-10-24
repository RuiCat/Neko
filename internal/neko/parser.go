// parser.go converts scanned .neko blocks into the frozen ScriptAST, keeping the
// per-block fail-soft behaviour of the Python reference (errors land in AST.Errors).
package neko

import (
	"fmt"
	"regexp"
	"strings"
)

var (
	funcHeaderRe = regexp.MustCompile(`^([\p{L}\p{N}_]+)\s*\((.*?)\)\s*(?:->\s*([\p{L}\p{N}_]+))?$`)
	metaLineRe   = regexp.MustCompile(`^#([\p{L}\p{N}_]+)\s*:\s*(.*)$`)
)

// Parse parses .neko source text into a ScriptAST. Parse failures are collected
// in AST.Errors and never abort the remaining blocks.
func Parse(text string) *ScriptAST {
	ast := &ScriptAST{
		DataDefs:  []DataDef{},
		RuleDefs:  []RuleDef{},
		EventDefs: []EventDef{},
		FuncDefs:  []FuncDef{},
		Metadata:  []MetaDef{},
		Errors:    []error{},
		Warnings:  []Warning{},
	}
	for _, block := range ScanBlocks(text) {
		parseBlock(ast, block)
	}
	return ast
}

// parseBlock dispatches one scanned block; a panic is converted into a parse
// error so one malformed block cannot take the whole script down.
func parseBlock(ast *ScriptAST, block Block) {
	pos := block.Pos()
	defer func() {
		if r := recover(); r != nil {
			ast.Errors = append(ast.Errors, &ParseError{
				Message: fmt.Sprintf("解析异常: %v", r),
				Pos:     pos,
			})
		}
	}()

	switch block.Kind {
	case "at_desc":
		return

	case "at_data", "at_data_legacy":
		name := strings.TrimSpace(block.Name)
		data := map[string]any{}
		if block.Body != "" {
			decoded, err := decodeJSONAny(block.Body)
			if err != nil {
				ast.Errors = append(ast.Errors, &ParseError{
					Message: "JSON 解析失败: " + err.Error(),
					Pos:     pos,
					Hint:    "检查 JSON 语法（引号/逗号/括号）",
				})
				return
			}
			obj, ok := decoded.(map[string]any)
			if !ok {
				ast.Errors = append(ast.Errors, &ParseError{
					Message: "JSON 解析失败: 顶层不是对象",
					Pos:     pos,
					Hint:    "检查 JSON 语法（引号/逗号/括号）",
				})
				return
			}
			data = obj
		}
		ast.DataDefs = append(ast.DataDefs, DataDef{
			TypeName: name,
			Data:     data,
			IsEntity: block.Kind == "at_data",
			Pos:      pos,
		})

	case "at_multicond":
		for _, rule := range parseMulticond(block.Body, block.Name, pos) {
			rule.HeaderRaw = block.Name
			ast.RuleDefs = append(ast.RuleDefs, rule)
		}

	case "at_rule_range":
		for _, rule := range parseRuleRange(block.Body, block.Name, pos) {
			rule.HeaderRaw = block.Name
			ast.RuleDefs = append(ast.RuleDefs, rule)
		}

	case "at_rule":
		subjects := parseRuleHeader(block.Name)
		if looksMulticond(block.Body) {
			for _, rule := range parseMulticond(block.Body, block.Name, pos) {
				rule.HeaderRaw = block.Name
				ast.RuleDefs = append(ast.RuleDefs, rule)
			}
			return
		}
		rules := parseRuleBody(block.Body, subjects, pos)
		for i := range rules {
			rules[i].HeaderRaw = block.Name
			ast.RuleDefs = append(ast.RuleDefs, rules[i])
		}

	case "at_event":
		ast.EventDefs = append(ast.EventDefs, parseEvent(block.Name, block.Body, pos))

	case "at_func":
		funcDef, err := parseFunc(block.Name, block.Body, pos)
		if err != nil {
			ast.Errors = append(ast.Errors, err)
			return
		}
		ast.FuncDefs = append(ast.FuncDefs, funcDef)

	case "hash_rule":
		ast.RuleDefs = append(ast.RuleDefs, parseHashRule(block.Name, pos))

	case "meta":
		if m := metaLineRe.FindStringSubmatch(block.Name); m != nil {
			ast.Metadata = append(ast.Metadata, MetaDef{Key: m[1], Value: m[2], Pos: pos})
		}
	}
}

// parseEvent parses an @事件 block body into an EventDef.
func parseEvent(name, body string, pos Pos) EventDef {
	body = strings.TrimSpace(body)
	if strings.HasPrefix(body, "{") && strings.HasSuffix(body, "}") {
		body = runeSlice(body, 1, runeCount(body)-1)
	}
	cond := ""
	handler := ""
	actions := []any{}
	for _, raw := range strings.Split(body, "\n") {
		bl := strings.TrimSpace(raw)
		if strings.HasPrefix(bl, "条件:") || strings.HasPrefix(bl, "condition:") {
			cond = strings.TrimSpace(strings.SplitN(bl, ":", 2)[1])
		} else if strings.HasPrefix(bl, "处理器:") || strings.HasPrefix(bl, "handler:") || strings.HasPrefix(bl, "处理:") {
			handler = strings.TrimSpace(strings.SplitN(bl, ":", 2)[1])
		} else if strings.Contains(bl, "=") && !strings.Contains(runeSlice(bl, 0, 2), "动作") {
			actions = append(actions, bl)
		}
	}
	return EventDef{Name: name, Condition: cond, Handler: handler, Actions: actions, Pos: pos}
}

// parseFunc parses an @func header and body into a FuncDef.
func parseFunc(header, body string, pos Pos) (FuncDef, error) {
	m := funcHeaderRe.FindStringSubmatch(header)
	if m == nil {
		return FuncDef{}, &ParseError{
			Message: "@func 头格式错误: " + header,
			Pos:     pos,
			Hint:    "@func 函数名(参数:类型) -> 返回类型 { 函数体 }",
		}
	}
	params := []Param{}
	if strings.TrimSpace(m[2]) != "" {
		for _, rawParam := range strings.Split(m[2], ",") {
			p := strings.TrimSpace(rawParam)
			if p == "" {
				continue
			}
			if idx := strings.Index(p, ":"); idx >= 0 {
				params = append(params, Param{Name: strings.TrimSpace(p[:idx]), Type: strings.TrimSpace(p[idx+1:])})
			} else {
				params = append(params, Param{Name: p, Type: "any"})
			}
		}
	}
	trimmedBody := strings.TrimSpace(body)
	if strings.HasPrefix(trimmedBody, "{") && strings.HasSuffix(trimmedBody, "}") {
		trimmedBody = runeSlice(trimmedBody, 1, runeCount(trimmedBody)-1)
	}
	return FuncDef{Name: m[1], Params: params, ReturnType: m[3], Body: trimmedBody, Pos: pos}, nil
}

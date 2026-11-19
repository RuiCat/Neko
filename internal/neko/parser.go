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

	case "at_data", "at_instance":
		name := strings.TrimSpace(block.Name)
		parents := []string(nil)
		if block.Kind == "at_instance" {
			parsedName, parsedParents, err := ParseInstanceHeader(name)
			if err != nil {
				ast.Errors = append(ast.Errors, &ParseError{
					Message: err.Error(),
					Pos:     pos,
					Hint:    "@实例 分类:名称 [类型1, 类型2] { JSON }",
				})
				return
			}
			name, parents = parsedName, parsedParents
		} else if strings.ContainsRune(name, '[') {
			ast.Errors = append(ast.Errors, &ParseError{
				Message: "@定义 不接受方括号列表",
				Pos:     pos,
				Hint:    "纯模板用 @定义 名称 { JSON }；需要组合类型或声明实例请用 @实体 / @实例",
			})
			return
		}
		data, ok := decodeEntityBody(block.Body, ast, pos)
		if !ok {
			return
		}
		// @实例 是「已经算好的值」：注解只在模板（@定义/@实体）里展开。
		// 若在 @实例 里写了 `"体力[int,min:0,max:100]": 75`，键会被原样写成字面字段名，
		// 真正的 `体力` 反而没有赋值，且不报错——这里直接拒绝，避免静默写坏存档。
		if block.Kind == "at_instance" {
			if path, found := findAnnotatedKey(data, ""); found {
				ast.Errors = append(ast.Errors, &ParseError{
					Message: fmt.Sprintf("@实例 里不能使用字段注解: %s", path),
					Pos:     pos,
					Hint:    "注解只用于模板（@定义 / @实体），会在编译期展开为字段与 __annotations__；@实例 里请直接写已确定的值",
				})
				return
			}
		}
		ast.DataDefs = append(ast.DataDefs, DataDef{
			TypeName: name,
			Data:     data,
			Parents:  parents,
			IsEntity: block.Kind == "at_data",
			Pos:      pos,
		})

	case "at_unknown":
		// scanJsonBlock 只剥掉了 "@"，这里补回来再取关键字本身。
		fields := strings.Fields(block.Name)
		head := "@"
		if len(fields) > 0 {
			head += fields[0]
		}
		message := fmt.Sprintf("未知的顶层关键字 %q", head)
		hint := "可用的顶层块：@定义 / @实体 / @实例 / @规则 / @事件 / @函数 / @描述"
		switch head {
		case "@Data":
			message = "@Data 已移除（不再作为 @定义 的别名）"
			hint = "定义模板请用 @定义（或 @实体 组合模板）；建立数据实例请用 @实例"
		case "@data":
			message = "@data 已移除（关键字改为 @实例）"
			hint = "建立数据实例请用 @实例；定义模板请用 @定义"
		}
		ast.Errors = append(ast.Errors, &ParseError{
			Message: message,
			Pos:     pos,
			Hint:    hint,
		})

	case "at_entity":
		name, parents, err := ParseEntityHeader(block.Name)
		if err != nil {
			ast.Errors = append(ast.Errors, &ParseError{
				Message: err.Error(),
				Pos:     pos,
				Hint:    "@实体 名称 [父1, 父2] { JSON }",
			})
			return
		}
		data, ok := decodeEntityBody(block.Body, ast, pos)
		if !ok {
			return
		}
		ast.DataDefs = append(ast.DataDefs, DataDef{
			TypeName: name,
			Data:     data,
			Parents:  parents,
			IsEntity: true,
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

// decodeEntityBody decodes a template body into a JSON object, reporting a
// parse error and returning false when the body is malformed.
// annotatedKeyRe matches a template-style annotated field name such as
// `体力[int,@事件,min:0,max:100]`. @实例 blocks must not use it.
var annotatedKeyRe = regexp.MustCompile(`^[^\[\]]+\[.*\]$`)

// findAnnotatedKey walks instance data and returns the dotted path of the first
// annotated key, if any.
func findAnnotatedKey(data map[string]any, prefix string) (string, bool) {
	for key, value := range data {
		path := key
		if prefix != "" {
			path = prefix + "." + key
		}
		if annotatedKeyRe.MatchString(strings.TrimSpace(key)) {
			return path, true
		}
		if child, ok := value.(map[string]any); ok {
			if found, ok := findAnnotatedKey(child, path); ok {
				return found, true
			}
		}
	}
	return "", false
}

func decodeEntityBody(body string, ast *ScriptAST, pos Pos) (map[string]any, bool) {
	data := map[string]any{}
	if body == "" {
		return data, true
	}
	decoded, err := decodeJSONAny(body)
	if err != nil {
		ast.Errors = append(ast.Errors, &ParseError{
			Message: "JSON 解析失败: " + err.Error(),
			Pos:     pos,
			Hint:    "检查 JSON 语法（引号/逗号/括号）",
		})
		return nil, false
	}
	obj, ok := decoded.(map[string]any)
	if !ok {
		ast.Errors = append(ast.Errors, &ParseError{
			Message: "JSON 解析失败: 顶层不是对象",
			Pos:     pos,
			Hint:    "检查 JSON 语法（引号/逗号/括号）",
		})
		return nil, false
	}
	return obj, true
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

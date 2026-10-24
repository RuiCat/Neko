// parserRule.go parses rule headers, action lines, branch blocks and rule bodies
// (both the legacy key:value form and the condition { action } form).
package neko

import (
	"github.com/ruicat/astrbox/internal/config"
	"regexp"
	"strconv"
	"strings"
	"unicode"
)

var (
	subjectVarRe    = regexp.MustCompile(`^(\$\d*)\s*(?:\[(.*?)\])?$`)
	subjectGlobalRe = regexp.MustCompile(`^([\p{L}\p{N}_:]+)\s*(?:\[(.*?)\])?$`)
	actionOpRe      = regexp.MustCompile(`^\s*(\S+?)\s*(>=|<=|!=|==|\+=|-=|\*=|/=|=|>|<|\bin\b|\bcontains\b)\s*(.+)$`)
	subjectPathRe   = regexp.MustCompile(`^\$(\d+)(?:\.(.*))?$`)
	ruleKeyRe       = regexp.MustCompile(`^([\p{L}\p{N}_]+)\s*:\s*(.*)$`)
	branchArrowRe   = regexp.MustCompile(`^(.+?)\s*→\s*(.+)$`)
	branchCondRe    = regexp.MustCompile(`^[\p{L}\p{N}_$.\s<>=!]+$`)
)

// parseSubject parses one rule subject such as $, $1, $[NPC], $1[NPC.性别=女] or 场景.
func parseSubject(text string) Subject {
	text = strings.TrimSpace(text)
	if text == "" {
		return Subject{Kind: "primary", Var: "$"}
	}
	if strings.HasPrefix(text, "$") {
		if m := subjectVarRe.FindStringSubmatch(text); m != nil {
			varName := m[1]
			if varName == "" {
				varName = "$"
			}
			kind := "primary"
			if varName != "$" {
				kind = "multi"
			}
			qual := m[2]
			if qual != "" {
				if idx := strings.Index(qual, "."); idx >= 0 {
					return Subject{
						Var:       varName,
						Kind:      kind,
						TypeName:  strings.TrimSpace(qual[:idx]),
						Condition: strings.TrimSpace(qual[idx+1:]),
					}
				}
				return Subject{Var: varName, Kind: kind, TypeName: strings.TrimSpace(qual)}
			}
			return Subject{Var: varName, Kind: kind}
		}
	}
	if m := subjectGlobalRe.FindStringSubmatch(text); m != nil {
		subject := Subject{Var: m[1], Kind: "global", EntityID: m[1]}
		if m[2] != "" {
			subject.TypeName = strings.TrimSpace(m[2])
		}
		return subject
	}
	return Subject{Var: text, Kind: "global", EntityID: text}
}

// parseRuleHeader parses a `@规则[$1[NPC]，场景]` header into its subjects.
func parseRuleHeader(header string) []Subject {
	header = strings.TrimSpace(header)
	if !strings.HasPrefix(header, "[") {
		return []Subject{{Kind: "primary", Var: "$"}}
	}
	inner := runeSlice(header, 1, runeCount(header)-1)
	parts := []string{}
	for _, raw := range strings.Split(inner, "，") {
		p := strings.TrimSpace(raw)
		if p != "" {
			parts = append(parts, p)
		}
	}
	if len(parts) == 0 {
		return []Subject{{Kind: "primary", Var: "$"}}
	}
	subjects := make([]Subject, 0, len(parts))
	for _, p := range parts {
		subjects = append(subjects, parseSubject(p))
	}
	return subjects
}

// parseActionLine parses one `target op value` action line into an ActionOp.
func parseActionLine(line string, pos Pos) *ActionOp {
	line = strings.TrimSpace(line)
	if line == "" {
		return nil
	}
	m := actionOpRe.FindStringSubmatch(line)
	if m == nil {
		return nil
	}
	target := strings.TrimSpace(m[1])
	op := strings.TrimSpace(m[2])
	valueRaw := strings.TrimSpace(m[3])

	subject := 0
	entity := ""
	if m2 := subjectPathRe.FindStringSubmatch(target); m2 != nil {
		subject, _ = strconv.Atoi(m2[1])
		target = m2[2]
	} else if strings.HasPrefix(target, "{e}.") {
		target = runeSliceFrom(target, 4)
	} else if strings.HasPrefix(target, "$.") {
		target = runeSliceFrom(target, 2)
	} else if strings.Contains(target, ".") && !strings.HasPrefix(target, "{e}") {
		first := strings.SplitN(target, ".", 2)[0]
		if !strings.HasPrefix(first, "$") && config.Current().IsGlobalEntity(first) {
			entity = first
			target = runeSliceFrom(target, runeCount(first)+1)
		}
	}

	opType := "set"
	switch op {
	case "=":
		opType = "set"
	case "+=", "-=":
		if op == "-=" {
			valueRaw = "-" + valueRaw
		}
		if isContainerValue(parseScalar(valueRaw)) || entity != "" {
			opType = "append"
		} else {
			opType = "add"
		}
	case "in":
		opType = "set"
	default:
		opType = "set"
	}
	return &ActionOp{
		Op:      opType,
		Target:  target,
		Value:   parseScalar(valueRaw),
		Subject: subject,
		Entity:  entity,
		Pos:     pos,
	}
}

// parseConditionLine returns a condition expression with a trailing // comment removed.
func parseConditionLine(line string) string {
	line = strings.TrimSpace(line)
	if idx := strings.Index(line, "//"); idx >= 0 {
		line = strings.TrimSpace(line[:idx])
	}
	return line
}

// parseActionLines parses every action line of a block body, skipping comments
// and optional 动作:/action: prefixes.
func parseActionLines(text string, pos Pos) []ActionOp {
	actions := []ActionOp{}
	for _, raw := range strings.Split(text, "\n") {
		s := strings.TrimSpace(raw)
		if s == "" || strings.HasPrefix(s, "//") {
			continue
		}
		if strings.HasPrefix(s, "动作:") || strings.HasPrefix(s, "action:") {
			s = strings.TrimSpace(strings.SplitN(s, ":", 2)[1])
		}
		if op := parseActionLine(s, pos); op != nil {
			actions = append(actions, *op)
		}
	}
	return actions
}

// parseBranchBlock parses a 分支: { ... } block body into BranchDef entries.
func parseBranchBlock(body string, pos Pos) []BranchDef {
	branches := []BranchDef{}
	for _, raw := range strings.Split(body, "\n") {
		line := strings.TrimSpace(raw)
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "默认") || strings.HasPrefix(line, "else") {
			branches = append(branches, BranchDef{Pos: pos})
			continue
		}
		if m := branchArrowRe.FindStringSubmatch(line); m != nil {
			cond := strings.TrimSpace(m[1])
			target := strings.TrimSpace(m[2])
			var state map[string]any
			if m2 := branchStateRegex().FindStringSubmatch(target); m2 != nil {
				group := m2[1]
				if group == "" {
					group = config.Current().DefaultStateSubgroup
				}
				state = map[string]any{group: parseScalar(m2[2])}
			}
			branches = append(branches, BranchDef{Condition: &cond, State: state, Pos: pos})
			continue
		}
		if branchCondRe.MatchString(line) {
			cond := line
			branches = append(branches, BranchDef{Condition: &cond, Pos: pos})
		}
	}
	return branches
}

// findActionBrace locates the action block '{' that starts after the condition,
// skipping string literals, `in {集合}` sets and leading `{e}` placeholders.
func findActionBrace(text string, start int) int {
	rs := []rune(text)
	n := len(rs)
	inStr := rune(0)
	j := start
	for j < n {
		ch := rs[j]
		if inStr != 0 {
			if ch == inStr {
				inStr = 0
			}
			j++
			continue
		}
		if ch == '"' || ch == '\'' {
			inStr = ch
			j++
			continue
		}
		if ch == '{' {
			before := strings.TrimSpace(string(rs[start:j]))
			if before == "" || strings.HasSuffix(before, " in") || before == "in" {
				depth := 1
				k := j + 1
				for k < n && depth > 0 {
					if rs[k] == '{' {
						depth++
					} else if rs[k] == '}' {
						depth--
					}
					k++
				}
				j = k
				continue
			}
			return j
		}
		j++
	}
	return -1
}

// parseRuleBody parses a rule body into one or more RuleDef entries; multiple
// entries appear when the body holds several `条件 { 动作 }` branches.
func parseRuleBody(body string, subjects []Subject, pos Pos) []RuleDef {
	rule := RuleDef{Subjects: subjects, HeaderRaw: "", Pos: pos}
	body = strings.TrimSpace(body)
	if strings.HasPrefix(body, "{") && strings.HasSuffix(body, "}") {
		body = runeSlice(body, 1, runeCount(body)-1)
	}

	ruleKeys := []string{"条件:", "动作:", "状态:", "分支:", "优先级:", "id:", "condition:", "action:"}
	rest := strings.TrimSpace(body)
	metaLines := []string{}
	for {
		head := firstLineTrim(rest)
		if strings.HasPrefix(head, "优先级:") || strings.HasPrefix(head, "id:") {
			metaLines = append(metaLines, head)
			rest = dropFirstLine(rest)
			continue
		}
		break
	}
	first := ""
	if strings.TrimSpace(rest) != "" {
		first = firstLineTrim(strings.TrimSpace(rest))
	}
	if strings.Contains(first, "{") && !hasAnyPrefix(first, ruleKeys) && strings.Contains(rest, "{") {
		if multi := parseMultiIf(body, subjects, pos); multi != nil {
			return multi
		}
		for _, ml := range metaLines {
			if strings.HasPrefix(ml, "优先级:") {
				if v, err := strconv.Atoi(strings.TrimSpace(strings.SplitN(ml, ":", 2)[1])); err == nil {
					n := v
					rule.Priority = &n
				}
			} else if strings.HasPrefix(ml, "id:") {
				rule.RuleID = strings.TrimSpace(strings.SplitN(ml, ":", 2)[1])
			}
		}
		openPos := findActionBrace(rest, 0)
		if openPos < 0 {
			openPos = findRune(rest, '{')
		}
		var condExpr string
		if openPos < 0 {
			condExpr = strings.TrimSpace(runeSlice(rest, 0, runeCount(rest)-1))
			openPos = -1
		} else {
			condExpr = strings.TrimSpace(runeSlice(rest, 0, openPos))
		}
		depth := 1
		i := openPos + 1
		restLen := runeCount(rest)
		if i < 0 {
			i = 0
		}
		for i < restLen && depth > 0 {
			ch := runeAt(rest, i)
			if ch == '{' {
				depth++
			} else if ch == '}' {
				depth--
			}
			i++
		}
		actionText := runeSlice(rest, openPos+1, i-1)
		rule.Actions = append(rule.Actions, parseActionLines(actionText, pos)...)
		rule.Conditions = append(rule.Conditions, condExpr)
		return []RuleDef{rule}
	}

	lines := strings.Split(body, "\n")
	for i := range lines {
		lines[i] = strings.TrimRightFunc(lines[i], unicode.IsSpace)
	}
	currentKey := ""
	currentValueLines := []string{}
	branchDepth := 0

	flush := func(key string, valueLines []string) {
		text := strings.TrimSpace(strings.Join(valueLines, "\n"))
		if text == "" {
			return
		}
		switch key {
		case "id":
			rule.RuleID = strings.TrimSpace(text)
		case "优先级":
			if v, err := strconv.Atoi(strings.TrimSpace(text)); err == nil {
				n := v
				rule.Priority = &n
			}
		case "状态":
			value := text
			rule.StateWhen = &value
		case "条件":
			rule.Conditions = append(rule.Conditions, text)
		case "动作":
			if op := parseActionLine(text, pos); op != nil {
				rule.Actions = append(rule.Actions, *op)
			}
		case "分支":
			rule.Branches = append(rule.Branches, parseBranchBlock(text, pos)...)
		}
	}

	for _, line := range lines {
		stripped := strings.TrimSpace(line)
		if stripped == "" {
			continue
		}
		if branchDepth > 0 {
			branchDepth += strings.Count(stripped, "{") - strings.Count(stripped, "}")
			currentValueLines = append(currentValueLines, line)
			if branchDepth == 0 && currentKey == "分支" {
				flush("分支", currentValueLines)
				currentKey = ""
				currentValueLines = nil
			}
			continue
		}
		if strings.Contains(stripped, "分支") && strings.Contains(strings.SplitN(stripped, "{", 2)[0], ":") {
			if currentKey != "" && len(currentValueLines) > 0 {
				flush(currentKey, currentValueLines)
			}
			currentKey = "分支"
			braceRest := ""
			if idx := strings.Index(stripped, "{"); idx >= 0 {
				braceRest = stripped[idx+1:]
			}
			currentValueLines = nil
			if strings.Contains(stripped, "{") {
				branchDepth = 1
				if strings.TrimSpace(braceRest) != "" {
					currentValueLines = append(currentValueLines, strings.TrimSpace(braceRest))
				}
				if strings.Contains(braceRest, "}") {
					inner := strings.TrimSpace(strings.SplitN(braceRest, "}", 2)[0])
					currentValueLines = []string{inner}
					branchDepth = 0
					flush("分支", currentValueLines)
					currentKey = ""
					currentValueLines = nil
				}
			}
			continue
		}
		if m := ruleKeyRe.FindStringSubmatch(stripped); m != nil {
			if currentKey != "" && len(currentValueLines) > 0 {
				flush(currentKey, currentValueLines)
			}
			currentKey = m[1]
			val := m[2]
			if val != "" {
				currentValueLines = []string{val}
			} else {
				currentValueLines = nil
			}
			continue
		}
		if currentKey != "" {
			currentValueLines = append(currentValueLines, line)
			continue
		}
		if strings.Contains(stripped, "=") || strings.Contains(stripped, ">=") || strings.Contains(stripped, "<=") {
			if op := parseActionLine(stripped, pos); op != nil {
				rule.Actions = append(rule.Actions, *op)
			} else {
				rule.Conditions = append(rule.Conditions, stripped)
			}
		} else if strings.Contains(stripped, "→") {
			rule.Conditions = append(rule.Conditions, stripped)
		}
	}
	if currentKey != "" && len(currentValueLines) > 0 {
		flush(currentKey, currentValueLines)
	}
	return []RuleDef{rule}
}

// parseHashRule parses a `#规则: 条件 → 动作` line into a line-form RuleDef.
func parseHashRule(line string, pos Pos) RuleDef {
	line = strings.TrimSpace(line)
	content := line
	if strings.HasPrefix(line, "#规则") {
		content = strings.TrimSpace(strings.TrimLeft(strings.TrimSpace(runeSliceFrom(line, 3)), ":"))
	}
	rule := RuleDef{
		Subjects: []Subject{{Kind: "primary", Var: "$"}},
		LineForm: &line,
		Pos:      pos,
	}
	if idx := strings.Index(content, "→"); idx >= 0 {
		cond := content[:idx]
		action := content[idx+len("→"):]
		rule.Conditions = append(rule.Conditions, parseConditionLine(cond))
		if op := parseActionLine(action, pos); op != nil {
			rule.Actions = append(rule.Actions, *op)
		}
		return rule
	}
	rule.Conditions = append(rule.Conditions, parseConditionLine(content))
	return rule
}

// hasAnyPrefix reports whether s starts with any of the given prefixes.
func hasAnyPrefix(s string, prefixes []string) bool {
	for _, p := range prefixes {
		if strings.HasPrefix(s, p) {
			return true
		}
	}
	return false
}

// branchStateRegex builds the `状态 = "x"` sugar matcher from the active config.
func branchStateRegex() *regexp.Regexp {
	group := regexp.QuoteMeta(config.Current().StateGroup)
	return regexp.MustCompile(`^` + group + `(?:\.([\p{L}\p{N}_]+))?\s*=\s*(.+)$`)
}

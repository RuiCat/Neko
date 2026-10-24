// parserMulti.go parses the multi-rule block forms: @多规则/@多条件 child rules,
// range-threshold @规则 blocks and blocks holding several `条件 { 动作 }` branches.
package neko

import (
	"regexp"
	"sort"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"
)

var (
	multiSubRe     = regexp.MustCompile(`[\p{L}\p{N}_]+\s*:\s*\{`)
	rangeSubjectRe = regexp.MustCompile(`\[\$\[[^\]]+\]\]`)
	rangeBranchRe  = regexp.MustCompile(`([\p{L}\p{N}_=><+\-.]*?)\s*:\s*\{`)
	rangeCmpRe     = regexp.MustCompile(`^(>=|<=|>|<|=)\s*(.+)$`)
)

// looksMulticond reports whether a rule body uses the `名字: { ... }` multi-rule form.
func looksMulticond(body string) bool {
	content := strings.TrimSpace(body)
	if strings.HasPrefix(content, "{") && strings.HasSuffix(content, "}") {
		content = runeSlice(content, 1, runeCount(content)-1)
	}
	content = strings.TrimSpace(content)
	if content == "" {
		return false
	}
	for {
		fl := firstLineTrim(content)
		if strings.HasPrefix(fl, "优先级:") || strings.HasPrefix(fl, "id:") {
			content = strings.TrimSpace(dropFirstLine(content))
			if content == "" {
				return false
			}
		} else {
			break
		}
	}
	firstLine := firstLineTrim(content)
	if !strings.Contains(firstLine, "{") {
		return false
	}
	beforeBrace := strings.TrimSpace(strings.SplitN(firstLine, "{", 2)[0])
	if !strings.Contains(beforeBrace, ":") {
		return false
	}
	idx := strings.LastIndex(beforeBrace, ":")
	afterColon := strings.TrimSpace(beforeBrace[idx+1:])
	if afterColon != "" {
		return false
	}
	name := strings.TrimSpace(beforeBrace[:idx])
	switch name {
	case "条件", "动作", "状态", "分支", "优先级", "id", "condition", "action":
		return false
	}
	return true
}

// stripOuterBraces removes one leading `{` and one trailing `}` when both are
// present, matching the Python helper's shared pre-step.
func stripOuterBraces(s string) string {
	if strings.HasPrefix(s, "{") && strings.HasSuffix(s, "}") {
		return runeSlice(s, 1, runeCount(s)-1)
	}
	return s
}

// parseMulticond parses a @多规则/@多条件 body into one RuleDef per child rule.
func parseMulticond(body, header string, pos Pos) []RuleDef {
	hdr := strings.TrimSpace(strings.TrimRight(strings.TrimSpace(header), "{"))
	subjects := []Subject{}
	if hdr != "" {
		subjects = parseRuleHeader(hdr)
	}
	content := strings.TrimSpace(stripOuterBraces(strings.TrimSpace(body)))

	content, priority, ruleID := consumeRuleMetaPrefixes(content)

	subInners := []string{}
	rs := []rune(content)
	n := len(rs)
	i := 0
	for i < n {
		sub := string(rs[i:])
		loc := multiSubRe.FindStringSubmatchIndex(sub)
		if loc == nil {
			break
		}
		bracePos := i + byteToRuneIndex(sub, loc[1]-1)
		depth := 1
		j := bracePos + 1
		for j < n && depth > 0 {
			if rs[j] == '{' {
				depth++
			} else if rs[j] == '}' {
				depth--
			}
			j++
		}
		subInners = append(subInners, runeSlice(content, bracePos+1, j-1))
		i = j
	}

	result := []RuleDef{}
	for _, inner := range subInners {
		conds := []string{}
		actions := []ActionOp{}
		for _, raw := range strings.Split(inner, "\n") {
			s := strings.TrimSpace(raw)
			if s == "" || strings.HasPrefix(s, "//") {
				continue
			}
			if strings.HasPrefix(s, "条件:") || strings.HasPrefix(s, "condition:") {
				conds = append(conds, strings.TrimSpace(strings.SplitN(s, ":", 2)[1]))
			} else if strings.HasPrefix(s, "动作:") || strings.HasPrefix(s, "action:") {
				if op := parseActionLine(strings.TrimSpace(strings.SplitN(s, ":", 2)[1]), pos); op != nil {
					actions = append(actions, *op)
				}
			} else if op := parseActionLine(s, pos); op != nil {
				actions = append(actions, *op)
			}
		}
		if len(actions) == 0 && len(conds) == 0 {
			continue
		}
		result = append(result, RuleDef{
			Subjects: subjects, Conditions: conds, Actions: actions,
			Priority: priority, RuleID: ruleID, Pos: pos,
		})
	}
	return result
}

// consumeRuleMetaPrefixes removes the leading 优先级:/id: lines and returns the
// remaining content together with the parsed values.
func consumeRuleMetaPrefixes(content string) (string, *int, string) {
	var priority *int
	ruleID := ""
	for {
		fl := firstLineTrim(content)
		if strings.HasPrefix(fl, "优先级:") {
			if v, err := strconv.Atoi(strings.TrimSpace(strings.SplitN(fl, ":", 2)[1])); err == nil {
				n := v
				priority = &n
			}
			content = strings.TrimSpace(dropFirstLine(content))
			continue
		}
		if strings.HasPrefix(fl, "id:") {
			ruleID = strings.TrimSpace(strings.SplitN(fl, ":", 2)[1])
			content = strings.TrimSpace(dropFirstLine(content))
			continue
		}
		break
	}
	return content, priority, ruleID
}

// valStr renders a range branch value the way the Python reference does: bare
// when it starts like a number, double-quoted otherwise.
func valStr(v string) string {
	v = strings.TrimSpace(v)
	if v != "" {
		first, _ := utf8.DecodeRuneInString(v)
		if unicode.IsDigit(first) || first == '-' || first == '.' {
			return v
		}
	}
	return `"` + v + `"`
}

// isDigitString reports whether every rune of s is a Unicode decimal digit.
func isDigitString(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if !unicode.IsDigit(r) {
			return false
		}
	}
	return true
}

// pythonFloatStr formats a float like Python's str(), keeping the trailing .0.
func pythonFloatStr(f float64) string {
	s := strconv.FormatFloat(f, 'g', -1, 64)
	if !strings.ContainsAny(s, ".eE") {
		s += ".0"
	}
	return s
}

// parseRuleRange parses a threshold rule (`@规则[$[NPC]]:$.状态 { 30:{...} }`) into
// one RuleDef per threshold bucket.
func parseRuleRange(body, header string, pos Pos) []RuleDef {
	hdr := strings.TrimSpace(header)
	subjects := []Subject{}
	if m := rangeSubjectRe.FindString(hdr); m != "" {
		subjects = parseRuleHeader(m)
	}
	field := ""
	if idx := rfindRune(hdr, ':'); idx >= 0 {
		field = strings.TrimSpace(strings.TrimRight(strings.TrimSpace(runeSliceFrom(hdr, idx+1)), "{"))
	}
	tname := "NPC"
	if len(subjects) > 0 {
		tname = subjects[0].TypeName
	}
	fieldNorm := field
	if strings.HasPrefix(field, "$.") {
		fieldNorm = "$[" + tname + "]." + runeSliceFrom(field, 2)
	} else if !strings.HasPrefix(field, "$") {
		fieldNorm = "$[" + tname + "]." + strings.TrimLeft(field, ".")
	}
	content := strings.TrimSpace(stripOuterBraces(strings.TrimSpace(body)))

	type rawBranch struct {
		name    string
		actions []ActionOp
	}
	branches := []rawBranch{}
	rs := []rune(content)
	n := len(rs)
	i := 0
	for i < n {
		sub := string(rs[i:])
		loc := rangeBranchRe.FindStringSubmatchIndex(sub)
		if loc == nil {
			break
		}
		name := strings.TrimSpace(sub[loc[2]:loc[3]])
		bracePos := i + byteToRuneIndex(sub, loc[1]-1)
		depth := 1
		j := bracePos + 1
		for j < n && depth > 0 {
			if rs[j] == '{' {
				depth++
			} else if rs[j] == '}' {
				depth--
			}
			j++
		}
		inner := runeSlice(content, bracePos+1, j-1)
		branches = append(branches, rawBranch{name: name, actions: parseActionLines(inner, pos)})
		i = j
	}

	type rangeBucket struct {
		value   float64
		isInt   bool
		intVal  int64
		actions []ActionOp
	}
	type otherBranch struct {
		cond    string
		actions []ActionOp
	}
	buckets := []rangeBucket{}
	others := []otherBranch{}
	for _, br := range branches {
		if len(br.actions) == 0 {
			continue
		}
		name := strings.TrimSpace(br.name)
		if m := rangeCmpRe.FindStringSubmatch(name); m != nil {
			op := m[1]
			if op == "=" {
				op = "=="
			}
			others = append(others, otherBranch{cond: fieldNorm + " " + op + " " + valStr(m[2]), actions: br.actions})
			continue
		}
		if isDigitString(name) {
			if v, err := strconv.ParseInt(name, 10, 64); err == nil {
				buckets = append(buckets, rangeBucket{value: float64(v), isInt: true, intVal: v, actions: br.actions})
			} else {
				others = append(others, otherBranch{cond: fieldNorm + " == " + valStr(name), actions: br.actions})
			}
			continue
		}
		if isDigitString(strings.Replace(name, ".", "", 1)) {
			if f, err := strconv.ParseFloat(name, 64); err == nil {
				buckets = append(buckets, rangeBucket{value: f, actions: br.actions})
			} else {
				others = append(others, otherBranch{cond: fieldNorm + " == " + valStr(name), actions: br.actions})
			}
			continue
		}
		others = append(others, otherBranch{cond: fieldNorm + " == " + valStr(name), actions: br.actions})
	}

	sort.SliceStable(buckets, func(a, b int) bool { return buckets[a].value < buckets[b].value })

	renderBucket := func(b rangeBucket) string {
		if b.isInt {
			return strconv.FormatInt(b.intVal, 10)
		}
		return pythonFloatStr(b.value)
	}

	result := []RuleDef{}
	for idx, bucket := range buckets {
		var cond string
		if idx == 0 {
			cond = fieldNorm + " <= " + renderBucket(bucket)
		} else {
			cond = fieldNorm + " > " + renderBucket(buckets[idx-1]) + " ∧ " + fieldNorm + " <= " + renderBucket(bucket)
		}
		result = append(result, RuleDef{Subjects: subjects, Conditions: []string{cond}, Actions: bucket.actions, Pos: pos})
	}
	for _, other := range others {
		result = append(result, RuleDef{Subjects: subjects, Conditions: []string{other.cond}, Actions: other.actions, Pos: pos})
	}
	return result
}

// parseMultiIf parses several `条件 { 动作 }` branches separated by commas; it
// returns nil when the body is not such a multi-branch structure.
func parseMultiIf(body string, subjects []Subject, pos Pos) []RuleDef {
	content := strings.TrimSpace(stripOuterBraces(strings.TrimSpace(body)))
	if content == "" {
		return nil
	}
	content, priority, ruleID := consumeRuleMetaPrefixes(content)
	if content == "" {
		return nil
	}

	type multiBranch struct {
		cond    string
		actions []ActionOp
	}
	branches := []multiBranch{}
	rs := []rune(content)
	n := len(rs)
	i := 0
	for i < n {
		bracePos := -1
		j := i
		for j < n {
			if rs[j] == '{' {
				before := strings.TrimSpace(string(rs[i:j]))
				if before == "" || strings.HasSuffix(before, " in") || before == "in" {
					depthIn := 1
					k := j + 1
					for k < n && depthIn > 0 {
						if rs[k] == '{' {
							depthIn++
						} else if rs[k] == '}' {
							depthIn--
						}
						k++
					}
					j = k
					continue
				}
				bracePos = j
				break
			}
			j++
		}
		if bracePos < 0 {
			break
		}
		condExpr := strings.TrimSpace(string(rs[i:bracePos]))
		depth := 1
		k := bracePos + 1
		for k < n && depth > 0 {
			if rs[k] == '{' {
				depth++
			} else if rs[k] == '}' {
				depth--
			}
			k++
		}
		actionText := runeSlice(content, bracePos+1, k-1)
		branches = append(branches, multiBranch{cond: condExpr, actions: parseActionLines(actionText, pos)})
		i = k
		for i < n && (unicode.IsSpace(rs[i]) || rs[i] == ',') {
			i++
		}
		if i >= n {
			break
		}
	}
	if len(branches) < 2 {
		return nil
	}
	result := []RuleDef{}
	for _, br := range branches {
		if len(br.actions) == 0 {
			continue
		}
		result = append(result, RuleDef{
			Subjects: subjects, Conditions: []string{br.cond}, Actions: br.actions,
			Priority: priority, RuleID: ruleID, Pos: pos,
		})
	}
	if len(result) == 0 {
		return nil
	}
	return result
}

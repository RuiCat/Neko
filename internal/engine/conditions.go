// Condition expression compilation and evaluation.
//
// This file is the Go port of astrbox-plugin/state_machine/conditions.py.
// Grammar:
//
//	expr  := atom | expr ('∧'|'and') expr | expr ('∨'|'or') expr
//	       | ('!'|'~') expr | '(' expr ')'
//	atom  := path op value | path            (existence / truthiness)
//	op    := >= | <= | != | == | contains | in | > | < | =
//	value := number | "string" | true/false | {a,b,c} | path reference
package engine

import (
	"fmt"
	"strconv"
	"strings"
	"unicode"
)

// ValueResolver resolves a condition/action path to a value. An error means the
// path is unresolved; unresolved paths make comparisons and existence checks
// evaluate to false instead of aborting the whole expression.
type ValueResolver func(path string) (any, error)

// Condition is a compiled condition expression.
type Condition func(resolver ValueResolver) bool

// Eval evaluates the condition; it is equivalent to calling the Condition.
func (c Condition) Eval(resolver ValueResolver) bool { return c(resolver) }

// ConditionError reports an invalid condition expression.
type ConditionError struct{ Message string }

func (e *ConditionError) Error() string { return e.Message }

// setLiteral is the parsed form of "{a,b,c}" on the right-hand side.
type setLiteral struct{ items []string }

// stripOuterParens removes redundant wrapping parentheses.
func stripOuterParens(expr string) string {
	expr = strings.TrimSpace(expr)
	for strings.HasPrefix(expr, "(") && strings.HasSuffix(expr, ")") {
		runes := []rune(expr)
		depth := 0
		wrap := true
		for i, ch := range runes {
			switch ch {
			case '(':
				depth++
			case ')':
				depth--
				if depth == 0 && i < len(runes)-1 {
					wrap = false
				}
			}
			if !wrap {
				break
			}
		}
		if !wrap || depth != 0 {
			break
		}
		expr = strings.TrimSpace(string(runes[1 : len(runes)-1]))
	}
	return expr
}

func isIdentRune(ch rune) bool {
	return unicode.IsLetter(ch) || unicode.IsDigit(ch) || ch == '_'
}

// splitTopLevel splits expr on top-level occurrences of the given operators,
// ignoring parentheses and quoted strings.
func splitTopLevel(expr string, ops ...string) ([]string, error) {
	runes := []rune(expr)
	var parts []string
	var current []rune
	depth := 0
	var quote rune
	for i := 0; i < len(runes); {
		ch := runes[i]
		if quote != 0 {
			current = append(current, ch)
			if ch == quote {
				quote = 0
			}
			i++
			continue
		}
		if ch == '"' || ch == '\'' {
			quote = ch
			current = append(current, ch)
			i++
			continue
		}
		if ch == '(' {
			depth++
			current = append(current, ch)
			i++
			continue
		}
		if ch == ')' {
			depth--
			if depth < 0 {
				return nil, &ConditionError{Message: fmt.Sprintf("括号不匹配: %s", expr)}
			}
			current = append(current, ch)
			i++
			continue
		}
		matched := false
		for _, op := range ops {
			opRunes := []rune(op)
			if i+len(opRunes) > len(runes) || string(runes[i:i+len(opRunes)]) != op {
				continue
			}
			if op == "and" || op == "or" {
				leftOK := i == 0 || !isIdentRune(runes[i-1])
				rightOK := i+len(opRunes) >= len(runes) || !isIdentRune(runes[i+len(opRunes)])
				if !leftOK || !rightOK {
					break
				}
			}
			parts = append(parts, strings.TrimSpace(string(current)))
			current = current[:0]
			i += len(opRunes)
			matched = true
			break
		}
		if matched {
			continue
		}
		current = append(current, ch)
		i++
	}
	if quote != 0 {
		return nil, &ConditionError{Message: fmt.Sprintf("字符串引号未闭合: %s", expr)}
	}
	if depth != 0 {
		return nil, &ConditionError{Message: fmt.Sprintf("括号不匹配: %s", expr)}
	}
	parts = append(parts, strings.TrimSpace(string(current)))
	return parts, nil
}

func parseCondition(expr string) (Condition, error) {
	expr = stripOuterParens(expr)
	if expr == "" {
		return func(ValueResolver) bool { return true }, nil
	}
	if strings.HasPrefix(expr, "!") || strings.HasPrefix(expr, "~") {
		inner, err := parseCondition(strings.TrimSpace(expr[1:]))
		if err != nil {
			return nil, err
		}
		return func(resolver ValueResolver) bool { return !inner(resolver) }, nil
	}
	orParts, err := splitTopLevel(expr, "∨", "or")
	if err != nil {
		return nil, err
	}
	if len(orParts) > 1 {
		subs := make([]Condition, 0, len(orParts))
		for _, part := range orParts {
			sub, err := parseCondition(part)
			if err != nil {
				return nil, err
			}
			subs = append(subs, sub)
		}
		return func(resolver ValueResolver) bool {
			for _, sub := range subs {
				if sub(resolver) {
					return true
				}
			}
			return false
		}, nil
	}
	andParts, err := splitTopLevel(expr, "∧", "and")
	if err != nil {
		return nil, err
	}
	if len(andParts) > 1 {
		subs := make([]Condition, 0, len(andParts))
		for _, part := range andParts {
			sub, err := parseCondition(part)
			if err != nil {
				return nil, err
			}
			subs = append(subs, sub)
		}
		return func(resolver ValueResolver) bool {
			for _, sub := range subs {
				if !sub(resolver) {
					return false
				}
			}
			return true
		}, nil
	}
	return parseAtom(expr)
}

var comparisonOperators = []string{">=", "<=", "!=", "==", "contains", "in", ">", "<", "="}

func findOperatorIndex(runes []rune, op string) int {
	opRunes := []rune(op)
	depth := 0
	var quote rune
	for i := 0; i < len(runes); i++ {
		ch := runes[i]
		if quote != 0 {
			if ch == quote {
				quote = 0
			}
			continue
		}
		if ch == '"' || ch == '\'' {
			quote = ch
			continue
		}
		switch ch {
		case '(':
			depth++
			continue
		case ')':
			depth--
			continue
		}
		if depth != 0 || i+len(opRunes) > len(runes) || string(runes[i:i+len(opRunes)]) != op {
			continue
		}
		if op == "=" && i > 0 && strings.ContainsRune("=!<>", runes[i-1]) {
			continue
		}
		if op == "in" || op == "contains" {
			before := rune(' ')
			after := rune(' ')
			if i > 0 {
				before = runes[i-1]
			}
			if i+len(opRunes) < len(runes) {
				after = runes[i+len(opRunes)]
			}
			if isIdentRune(before) || isIdentRune(after) {
				continue
			}
		}
		return i
	}
	return -1
}

func parseAtom(expr string) (Condition, error) {
	expr = strings.TrimSpace(expr)
	runes := []rune(expr)
	for _, op := range comparisonOperators {
		idx := findOperatorIndex(runes, op)
		if idx < 0 {
			continue
		}
		left := strings.TrimSpace(string(runes[:idx]))
		right := strings.TrimSpace(string(runes[idx+len([]rune(op)):]))
		if left == "" || right == "" {
			return nil, &ConditionError{Message: fmt.Sprintf("比较运算缺少操作数: %s", expr)}
		}
		return makeComparison(left, right, op), nil
	}
	return func(resolver ValueResolver) bool {
		value, ok := resolveAnyValue(expr, resolver)
		return ok && value != nil
	}, nil
}

func makeComparison(left, right, op string) Condition {
	rightValue := parseLiteralValue(right)
	rightIsRef := looksLikePathRef(right)
	return func(resolver ValueResolver) bool {
		leftValue, ok := resolveAnyValue(left, resolver)
		if !ok {
			return false
		}
		if rightIsRef {
			resolved, ok := resolveAnyValue(right, resolver)
			if !ok {
				return false
			}
			return compareValues(leftValue, resolved, op)
		}
		return compareValues(leftValue, rightValue, op)
	}
}

// parseLiteralValue parses the right-hand side literal forms.
func parseLiteralValue(raw string) any {
	raw = strings.TrimSpace(raw)
	if strings.HasPrefix(raw, "{") && strings.HasSuffix(raw, "}") {
		inner := raw[1 : len(raw)-1]
		fields := strings.FieldsFunc(inner, func(r rune) bool { return r == ',' || r == '，' })
		items := make([]string, 0, len(fields))
		for _, field := range fields {
			if item := strings.TrimSpace(field); item != "" {
				items = append(items, item)
			}
		}
		return setLiteral{items: items}
	}
	if raw == "true" {
		return true
	}
	if raw == "false" {
		return false
	}
	if number, ok := parseNumber(raw); ok {
		return number
	}
	if len(raw) >= 2 {
		if (raw[0] == '"' && raw[len(raw)-1] == '"') || (raw[0] == '\'' && raw[len(raw)-1] == '\'') {
			return raw[1 : len(raw)-1]
		}
	}
	return raw
}

// looksLikePathRef reports whether the right-hand side should be resolved at
// runtime rather than treated as a literal.
func looksLikePathRef(raw string) bool {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return false
	}
	if strings.HasPrefix(raw, "$") || strings.HasPrefix(raw, "{e}") {
		return true
	}
	if strings.HasPrefix(raw, "\"") || strings.HasPrefix(raw, "'") || strings.HasPrefix(raw, "{") {
		return false
	}
	if raw == "true" || raw == "false" {
		return false
	}
	if _, ok := parseNumber(raw); ok {
		return false
	}
	return strings.Contains(raw, ".")
}

// resolveAnyValue resolves a condition operand: boolean/number literal first,
// then a resolver lookup.
func resolveAnyValue(ref string, resolver ValueResolver) (any, bool) {
	ref = strings.TrimSpace(ref)
	if ref == "true" {
		return true, true
	}
	if ref == "false" {
		return false, true
	}
	if number, ok := parseNumber(ref); ok {
		return number, true
	}
	if resolver != nil && looksArithmetic(ref) {
		if value, ok := evalArithmetic(ref, resolver); ok {
			return value, true
		}
	}
	if resolver == nil {
		return nil, false
	}
	value, err := resolver(ref)
	if err != nil {
		return nil, false
	}
	return value, true
}

func parseNumber(raw string) (float64, bool) {
	value, err := strconv.ParseFloat(strings.TrimSpace(raw), 64)
	if err != nil {
		return 0, false
	}
	return value, true
}

func compareValues(left, right any, op string) bool {
	if left == nil || right == nil {
		return false
	}
	if set, ok := right.(setLiteral); ok {
		if op == "in" || op == "==" {
			for _, item := range set.items {
				if item == pyString(left) {
					return true
				}
			}
		}
		return false
	}
	if isNonScalar(left) || isNonScalar(right) {
		return false
	}
	if op == "contains" {
		return strings.Contains(pyString(left), pyString(right))
	}
	leftNumber, leftNumeric := toFloatValue(left)
	rightNumber, rightNumeric := toFloatValue(right)
	numeric := leftNumeric && rightNumeric
	switch op {
	case "==", "=":
		if numeric {
			return leftNumber == rightNumber
		}
		return pyString(left) == pyString(right)
	case "!=":
		if numeric {
			return leftNumber != rightNumber
		}
		return pyString(left) != pyString(right)
	case ">":
		if numeric {
			return leftNumber > rightNumber
		}
		return pyString(left) > pyString(right)
	case "<":
		if numeric {
			return leftNumber < rightNumber
		}
		return pyString(left) < pyString(right)
	case ">=":
		if numeric {
			return leftNumber >= rightNumber
		}
		return pyString(left) >= pyString(right)
	case "<=":
		if numeric {
			return leftNumber <= rightNumber
		}
		return pyString(left) <= pyString(right)
	case "in":
		return strings.Contains(pyString(right), pyString(left))
	}
	return false
}

func isNonScalar(value any) bool {
	switch value.(type) {
	case map[string]any, []any:
		return true
	}
	return false
}

func toFloatValue(value any) (float64, bool) {
	switch typed := value.(type) {
	case float64:
		return typed, true
	case float32:
		return float64(typed), true
	case int:
		return float64(typed), true
	case int64:
		return float64(typed), true
	case int32:
		return float64(typed), true
	case uint:
		return float64(typed), true
	case uint64:
		return float64(typed), true
	}
	return 0, false
}

// pyString mirrors Python's str() for the value types that appear in entities.
func pyString(value any) string {
	switch typed := value.(type) {
	case nil:
		return "None"
	case string:
		return typed
	case bool:
		if typed {
			return "True"
		}
		return "False"
	case float64:
		return pyFloatString(typed)
	case float32:
		return pyFloatString(float64(typed))
	case int:
		return strconv.Itoa(typed)
	case int64:
		return strconv.FormatInt(typed, 10)
	case int32:
		return strconv.FormatInt(int64(typed), 10)
	default:
		return fmt.Sprint(value)
	}
}

func pyFloatString(value float64) string {
	absolute := value
	if absolute < 0 {
		absolute = -absolute
	}
	var text string
	if absolute == 0 || (absolute >= 1e-4 && absolute < 1e16) {
		text = strconv.FormatFloat(value, 'f', -1, 64)
	} else {
		text = strconv.FormatFloat(value, 'g', -1, 64)
	}
	if !strings.ContainsAny(text, ".eE") {
		text += ".0"
	}
	return text
}

// CompileCondition parses an expression into a reusable evaluator.
func CompileCondition(expr string) (Condition, error) {
	return parseCondition(expr)
}

// EvaluateCondition compiles and evaluates an expression in one call.
func EvaluateCondition(expr string, resolver ValueResolver) (bool, error) {
	condition, err := CompileCondition(expr)
	if err != nil {
		return false, err
	}
	return condition(resolver), nil
}

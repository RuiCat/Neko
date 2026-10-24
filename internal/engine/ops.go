// Action operations (set/add/append/emit) and action-value evaluation.
//
// Ported from StateMachine._apply_op and the concatenation helpers in
// astrbox-plugin/state_machine/engine.py.
package engine

import (
	"fmt"
	"reflect"
	"strconv"
	"strings"

	"github.com/ruicat/astrbox/internal/config"
	"github.com/ruicat/astrbox/internal/entity"
)

// applyOp runs one op against the triggering entity or an explicit target and
// returns its recorded operation, or nil when nothing happens.
func (m *StateMachine) applyOp(ent *entity.Entity, op Op, resolver ValueResolver, bindings map[int]*entity.Entity) map[string]any {
	target := ent
	switch {
	case op.Entity != "":
		target = m.store.Get(op.Entity)
		if target == nil {
			return nil
		}
	case op.Subject > 0 && bindings != nil:
		target = bindings[op.Subject]
		if target == nil {
			return nil
		}
	}

	path := normalizeFieldAlias(op.Path)
	path = strings.ReplaceAll(path, "{e}", target.ID)
	if path == target.ID {
		path = ""
	} else if strings.HasPrefix(path, target.ID+".") {
		path = path[len(target.ID)+1:]
	}
	if op.Subject > 0 {
		varName := fmt.Sprintf("$%d", op.Subject)
		if strings.HasPrefix(path, varName+".") {
			path = path[len(varName)+1:]
		}
	}
	if bindings != nil && strings.HasPrefix(path, "$") {
		keys := sortedBindingKeys(bindings)
		for _, key := range keys {
			varName := fmt.Sprintf("$%d", key)
			if path == varName {
				target = bindings[key]
				path = ""
				break
			}
			if strings.HasPrefix(path, varName+".") {
				target = bindings[key]
				path = path[len(varName)+1:]
				break
			}
		}
	}

	value := m.resolveOpValue(op.Value, ent, target, resolver)

	switch op.Op {
	case "set", "":
		oldValue := target.GetOrNone(path)
		newValue := m.clampValue(target, path, value)
		if err := target.Set(path, newValue); err != nil {
			return nil
		}
		m.applyTextEffects(target, value)
		if equalValues(oldValue, value) {
			return nil
		}
		return map[string]any{"op": "set", "path": path, "value": value}
	case "add":
		current := target.GetOrNone(path)
		if isFalsy(current) {
			current = 0
		}
		currentNumber, currentOK := toFloatValue(current)
		valueNumber, valueOK := toFloatValue(value)
		var newValue any
		if currentOK && valueOK {
			newValue = m.clampValue(target, path, currentNumber+valueNumber)
		} else {
			newValue = m.clampValue(target, path, value)
		}
		if err := target.Set(path, newValue); err != nil {
			return nil
		}
		return map[string]any{"op": "add", "path": path, "value": value}
	case "append":
		current := target.GetOrNone(path)
		items, ok := current.([]any)
		if !ok {
			items = []any{}
		}
		items = append(items, value)
		if err := target.Set(path, items); err != nil {
			return nil
		}
		return map[string]any{"op": "append", "path": path, "value": value}
	case "emit":
		sceneID := op.Entity
		if sceneID == "" {
			sceneID = config.Current().DefaultScene
		}
		scene := m.store.Get(sceneID)
		if scene == nil {
			return nil
		}
		events, ok := scene.GetOrNone(config.Current().SceneEventField).([]any)
		if !ok {
			events = []any{}
		}
		events = append(events, value)
		if err := scene.Set(config.Current().SceneEventField, events); err != nil {
			return nil
		}
		return map[string]any{"op": "emit", "path": sceneID + ".事件", "value": value}
	}
	return nil
}

func sortedBindingKeys(bindings map[int]*entity.Entity) []int {
	keys := make([]int, 0, len(bindings))
	for key := range bindings {
		keys = append(keys, key)
	}
	for i := 0; i < len(keys); i++ {
		for j := i + 1; j < len(keys); j++ {
			if keys[j] < keys[i] {
				keys[i], keys[j] = keys[j], keys[i]
			}
		}
	}
	return keys
}

// resolveOpValue evaluates concat expressions, $ path references and &{field}
// placeholders; anything unresolvable stays a literal.
func (m *StateMachine) resolveOpValue(value any, ent, target *entity.Entity, resolver ValueResolver) any {
	text, ok := value.(string)
	if !ok {
		return value
	}
	if isFunctionCall(text) {
		if resolved, handled := m.evaluateFuncCall(text, resolver, ent); handled {
			value = resolved
			if resolvedText, ok := resolved.(string); ok {
				text = resolvedText
			} else {
				text = ""
			}
		}
	} else if resolved, ok := evalConcatExpr(text, resolver); ok {
		value = resolved
		text = resolved
	} else if strings.HasPrefix(text, "$") {
		if resolved, err := resolver(text[1:]); err == nil {
			value = resolved
			if resolvedText, ok := resolved.(string); ok {
				text = resolvedText
			} else {
				text = ""
			}
		}
	}
	if strings.Contains(text, "&{") {
		value = m.replacePlaceholders(text, target)
	}
	return value
}

func (m *StateMachine) replacePlaceholders(text string, target *entity.Entity) string {
	for {
		start := strings.Index(text, "&{")
		if start < 0 {
			return text
		}
		end := strings.Index(text[start:], "}")
		if end < 0 {
			return text
		}
		end += start
		field := text[start+2 : end]
		placeholder := text[start : end+1]
		fieldValue := target.GetOrNone(field)
		if fieldValue != nil {
			text = strings.Replace(text, placeholder, pyString(fieldValue), 1)
			continue
		}
		if field == "自称" {
			text = strings.Replace(text, placeholder, target.ID, 1)
			continue
		}
		// Unresolved placeholder: leave it untouched and stop scanning.
		return text
	}
}

// applyTextEffects mirrors the side effect of a set op whose value is effect
// text ("撒娇+15" / "安全等级=red").
func (m *StateMachine) applyTextEffects(target *entity.Entity, value any) {
	text, ok := value.(string)
	if !ok {
		return
	}
	hasAssign := strings.ContainsAny(text, ":=：＝")
	hasEffect := strings.ContainsAny(text, "+-＋－")
	if hasAssign {
		ApplyAssign(target, text)
	}
	if hasEffect {
		ApplyEffects(target, text)
	}
}

func isFunctionCall(text string) bool {
	trimmed := strings.TrimRight(text, " \t")
	return strings.Contains(trimmed, "(") && strings.HasSuffix(trimmed, ")")
}

// splitConcatExpr splits "A + B + C" on plus signs that have whitespace on
// both sides, ignoring quoted strings. It returns nil when the text is not a
// concat expression.
func splitConcatExpr(value string) []string {
	runes := []rune(value)
	var parts []string
	var buffer []rune
	for i := 0; i < len(runes); {
		ch := runes[i]
		if ch == '"' || ch == '\'' {
			quote := ch
			buffer = append(buffer, ch)
			i++
			for i < len(runes) {
				current := runes[i]
				buffer = append(buffer, current)
				if current == '\\' && i+1 < len(runes) {
					buffer = append(buffer, runes[i+1])
					i += 2
					continue
				}
				i++
				if current == quote {
					break
				}
			}
			continue
		}
		if ch == '+' {
			previousSpace := len(buffer) == 0 || isSpaceRune(buffer[len(buffer)-1])
			nextSpace := i+1 < len(runes) && isSpaceRune(runes[i+1])
			if previousSpace && nextSpace {
				parts = append(parts, strings.TrimSpace(string(buffer)))
				buffer = buffer[:0]
				i++
				continue
			}
			buffer = append(buffer, ch)
			i++
			continue
		}
		buffer = append(buffer, ch)
		i++
	}
	if len(parts) == 0 {
		return nil
	}
	parts = append(parts, strings.TrimSpace(string(buffer)))
	return parts
}

func isSpaceRune(ch rune) bool {
	return ch == ' ' || ch == '\t' || ch == '\n' || ch == '\r'
}

// evalConcatExpr evaluates a concat expression, returning ok=false when the
// text is not a concat expression or any fragment cannot be resolved.
func evalConcatExpr(value string, resolver ValueResolver) (string, bool) {
	fragments := splitConcatExpr(value)
	if len(fragments) < 2 {
		return value, false
	}
	var builder strings.Builder
	for _, fragment := range fragments {
		resolved, err := resolveConcatFragment(fragment, resolver)
		if err != nil {
			return value, false
		}
		builder.WriteString(resolved)
	}
	return builder.String(), true
}

func resolveConcatFragment(fragment string, resolver ValueResolver) (string, error) {
	fragment = strings.TrimSpace(fragment)
	if fragment == "" {
		return "", fmt.Errorf("空片段")
	}
	if fragment[0] == '"' || fragment[0] == '\'' {
		if len(fragment) >= 2 && fragment[len(fragment)-1] == fragment[0] {
			return fragment[1 : len(fragment)-1], nil
		}
	}
	if strings.HasPrefix(fragment, "$") {
		runes := []rune(fragment)
		if len(runes) > 1 && runes[1] >= '0' && runes[1] <= '9' {
			value, err := resolver(fragment)
			if err != nil {
				return "", err
			}
			return pyString(value), nil
		}
		if strings.HasPrefix(fragment, "$[") {
			closeIndex := strings.Index(fragment, "]")
			if closeIndex >= 0 {
				inner := fragment[2:closeIndex]
				rest := fragment[closeIndex+1:]
				if strings.HasPrefix(inner, "*") {
					value, err := resolver(fragment[1:])
					if err != nil {
						return "", err
					}
					return pyString(value), nil
				}
				value, err := resolver("{e}" + rest)
				if err != nil {
					return "", err
				}
				return pyString(value), nil
			}
		}
		if fragment == "$" {
			value, err := resolver("{e}")
			if err != nil {
				return "", err
			}
			return pyString(value), nil
		}
		if strings.HasPrefix(fragment, "$.") {
			value, err := resolver("{e}" + fragment[1:])
			if err != nil {
				return "", err
			}
			return pyString(value), nil
		}
		value, err := resolver(fragment[1:])
		if err != nil {
			return "", err
		}
		return pyString(value), nil
	}
	if strings.HasPrefix(fragment, "{e}") {
		value, err := resolver(fragment)
		if err != nil {
			return "", err
		}
		return pyString(value), nil
	}
	if number, err := strconv.Atoi(fragment); err == nil {
		return strconv.Itoa(number), nil
	}
	if number, err := strconv.ParseFloat(fragment, 64); err == nil {
		return pyFloatString(number), nil
	}
	return "", fmt.Errorf("无法解析片段: %s", fragment)
}

func isFalsy(value any) bool {
	switch typed := value.(type) {
	case nil:
		return true
	case bool:
		return !typed
	case int:
		return typed == 0
	case int64:
		return typed == 0
	case float64:
		return typed == 0
	case string:
		return typed == ""
	case []any:
		return len(typed) == 0
	case map[string]any:
		return len(typed) == 0
	}
	return false
}

// equalValues compares like Python's == for the scalar/collection types here.
func equalValues(left, right any) bool {
	leftNumber, leftNumeric := toFloatValue(left)
	rightNumber, rightNumeric := toFloatValue(right)
	if leftNumeric && rightNumeric {
		return leftNumber == rightNumber
	}
	return reflect.DeepEqual(left, right)
}

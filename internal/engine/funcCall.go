// Function-call bridge between rules and an external @函数 executor: conditions
// and action values of the form 函数名(参数...) are resolved here.
package engine

import (
	"strconv"
	"strings"
	"unicode"

	"github.com/ruicat/astrbox/internal/entity"
)

// FuncCaller executes a named .neko function with resolved arguments.
type FuncCaller interface {
	Call(name string, args []any, current *entity.Entity) (any, error)
}

// SetFuncCaller installs the function executor used by conditions and action
// values; nil restores the literal-only behaviour.
func (m *StateMachine) SetFuncCaller(caller FuncCaller) {
	m.callMu.Lock()
	defer m.callMu.Unlock()
	m.funcCaller = caller
}

// currentFuncCaller reads the installed caller under its own lock.
func (m *StateMachine) currentFuncCaller() FuncCaller {
	m.callMu.RLock()
	defer m.callMu.RUnlock()
	return m.funcCaller
}

// evaluateFuncCall resolves "函数名(参数...)" through the installed caller.
// handled is false when the text is not a call or no caller is installed, so
// callers keep the legacy literal/path behaviour.
func (m *StateMachine) evaluateFuncCall(text string, resolver ValueResolver, current *entity.Entity) (any, bool) {
	caller := m.currentFuncCaller()
	if caller == nil || !isFunctionCall(text) {
		return nil, false
	}
	name, rawArgs, ok := splitCallExpression(strings.TrimSpace(text))
	if !ok {
		return nil, false
	}
	args := make([]any, 0, len(rawArgs))
	for _, raw := range rawArgs {
		args = append(args, m.resolveCallArgument(raw, resolver, current))
	}
	value, err := caller.Call(name, args, current)
	if err != nil {
		return nil, false
	}
	return value, true
}

// splitCallExpression splits "函数名(a, b)" into its name and raw arguments.
func splitCallExpression(text string) (string, []string, bool) {
	runes := []rune(text)
	open := -1
	for index, ch := range runes {
		if ch == '(' {
			open = index
			break
		}
	}
	if open <= 0 {
		return "", nil, false
	}
	name := strings.TrimSpace(string(runes[:open]))
	if name == "" || !isCallName(name) {
		return "", nil, false
	}
	depth := 0
	closeIndex := -1
	var quote rune
	for index := open; index < len(runes); index++ {
		ch := runes[index]
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
		if ch == '(' {
			depth++
			continue
		}
		if ch == ')' {
			depth--
			if depth == 0 {
				closeIndex = index
				break
			}
		}
	}
	if closeIndex < 0 || strings.TrimSpace(string(runes[closeIndex+1:])) != "" {
		return "", nil, false
	}
	inner := strings.TrimSpace(string(runes[open+1 : closeIndex]))
	if inner == "" {
		return name, nil, true
	}
	return name, splitCallArguments(inner), true
}

// isCallName reports whether every rune of a function name is identifier-like.
func isCallName(name string) bool {
	for _, ch := range name {
		if !unicode.IsLetter(ch) && !unicode.IsDigit(ch) && ch != '_' {
			return false
		}
	}
	return true
}

// splitCallArguments splits raw arguments on top-level commas.
func splitCallArguments(inner string) []string {
	args := []string{}
	var current []rune
	depth := 0
	var quote rune
	for _, ch := range inner {
		if quote != 0 {
			current = append(current, ch)
			if ch == quote {
				quote = 0
			}
			continue
		}
		if ch == '"' || ch == '\'' {
			quote = ch
			current = append(current, ch)
			continue
		}
		switch ch {
		case '(', '[', '{':
			depth++
		case ')', ']', '}':
			depth--
		case ',':
			if depth == 0 {
				if argument := strings.TrimSpace(string(current)); argument != "" {
					args = append(args, argument)
				}
				current = current[:0]
				continue
			}
		}
		current = append(current, ch)
	}
	if argument := strings.TrimSpace(string(current)); argument != "" {
		args = append(args, argument)
	}
	return args
}

// resolveCallArgument resolves one raw argument: literal, nested call, slice or
// path lookup, falling back to the raw text like the Python bridge.
func (m *StateMachine) resolveCallArgument(raw string, resolver ValueResolver, current *entity.Entity) any {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	if len(raw) >= 2 && ((raw[0] == '"' && raw[len(raw)-1] == '"') || (raw[0] == '\'' && raw[len(raw)-1] == '\'')) {
		return strings.Trim(raw, `"'`)
	}
	switch raw {
	case "true":
		return true
	case "false":
		return false
	}
	if number, err := strconv.Atoi(raw); err == nil {
		return number
	}
	if number, err := strconv.ParseFloat(raw, 64); err == nil {
		return number
	}
	if isFunctionCall(raw) {
		if value, handled := m.evaluateFuncCall(raw, resolver, current); handled {
			return value
		}
	}
	if base, low, high, ok := splitSliceArgument(raw); ok {
		if resolver == nil {
			return raw
		}
		value, err := resolver(base)
		if err != nil {
			return raw
		}
		return sliceCallValue(value, low, high)
	}
	if resolver == nil {
		return raw
	}
	value, err := resolver(raw)
	if err != nil {
		return raw
	}
	return value
}

// splitSliceArgument parses "base[low:high]" and returns 1-based bounds sentinels
// with -1 meaning "unspecified".
func splitSliceArgument(raw string) (string, int, int, bool) {
	if !strings.HasSuffix(raw, "]") {
		return "", 0, 0, false
	}
	open := strings.Index(raw, "[")
	if open <= 0 {
		return "", 0, 0, false
	}
	parts := strings.SplitN(raw[open+1:len(raw)-1], ":", 2)
	low, high := -1, -1
	if len(parts) >= 1 && strings.TrimSpace(parts[0]) != "" {
		parsed, err := strconv.Atoi(strings.TrimSpace(parts[0]))
		if err != nil {
			return "", 0, 0, false
		}
		low = parsed
	}
	if len(parts) == 2 && strings.TrimSpace(parts[1]) != "" {
		parsed, err := strconv.Atoi(strings.TrimSpace(parts[1]))
		if err != nil {
			return "", 0, 0, false
		}
		high = parsed
	}
	return strings.TrimSpace(raw[:open]), low, high, true
}

// sliceCallValue slices a string or list with Python-style bounds.
func sliceCallValue(value any, low, high int) any {
	switch typed := value.(type) {
	case string:
		runes := []rune(typed)
		start, end := clampCallBounds(len(runes), low, high)
		return string(runes[start:end])
	case []any:
		start, end := clampCallBounds(len(typed), low, high)
		return append([]any{}, typed[start:end]...)
	}
	return value
}

// clampCallBounds normalises optional slice bounds.
func clampCallBounds(length, low, high int) (int, int) {
	if low < 0 {
		low = 0
	}
	if high < 0 || high > length {
		high = length
	}
	if low > high {
		low = high
	}
	return low, high
}

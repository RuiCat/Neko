// Expression evaluation: literals, entity paths, calls, indexing, slices and
// the boolean/arithmetic operator semantics of the .neko function dialect.
package function

import (
	"fmt"
	"strings"
)

// eval evaluates one expression node.
func (r *runner) eval(node expression) (any, error) {
	switch typed := node.(type) {
	case *literalExpression:
		return typed.value, nil
	case *pathExpression:
		return r.getPath(typed), nil
	case *listExpression:
		return r.evalList(typed)
	case *mapExpression:
		return r.evalMap(typed)
	case *listCompExpression:
		return r.evalListComp(typed)
	case *unaryExpression:
		return r.evalUnary(typed)
	case *binaryExpression:
		return r.evalBinary(typed)
	case *chainExpression:
		return r.evalChain(typed)
	case *indexExpression:
		return r.evalIndex(typed)
	case *callExpression:
		return r.evalCall(typed)
	case *methodCallExpression:
		return r.evalMethodCall(typed)
	}
	return nil, fmt.Errorf("不支持的表达式: %T", node)
}

// evalList builds a list literal.
func (r *runner) evalList(node *listExpression) (any, error) {
	items := make([]any, 0, len(node.items))
	for _, item := range node.items {
		value, err := r.eval(item)
		if err != nil {
			return nil, err
		}
		items = append(items, value)
	}
	return items, nil
}

// evalListComp evaluates a list comprehension.
func (r *runner) evalListComp(node *listCompExpression) (any, error) {
	iterable, err := r.eval(node.iterable)
	if err != nil {
		return nil, err
	}
	result := []any{}
	for _, item := range iterableItems(iterable) {
		r.steps++
		if r.steps > MAX_EXEC_STEPS {
			return nil, fmt.Errorf("函数执行步数超过上限 %d", MAX_EXEC_STEPS)
		}
		r.locals[node.variable] = item
		if node.condition != nil {
			condition, err := r.eval(node.condition)
			if err != nil {
				return nil, err
			}
			if !isTruthy(condition) {
				continue
			}
		}
		value, err := r.eval(node.expression)
		if err != nil {
			return nil, err
		}
		result = append(result, value)
	}
	return result, nil
}

// evalMap builds a map literal.
func (r *runner) evalMap(node *mapExpression) (any, error) {
	result := make(map[string]any, len(node.keys))
	for index, key := range node.keys {
		value, err := r.eval(node.values[index])
		if err != nil {
			return nil, err
		}
		result[key] = value
	}
	return result, nil
}

// evalUnary applies "!"/"not" or numeric negation.
func (r *runner) evalUnary(node *unaryExpression) (any, error) {
	operand, err := r.eval(node.operand)
	if err != nil {
		return nil, err
	}
	if node.operator == "!" {
		return !isTruthy(operand), nil
	}
	switch typed := operand.(type) {
	case int:
		return -typed, nil
	case float64:
		return -typed, nil
	}
	return nil, nil
}

// evalBinary applies boolean short-circuit or an infix operator.
func (r *runner) evalBinary(node *binaryExpression) (any, error) {
	switch node.operator {
	case "∨", "or":
		left, err := r.eval(node.left)
		if err != nil {
			return nil, err
		}
		if isTruthy(left) {
			return true, nil
		}
		right, err := r.eval(node.right)
		if err != nil {
			return nil, err
		}
		return isTruthy(right), nil
	case "∧", "and":
		left, err := r.eval(node.left)
		if err != nil {
			return nil, err
		}
		if !isTruthy(left) {
			return false, nil
		}
		right, err := r.eval(node.right)
		if err != nil {
			return nil, err
		}
		return isTruthy(right), nil
	}
	left, err := r.eval(node.left)
	if err != nil {
		return nil, err
	}
	right, err := r.eval(node.right)
	if err != nil {
		return nil, err
	}
	return applyBinaryStrict(node.operator, left, right)
}

// evalChain evaluates a Python-style chained comparison left to right.
func (r *runner) evalChain(node *chainExpression) (any, error) {
	left, err := r.eval(node.operands[0])
	if err != nil {
		return nil, err
	}
	for index, operator := range node.operators {
		right, err := r.eval(node.operands[index+1])
		if err != nil {
			return nil, err
		}
		compared, err := applyBinaryStrict(operator, left, right)
		if err != nil {
			return nil, err
		}
		if !isTruthy(compared) {
			return false, nil
		}
		left = right
	}
	return true, nil
}

// evalIndex reads one list/string element or takes a slice.
func (r *runner) evalIndex(node *indexExpression) (any, error) {
	target, err := r.eval(node.target)
	if err != nil {
		return nil, err
	}
	if node.slice {
		return r.evalSlice(node, target)
	}
	index, err := r.eval(node.low)
	if err != nil {
		return nil, err
	}
	switch typed := target.(type) {
	case []any:
		position, ok := toIntValue(index)
		if !ok {
			return nil, nil
		}
		if position < 0 {
			position += len(typed)
		}
		if position < 0 || position >= len(typed) {
			return nil, nil
		}
		return typed[position], nil
	case string:
		position, ok := toIntValue(index)
		if !ok {
			return nil, nil
		}
		runes := []rune(typed)
		if position < 0 {
			position += len(runes)
		}
		if position < 0 || position >= len(runes) {
			return nil, nil
		}
		return string(runes[position]), nil
	case map[string]any:
		value, _ := typed[pyText(index)]
		return value, nil
	}
	return nil, nil
}

// evalSlice takes 列表[low:high:step] or 文本[low:high:step].
func (r *runner) evalSlice(node *indexExpression, target any) (any, error) {
	step := 1
	if node.step != nil {
		value, err := r.eval(node.step)
		if err != nil {
			return nil, err
		}
		parsed, ok := toIntValue(value)
		if !ok || parsed == 0 {
			return nil, newScriptError("value", "切片步长必须是非零整数")
		}
		step = parsed
	}
	var low, high *int
	if node.low != nil {
		value, err := r.eval(node.low)
		if err != nil {
			return nil, err
		}
		if parsed, ok := toIntValue(value); ok {
			low = &parsed
		}
	}
	if node.high != nil {
		value, err := r.eval(node.high)
		if err != nil {
			return nil, err
		}
		if parsed, ok := toIntValue(value); ok {
			high = &parsed
		}
	}
	switch typed := target.(type) {
	case []any:
		indices := sliceIndices(len(typed), low, high, step)
		result := make([]any, 0, len(indices))
		for _, index := range indices {
			result = append(result, typed[index])
		}
		return result, nil
	case string:
		runes := []rune(typed)
		indices := sliceIndices(len(runes), low, high, step)
		result := make([]rune, 0, len(indices))
		for _, index := range indices {
			result = append(result, runes[index])
		}
		return string(result), nil
	}
	return nil, nil
}

// sliceIndices resolves Python slice bounds/steps into concrete indices.
func sliceIndices(length int, low, high *int, step int) []int {
	indices := []int{}
	if step > 0 {
		start, end := 0, length
		if low != nil {
			start = normalizeIndex(*low, length)
			if start < 0 {
				start = 0
			}
			if start > length {
				start = length
			}
		}
		if high != nil {
			end = normalizeIndex(*high, length)
			if end < 0 {
				end = 0
			}
			if end > length {
				end = length
			}
		}
		for index := start; index < end; index += step {
			indices = append(indices, index)
		}
		return indices
	}
	start, end := length-1, -1
	if low != nil {
		start = normalizeIndex(*low, length)
		if start >= length {
			start = length - 1
		}
		if start < -1 {
			start = -1
		}
	}
	if high != nil {
		end = normalizeIndex(*high, length)
	}
	for index := start; index > end; index += step {
		indices = append(indices, index)
	}
	return indices
}

func normalizeIndex(index, length int) int {
	if index < 0 {
		return index + length
	}
	return index
}

// clampBounds normalises Python-style slice bounds; high<0 means "to the end".
func clampBounds(length, low, high int) (int, int) {
	if high < 0 {
		high = length
	}
	if low < 0 {
		low += length
	}
	if high > length {
		high = length
	}
	if low < 0 {
		low = 0
	}
	if low > high {
		low = high
	}
	return low, high
}

// evalCall evaluates arguments and dispatches to a user or builtin function.
func (r *runner) evalCall(node *callExpression) (any, error) {
	args := make([]any, 0, len(node.args))
	for _, argument := range node.args {
		value, err := r.eval(argument)
		if err != nil {
			return nil, err
		}
		args = append(args, value)
	}
	if r.executor.hasFunction(node.name) {
		return r.executor.callFunction(node.name, args, r.current, r.depth+1)
	}
	if value, handled, err := callBuiltin(node.name, args); handled {
		return value, err
	}
	return nil, newScriptError("name", "函数 '%s' 未定义", node.name)
}

// pyJoinText joins values with ", " for display purposes.
func pyJoinText(values []any) string {
	parts := make([]string, 0, len(values))
	for _, value := range values {
		parts = append(parts, pyText(value))
	}
	return strings.Join(parts, ", ")
}

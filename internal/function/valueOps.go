// Value semantics for interpreted functions: Python-like truthiness, numeric
// coercion, comparisons, membership, string conversion and builtin functions.
package function

import (
	"fmt"
	"math"
	"strconv"
	"strings"
)

// isTruthy mirrors Python truthiness for the value types used in scripts.
func isTruthy(value any) bool {
	switch typed := value.(type) {
	case nil:
		return false
	case bool:
		return typed
	case int:
		return typed != 0
	case int64:
		return typed != 0
	case float64:
		return typed != 0
	case string:
		return typed != ""
	case []any:
		return len(typed) > 0
	case map[string]any:
		return len(typed) > 0
	}
	return value != nil
}

// toNumber converts numeric values to float64.
func toNumber(value any) (float64, bool) {
	switch typed := value.(type) {
	case int:
		return float64(typed), true
	case int64:
		return float64(typed), true
	case float64:
		return typed, true
	case float32:
		return float64(typed), true
	}
	return 0, false
}

// toIntValue converts numeric and numeric-text values to int.
func toIntValue(value any) (int, bool) {
	switch typed := value.(type) {
	case int:
		return typed, true
	case int64:
		return int(typed), true
	case float64:
		return int(typed), true
	case bool:
		if typed {
			return 1, true
		}
		return 0, true
	case string:
		number, err := strconv.ParseFloat(strings.TrimSpace(typed), 64)
		if err != nil {
			return 0, false
		}
		return int(number), true
	}
	return 0, false
}

// applyBinary applies one infix operator, returning nil for invalid operands.
func applyBinary(operator string, left, right any) any {
	switch operator {
	case "+":
		leftNumber, leftNumeric := toNumber(left)
		rightNumber, rightNumeric := toNumber(right)
		if leftNumeric && rightNumeric {
			if leftInt, ok := left.(int); ok {
				if rightInt, ok := right.(int); ok {
					return leftInt + rightInt
				}
			}
			return leftNumber + rightNumber
		}
		return pyText(left) + pyText(right)
	case "-", "*", "/", "%":
		leftNumber, leftNumeric := toNumber(left)
		rightNumber, rightNumeric := toNumber(right)
		if !leftNumeric || !rightNumeric {
			return nil
		}
		switch operator {
		case "-":
			if leftInt, ok := left.(int); ok {
				if rightInt, ok := right.(int); ok {
					return leftInt - rightInt
				}
			}
			return leftNumber - rightNumber
		case "*":
			if leftInt, ok := left.(int); ok {
				if rightInt, ok := right.(int); ok {
					return leftInt * rightInt
				}
			}
			return leftNumber * rightNumber
		case "/":
			if rightNumber == 0 {
				return nil
			}
			return leftNumber / rightNumber
		default:
			if rightNumber == 0 {
				return nil
			}
			return math.Mod(leftNumber, rightNumber)
		}
	case "==", "=":
		return compareValues(left, right, "==")
	case "!=":
		return compareValues(left, right, "!=")
	case "<", "<=", ">", ">=":
		return compareValues(left, right, operator)
	case "in":
		return containsValue(right, left)
	case "contains":
		return containsValue(left, right)
	case "∨", "or":
		return isTruthy(left) || isTruthy(right)
	case "∧", "and":
		return isTruthy(left) && isTruthy(right)
	}
	return nil
}

// compareValues compares two values with Python-like rules.
func compareValues(left, right any, operator string) bool {
	if left == nil || right == nil {
		switch operator {
		case "==":
			return left == nil && right == nil
		case "!=":
			return !(left == nil && right == nil)
		}
		return false
	}
	if leftNumber, leftNumeric := toNumber(left); leftNumeric {
		if rightNumber, rightNumeric := toNumber(right); rightNumeric {
			return compareNumbers(leftNumber, rightNumber, operator)
		}
	}
	leftText, leftIsText := left.(string)
	rightText, rightIsText := right.(string)
	if leftIsText && rightIsText {
		return compareTexts(leftText, rightText, operator)
	}
	leftBool, leftIsBool := left.(bool)
	rightBool, rightIsBool := right.(bool)
	if leftIsBool && rightIsBool {
		switch operator {
		case "==":
			return leftBool == rightBool
		case "!=":
			return leftBool != rightBool
		}
		return false
	}
	switch operator {
	case "==":
		return fmt.Sprint(left) == fmt.Sprint(right)
	case "!=":
		return fmt.Sprint(left) != fmt.Sprint(right)
	}
	return false
}

// compareNumbers compares two floats with one operator.
func compareNumbers(left, right float64, operator string) bool {
	switch operator {
	case "==":
		return left == right
	case "!=":
		return left != right
	case "<":
		return left < right
	case "<=":
		return left <= right
	case ">":
		return left > right
	case ">=":
		return left >= right
	}
	return false
}

// compareTexts compares two strings with one operator.
func compareTexts(left, right, operator string) bool {
	switch operator {
	case "==":
		return left == right
	case "!=":
		return left != right
	case "<":
		return left < right
	case "<=":
		return left <= right
	case ">":
		return left > right
	case ">=":
		return left >= right
	}
	return false
}

// containsValue reports whether container holds item (substring for text).
func containsValue(container, item any) bool {
	switch typed := container.(type) {
	case string:
		return strings.Contains(typed, pyText(item))
	case []any:
		for _, candidate := range typed {
			if compareValues(candidate, item, "==") {
				return true
			}
		}
		return false
	case map[string]any:
		_, ok := typed[pyText(item)]
		return ok
	}
	return false
}

// pyText mirrors Python's str() for values produced by the interpreter.
func pyText(value any) string {
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
	case int:
		return strconv.Itoa(typed)
	case int64:
		return strconv.FormatInt(typed, 10)
	case float64:
		return strconv.FormatFloat(typed, 'f', -1, 64)
	case []any:
		return "[" + pyJoinText(typed) + "]"
	case map[string]any:
		return fmt.Sprint(typed)
	}
	return fmt.Sprint(value)
}

// callBuiltin evaluates the builtin functions available to scripts.
func callBuiltin(name string, args []any) (any, bool, error) {
	switch name {
	case "int":
		value, ok := toIntValue(argumentAt(args, 0))
		if !ok {
			return nil, true, newScriptError("value", "int() 无法转换: %v", argumentAt(args, 0))
		}
		return value, true, nil
	case "float":
		number, err := builtinFloat(argumentAt(args, 0))
		if err != nil {
			return nil, true, err
		}
		return number, true, nil
	case "str":
		return pyText(argumentAt(args, 0)), true, nil
	case "len":
		return builtinLength(argumentAt(args, 0)), true, nil
	case "abs":
		number, _ := toNumber(argumentAt(args, 0))
		if number < 0 {
			number = -number
		}
		return number, true, nil
	case "round":
		number, _ := toNumber(argumentAt(args, 0))
		return math.Round(number), true, nil
	case "min", "max":
		return builtinExtreme(name, args), true, nil
	case "range":
		return builtinRange(args), true, nil
	case "enumerate":
		return builtinEnumerate(args), true, nil
	case "zip":
		return builtinZip(args), true, nil
	case "sum":
		return builtinSum(argumentAt(args, 0), argumentAt(args, 1)), true, nil
	case "sorted":
		return builtinSorted(argumentAt(args, 0)), true, nil
	case "reversed":
		return builtinReversed(argumentAt(args, 0)), true, nil
	case "print":
		return nil, true, nil
	}
	return nil, false, nil
}

// builtinFloat parses numbers and numeric text like Python's float().
func builtinFloat(value any) (any, error) {
	if number, ok := toNumber(value); ok {
		return number, nil
	}
	text, ok := value.(string)
	if !ok {
		return nil, newScriptError("value", "float() 无法转换: %v", value)
	}
	number, err := strconv.ParseFloat(strings.TrimSpace(text), 64)
	if err != nil {
		return nil, newScriptError("value", "float() 无法转换: %v", value)
	}
	return number, nil
}

// builtinLength returns the length of a text, list or map.
func builtinLength(value any) any {
	switch typed := value.(type) {
	case string:
		return len([]rune(typed))
	case []any:
		return len(typed)
	case map[string]any:
		return len(typed)
	}
	return 0
}

// builtinExtreme implements min/max over the argument list.
func builtinExtreme(name string, args []any) any {
	if len(args) == 0 {
		return nil
	}
	best, ok := toNumber(args[0])
	if !ok {
		return nil
	}
	for _, argument := range args[1:] {
		number, numeric := toNumber(argument)
		if !numeric {
			return nil
		}
		if (name == "min" && number < best) || (name == "max" && number > best) {
			best = number
		}
	}
	return best
}

// argumentAt returns args[index] or nil when out of range.
func argumentAt(args []any, index int) any {
	if index < 0 || index >= len(args) {
		return nil
	}
	return args[index]
}

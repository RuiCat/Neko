// Strict operator semantics matching Python's type rules for arithmetic.
package function

// applyBinaryStrict applies an infix operator and rejects invalid operand types.
func applyBinaryStrict(operator string, left, right any) (any, error) {
	switch operator {
	case "+":
		if leftNumber, leftOK := toNumber(left); leftOK {
			if rightNumber, rightOK := toNumber(right); rightOK {
				if leftInt, ok := left.(int); ok {
					if rightInt, ok := right.(int); ok {
						return leftInt + rightInt, nil
					}
				}
				return leftNumber + rightNumber, nil
			}
		}
		if leftText, ok := left.(string); ok {
			if rightText, ok := right.(string); ok {
				return leftText + rightText, nil
			}
		}
		if leftList, ok := left.([]any); ok {
			if rightList, ok := right.([]any); ok {
				combined := append([]any{}, leftList...)
				combined = append(combined, rightList...)
				return combined, nil
			}
		}
		return nil, newScriptError("type", "不支持的类型组合: %T + %T", left, right)
	case "-", "*", "/", "%":
		if operator == "*" {
			if text, ok := left.(string); ok {
				if count, ok := toIntValue(right); ok {
					return repeatText(text, count), nil
				}
			}
			if list, ok := left.([]any); ok {
				if count, ok := toIntValue(right); ok {
					return repeatList(list, count), nil
				}
			}
		}
		leftNumber, leftOK := toNumber(left)
		rightNumber, rightOK := toNumber(right)
		if !leftOK || !rightOK {
			return nil, newScriptError("type", "不支持的类型组合: %T %s %T", left, operator, right)
		}
		if leftInt, ok := strictInt(left); ok {
			if rightInt, ok := strictInt(right); ok {
				switch operator {
				case "-":
					return leftInt - rightInt, nil
				case "*":
					return leftInt * rightInt, nil
				case "%":
					if rightInt == 0 {
						return nil, newScriptError("value", "整数取模除数为零")
					}
					return leftInt % rightInt, nil
				}
			}
		}
		value := applyBinary(operator, leftNumber, rightNumber)
		if value == nil {
			return nil, newScriptError("type", "运算失败: %v %s %v", left, operator, right)
		}
		return value, nil
	}
	return applyBinary(operator, left, right), nil
}

func repeatText(text string, count int) string {
	if count <= 0 {
		return ""
	}
	runes := []rune(text)
	result := make([]rune, 0, len(runes)*count)
	for index := 0; index < count; index++ {
		result = append(result, runes...)
	}
	return string(result)
}

func repeatList(items []any, count int) []any {
	if count <= 0 {
		return []any{}
	}
	result := make([]any, 0, len(items)*count)
	for index := 0; index < count; index++ {
		result = append(result, items...)
	}
	return result
}

// strictInt reports an integer value without accepting floats.
func strictInt(value any) (int, bool) {
	switch typed := value.(type) {
	case int:
		return typed, true
	case int64:
		return int(typed), true
	}
	return 0, false
}

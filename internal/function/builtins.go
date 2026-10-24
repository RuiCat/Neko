// Additional Python-style builtins available to .neko function bodies.
package function

import "sort"

const maxRangeItems = 100000

// builtinRange implements range(stop) / range(start, stop[, step]).
func builtinRange(args []any) any {
	start, stop, step := 0, 0, 1
	switch len(args) {
	case 0:
		return []any{}
	case 1:
		stop, _ = toIntValue(args[0])
	default:
		start, _ = toIntValue(args[0])
		stop, _ = toIntValue(args[1])
		if len(args) > 2 {
			step, _ = toIntValue(args[2])
		}
	}
	if step == 0 {
		return []any{}
	}
	items := []any{}
	for value := start; (step > 0 && value < stop) || (step < 0 && value > stop); value += step {
		if len(items) >= maxRangeItems {
			break
		}
		items = append(items, value)
	}
	return items
}

// builtinEnumerate implements enumerate(list[, start]).
func builtinEnumerate(args []any) any {
	items, _ := argumentAt(args, 0).([]any)
	start, _ := toIntValue(argumentAt(args, 1))
	result := make([]any, len(items))
	for index, item := range items {
		result[index] = []any{start + index, item}
	}
	return result
}

// builtinZip implements zip(a, b, ...) over the shortest input.
func builtinZip(args []any) any {
	if len(args) == 0 {
		return []any{}
	}
	lists := make([][]any, len(args))
	length := -1
	for index, argument := range args {
		items, ok := argument.([]any)
		if !ok {
			return []any{}
		}
		lists[index] = items
		if length < 0 || len(items) < length {
			length = len(items)
		}
	}
	result := make([]any, length)
	for row := 0; row < length; row++ {
		tuple := make([]any, len(lists))
		for column, items := range lists {
			tuple[column] = items[row]
		}
		result[row] = tuple
	}
	return result
}

// builtinSum implements sum(list[, start]).
func builtinSum(value, start any) any {
	items, ok := value.([]any)
	if !ok {
		return start
	}
	total := start
	if total == nil {
		total = 0
	}
	for _, item := range items {
		total = applyBinary("+", total, item)
	}
	return total
}

// builtinSorted returns a sorted copy of a list (texts/numbers ascending).
func builtinSorted(value any) any {
	items, ok := value.([]any)
	if !ok {
		return []any{}
	}
	copied := append([]any{}, items...)
	sort.SliceStable(copied, func(i, j int) bool {
		return compareValues(copied[i], copied[j], "<")
	})
	return copied
}

// builtinReversed returns a reversed copy of a list or text.
func builtinReversed(value any) any {
	switch typed := value.(type) {
	case []any:
		result := make([]any, len(typed))
		for index := range typed {
			result[index] = typed[len(typed)-1-index]
		}
		return result
	case string:
		runes := []rune(typed)
		for left, right := 0, len(runes)-1; left < right; left, right = left+1, right-1 {
			runes[left], runes[right] = runes[right], runes[left]
		}
		return string(runes)
	}
	return []any{}
}

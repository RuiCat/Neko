// Method calls on lists, texts and maps used by .neko function bodies.
package function

import (
	"fmt"
	"sort"
	"strings"
)

// evalMethodCall dispatches list/text/map methods.
func (r *runner) evalMethodCall(node *methodCallExpression) (any, error) {
	args := make([]any, 0, len(node.args))
	for _, argument := range node.args {
		value, err := r.eval(argument)
		if err != nil {
			return nil, err
		}
		args = append(args, value)
	}
	switch node.method {
	case "append", "extend", "pop", "popitem", "clear", "insert", "remove", "sort", "reverse", "update":
		return r.evalListMutation(node, args)
	}
	target, err := r.eval(node.target)
	if err != nil {
		return nil, err
	}
	if value, handled, err := methodTextCall(node.method, target, args); handled {
		return value, err
	}
	switch node.method {
	case "count":
		return methodCount(target, argumentAt(args, 0)), nil
	case "index":
		return methodIndex(target, argumentAt(args, 0)), nil
	}
	switch node.method {
	case "keys":
		return methodKeys(target), nil
	case "values":
		return methodValues(target), nil
	case "items":
		return methodItems(target), nil
	case "get":
		return methodGet(target, argumentAt(args, 0), argumentAt(args, 1)), nil
	case "copy":
		return methodCopy(target), nil
	case "setdefault":
		return methodSetDefault(target, argumentAt(args, 0), argumentAt(args, 1)), nil
	}
	return nil, fmt.Errorf("不支持的方法: %s", node.method)
}

func (r *runner) evalListMutation(node *methodCallExpression, args []any) (any, error) {
	path, ok := node.target.(*pathExpression)
	if !ok {
		return nil, fmt.Errorf("%s 只支持列表/映射路径", node.method)
	}
	current := r.getPath(path)
	switch node.method {
	case "clear":
		switch typed := current.(type) {
		case []any:
			_ = typed
			r.assignPath(path, []any{})
		case map[string]any:
			_ = typed
			r.assignPath(path, map[string]any{})
		default:
			return nil, nil
		}
		return nil, nil
	case "append", "extend":
		items, _ := current.([]any)
		copied := append([]any{}, items...)
		if node.method == "append" {
			if len(args) > 0 {
				copied = append(copied, args[0])
			}
		} else if extra, ok := argumentAt(args, 0).([]any); ok {
			copied = append(copied, extra...)
		}
		r.assignPath(path, copied)
		return nil, nil
	case "insert":
		items, _ := current.([]any)
		copied := append([]any{}, items...)
		index, _ := toIntValue(argumentAt(args, 0))
		if index < 0 {
			index += len(copied)
		}
		if index < 0 {
			index = 0
		}
		if index > len(copied) {
			index = len(copied)
		}
		copied = append(copied[:index], append([]any{argumentAt(args, 1)}, copied[index:]...)...)
		r.assignPath(path, copied)
		return nil, nil
	case "pop":
		if object, ok := current.(map[string]any); ok {
			key := pyText(argumentAt(args, 0))
			if value, exists := object[key]; exists {
				copied := map[string]any{}
				for name, item := range object {
					if name != key {
						copied[name] = item
					}
				}
				r.assignPath(path, copied)
				return value, nil
			}
			if len(args) > 1 {
				return args[1], nil
			}
			return nil, nil
		}
		items, _ := current.([]any)
		if len(items) == 0 {
			return nil, nil
		}
		index := len(items) - 1
		if len(args) > 0 {
			index, _ = toIntValue(args[0])
			if index < 0 {
				index += len(items)
			}
		}
		if index < 0 || index >= len(items) {
			return nil, nil
		}
		value := items[index]
		copied := append([]any{}, items[:index]...)
		copied = append(copied, items[index+1:]...)
		r.assignPath(path, copied)
		return value, nil
	case "sort":
		items, _ := current.([]any)
		copied := append([]any{}, items...)
		sort.SliceStable(copied, func(i, j int) bool {
			return compareValues(copied[i], copied[j], "<")
		})
		r.assignPath(path, copied)
		return nil, nil
	case "reverse":
		switch typed := current.(type) {
		case []any:
			copied := make([]any, len(typed))
			for index := range typed {
				copied[index] = typed[len(typed)-1-index]
			}
			r.assignPath(path, copied)
		case string:
			runes := []rune(typed)
			for left, right := 0, len(runes)-1; left < right; left, right = left+1, right-1 {
				runes[left], runes[right] = runes[right], runes[left]
			}
			r.assignPath(path, string(runes))
		}
		return nil, nil
	case "update":
		target, _ := current.(map[string]any)
		source, _ := argumentAt(args, 0).(map[string]any)
		merged := map[string]any{}
		for key, value := range target {
			merged[key] = value
		}
		for key, value := range source {
			merged[key] = value
		}
		r.assignPath(path, merged)
		return nil, nil
	case "popitem":
		object, _ := current.(map[string]any)
		if len(object) == 0 {
			return nil, nil
		}
		keys := make([]string, 0, len(object))
		for key := range object {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		key := keys[len(keys)-1]
		value := object[key]
		copied := map[string]any{}
		for name, item := range object {
			if name != key {
				copied[name] = item
			}
		}
		r.assignPath(path, copied)
		return []any{key, value}, nil
	case "remove":
		items, _ := current.([]any)
		needle := argumentAt(args, 0)
		for index, item := range items {
			if compareValues(item, needle, "==") {
				copied := append([]any{}, items[:index]...)
				copied = append(copied, items[index+1:]...)
				r.assignPath(path, copied)
				return nil, nil
			}
		}
	}
	return nil, nil
}

func methodCount(target, needle any) any {
	switch typed := target.(type) {
	case []any:
		count := 0
		for _, item := range typed {
			if compareValues(item, needle, "==") {
				count++
			}
		}
		return count
	case string:
		return strings.Count(typed, pyText(needle))
	}
	return 0
}

func methodIndex(target, needle any) any {
	switch typed := target.(type) {
	case []any:
		for index, item := range typed {
			if compareValues(item, needle, "==") {
				return index
			}
		}
	case string:
		index := strings.Index(typed, pyText(needle))
		if index < 0 {
			return -1
		}
		return len([]rune(typed[:index]))
	}
	return -1
}

func methodKeys(target any) any {
	object, ok := target.(map[string]any)
	if !ok {
		return []any{}
	}
	keys := make([]string, 0, len(object))
	for key := range object {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	items := make([]any, len(keys))
	for index, key := range keys {
		items[index] = key
	}
	return items
}

func methodValues(target any) any {
	object, ok := target.(map[string]any)
	if !ok {
		return []any{}
	}
	keys := methodKeys(target).([]any)
	items := make([]any, len(keys))
	for index, key := range keys {
		items[index] = object[pyText(key)]
	}
	return items
}

func methodItems(target any) any {
	object, ok := target.(map[string]any)
	if !ok {
		return []any{}
	}
	keys := methodKeys(target).([]any)
	items := make([]any, len(keys))
	for index, key := range keys {
		items[index] = []any{pyText(key), object[pyText(key)]}
	}
	return items
}

func methodGet(target, key, fallback any) any {
	object, ok := target.(map[string]any)
	if !ok {
		return fallback
	}
	if value, exists := object[pyText(key)]; exists {
		return value
	}
	return fallback
}

func methodCopy(target any) any {
	switch typed := target.(type) {
	case []any:
		return append([]any{}, typed...)
	case map[string]any:
		copied := make(map[string]any, len(typed))
		for key, value := range typed {
			copied[key] = value
		}
		return copied
	}
	return target
}

func methodSetDefault(target, key, fallback any) any {
	object, ok := target.(map[string]any)
	if !ok {
		return fallback
	}
	name := pyText(key)
	if value, exists := object[name]; exists {
		return value
	}
	object[name] = fallback
	return fallback
}

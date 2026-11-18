// pathValue.go holds the dotted-path read/write primitives EntityStore applies to
// its working copies. Their behavior mirrors entity.Entity.Get/Set, so a row
// validated on a copy resolves exactly like the row the entity would accept.
package host

import (
	"encoding/json"
	"fmt"
	"math"
	"strings"
)

// readPath resolves one dotted path through nested objects.
func readPath(data map[string]any, path string) (any, bool) {
	var current any = data
	for _, key := range strings.Split(path, ".") {
		object, ok := current.(map[string]any)
		if !ok {
			return nil, false
		}
		current, ok = object[key]
		if !ok {
			return nil, false
		}
	}
	return current, true
}

// writePath stores value at one dotted path and creates the intermediate
// objects, like entity.Set does. The empty path is refused so a batch can never
// write a nameless field.
func writePath(data map[string]any, path string, value any) error {
	if path == "" {
		return fmt.Errorf("empty write path")
	}
	parts := strings.Split(path, ".")
	current := data
	for _, key := range parts[:len(parts)-1] {
		child, ok := current[key].(map[string]any)
		if !ok {
			child = map[string]any{}
			current[key] = child
		}
		current = child
	}
	current[parts[len(parts)-1]] = value
	return nil
}

// asInt converts every scalar shape entity data can hold to int. A fractional,
// non-finite or non-numeric value reports false so a bad row stays visible
// instead of being silently coerced.
func asInt(raw any) (int, bool) {
	switch value := raw.(type) {
	case int:
		return value, true
	case int8:
		return int(value), true
	case int16:
		return int(value), true
	case int32:
		return int(value), true
	case int64:
		return int(value), true
	case uint:
		return int(value), true
	case uint8:
		return int(value), true
	case uint16:
		return int(value), true
	case uint32:
		return int(value), true
	case float32:
		return integralFloat(float64(value))
	case float64:
		return integralFloat(value)
	case json.Number:
		parsed, err := value.Int64()
		if err != nil {
			return 0, false
		}
		return int(parsed), true
	default:
		return 0, false
	}
}

// integralFloat accepts only finite integral values.
func integralFloat(value float64) (int, bool) {
	if math.IsNaN(value) || math.IsInf(value, 0) || math.Trunc(value) != value {
		return 0, false
	}
	return int(value), true
}

// intValue converts one scalar or reports a typed error naming the row.
func intValue(raw any, label string) (int, error) {
	value, ok := asInt(raw)
	if !ok {
		return 0, fmt.Errorf("%s: want integer, got %T", label, raw)
	}
	return value, nil
}

// cloneData deep-copies one entity map so a working copy never aliases the
// rollback snapshot. entity.Snapshot already returns a deep copy; this clone is
// what keeps before and work independent without reading the entity twice.
func cloneData(data map[string]any) map[string]any {
	copied := make(map[string]any, len(data))
	for key, value := range data {
		copied[key] = cloneValue(value)
	}
	return copied
}

// cloneValue deep-copies one value of the two containers entity data uses.
func cloneValue(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		return cloneData(typed)
	case []any:
		if typed == nil {
			return []any(nil)
		}
		copied := make([]any, len(typed))
		for index, item := range typed {
			copied[index] = cloneValue(item)
		}
		return copied
	default:
		return value
	}
}

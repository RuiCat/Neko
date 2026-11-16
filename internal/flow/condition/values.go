// values.go coerces snapshot scalars into ints. Entity data holds native ints
// from the runtime and float64/json.Number after a JSON round trip, so all
// three are accepted; fractional values are rejected instead of truncated.
package condition

import (
	"encoding/json"
	"fmt"
	"math"
)

// readInt reads one integer scalar snapshot path.
func readInt(snap Snapshot, path string) (int, error) {
	raw, err := snap.Get(path)
	if err != nil {
		return 0, err
	}
	value, ok := toInt(raw)
	if !ok {
		return 0, fmt.Errorf("%s: want integer, got %T", path, raw)
	}
	return value, nil
}

// toInt converts one snapshot scalar to int. Fractional, non-finite and
// non-numeric values return false so a bad snapshot stays visible.
func toInt(raw any) (int, bool) {
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
		return floatToInt(float64(value))
	case float64:
		return floatToInt(value)
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

// floatToInt accepts only finite integral values.
func floatToInt(value float64) (int, bool) {
	if math.IsNaN(value) || math.IsInf(value, 0) || math.Trunc(value) != value {
		return 0, false
	}
	return int(value), true
}

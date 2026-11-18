// values.go coerces JSON-decoded scalars to integers for set/add effects and
// item quantities. Entity data holds native Go ints in the runtime and json
// Number or float64 after a JSON round trip, so all three are accepted;
// fractional values are rejected instead of truncated.
package settle

import (
	"encoding/json"
	"fmt"
	"math"
)

// toInteger converts one scalar to int. Fractional, non-finite and non-numeric
// values return an error so a bad effect stays visible instead of being
// silently coerced.
func toInteger(raw any) (int, error) {
	switch value := raw.(type) {
	case int:
		return value, nil
	case int8:
		return int(value), nil
	case int16:
		return int(value), nil
	case int32:
		return int(value), nil
	case int64:
		return int(value), nil
	case uint:
		return int(value), nil
	case uint8:
		return int(value), nil
	case uint16:
		return int(value), nil
	case uint32:
		return int(value), nil
	case float32:
		return floatToInteger(float64(value))
	case float64:
		return floatToInteger(value)
	case json.Number:
		parsed, err := value.Int64()
		if err != nil {
			return 0, fmt.Errorf("want integer, got %q", value.String())
		}
		return int(parsed), nil
	default:
		return 0, fmt.Errorf("want integer, got %T", raw)
	}
}

// floatToInteger accepts only finite integral values.
func floatToInteger(value float64) (int, error) {
	if math.IsNaN(value) || math.IsInf(value, 0) || math.Trunc(value) != value {
		return 0, fmt.Errorf("want integer, got %v", value)
	}
	return int(value), nil
}

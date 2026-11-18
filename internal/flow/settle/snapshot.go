// snapshot.go adapts one entity map to the read-only condition.Snapshot the
// judge and settlement consume. It exposes 素材.物品 unchanged so both accepted
// item-view shapes (a list of records or a name-keyed map of records) decode
// identically downstream.
package settle

import (
	"fmt"
	"strings"

	"github.com/RuiCat/Neko/internal/flow/condition"
)

// Snapshot is the read-only condition.Snapshot adapter over one entity map.
// Get resolves a dotted path through nested objects; a flat dotted key wins
// when present, so runtime data and JSON-shaped data both work.
type Snapshot struct {
	data map[string]any
}

// NewSnapshot wraps one entity view. The map is not copied and settlement never
// mutates it; callers must not mutate it while a settlement is in flight.
func NewSnapshot(data map[string]any) Snapshot {
	return Snapshot{data: data}
}

// Get resolves one snapshot path. condition.PATH_ITEMS is returned verbatim
// (list or name-keyed map) for the item decoder.
func (s Snapshot) Get(path string) (any, error) {
	if path == "" {
		return nil, fmt.Errorf("empty snapshot path")
	}
	if s.data == nil {
		return nil, fmt.Errorf("%s: no entity data", path)
	}
	if raw, ok := s.data[path]; ok {
		return raw, nil
	}
	var current any = s.data
	for _, segment := range strings.Split(path, ".") {
		object, ok := current.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("%s: %q is not an object", path, segment)
		}
		value, ok := object[segment]
		if !ok {
			return nil, fmt.Errorf("%s: unknown path", path)
		}
		current = value
	}
	return current, nil
}

// Snapshot implements the K2 read-only view settlement is handed.
var _ condition.Snapshot = Snapshot{}

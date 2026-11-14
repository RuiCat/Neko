// seed.go records one entity's current leaf values as baseline events so a
// panel or a pre-step card can show long-lived fields that never change after
// creation. Seeded events never reach a delta push.
package observation

import (
	"encoding/json"
	"sort"
	"time"
)

// SeedEntity appends one baseline event per non-noise leaf path of an entity's
// state and returns how many it wrote.
func SeedEntity(bus *Bus, policy *Policy, entityID string, state map[string]any) int {
	if bus == nil || len(state) == 0 {
		return 0
	}
	paths := make([]string, 0, len(state))
	values := make(map[string]any, len(state))
	walkLeaves("", state, func(path string, value any) {
		if path == "" || policy.TierOf(path) == TierNoise {
			return
		}
		paths = append(paths, path)
		values[path] = value
	})
	sort.Strings(paths)
	stamp := time.Now().UTC().Format(time.RFC3339)
	scope := ScopeForEntity(entityID)
	events := make([]Event, 0, len(paths))
	for _, path := range paths {
		events = append(events, Event{
			Scope:  scope,
			Path:   path,
			To:     values[path],
			TickID: "seed",
			At:     stamp,
			Seeded: true,
		})
	}
	bus.AppendAll(events)
	return len(events)
}

// walkLeaves visits every leaf of a nested state map, joining keys with dots.
// Arrays and scalars are leaves; an empty map contributes nothing.
func walkLeaves(prefix string, node map[string]any, emit func(path string, value any)) {
	for key, value := range node {
		path := key
		if prefix != "" {
			path = prefix + "." + key
		}
		if nested, ok := value.(map[string]any); ok {
			walkLeaves(path, nested, emit)
			continue
		}
		emit(path, normalizeValue(value))
	}
}

// normalizeValue turns a decoded JSON value into the shape the control plane
// serializes back, so a seeded baseline and a live write compare equal.
func normalizeValue(value any) any {
	raw, err := json.Marshal(value)
	if err != nil {
		return value
	}
	var decoded any
	if err := json.Unmarshal(raw, &decoded); err != nil {
		return value
	}
	return decoded
}

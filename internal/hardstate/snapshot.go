// snapshot.go captures the hard-state subset of every entity and hashes it
// canonically, so the same state always yields the same digest and any change
// yields a different one.
package hardstate

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// Snapshot is one turn-level capture of the hard state. State is keyed by
// "<entityID>.<dotted path>" (for example "猫猫.状态.情绪") so every value is
// attributable without a second lookup.
type Snapshot struct {
	SnapshotID string         `json:"snapshot_id"`
	TurnID     string         `json:"turn_id"`
	TickID     string         `json:"tick_id,omitempty"`
	TakenAt    time.Time      `json:"taken_at"`
	Digest     string         `json:"digest"`
	State      map[string]any `json:"state"`
}

// IDs groups the identifiers that travel with one capture. SnapshotID is the
// authoritative anchor used by journal records and compensation packages.
type IDs struct {
	SnapshotID string
	TurnID     string
	TickID     string
}

// Capture flattens the hard-state subset of every selected entity and returns a
// sorted, digest-stable snapshot: the configured state group plus the
// field-level paths (C0 v2). entityData maps entity ID to its full data map
// (the caller takes a copy of each entity first). Arrays and scalars are kept
// as values; nested maps are flattened into dotted paths.
func Capture(entityData map[string]map[string]any, schema Schema, ids IDs, now time.Time) (Snapshot, error) {
	if err := schema.Validate(); err != nil {
		return Snapshot{}, err
	}
	if ids.SnapshotID == "" {
		return Snapshot{}, fmt.Errorf("hardstate: 快照 ID 不能为空")
	}
	state := map[string]any{}
	for entityID, data := range entityData {
		if !schema.includes(entityID) {
			continue
		}
		if group, ok := data[schema.Group]; ok {
			flattenInto(state, entityID+"."+schema.Group, group)
		}
		for _, path := range schema.Paths {
			value, found := lookupPath(data, path)
			if !found {
				continue
			}
			flattenInto(state, entityID+"."+path, value)
		}
	}
	digest, err := DigestOf(state)
	if err != nil {
		return Snapshot{}, err
	}
	return Snapshot{
		SnapshotID: ids.SnapshotID,
		TurnID:     ids.TurnID,
		TickID:     ids.TickID,
		TakenAt:    now,
		Digest:     digest,
		State:      state,
	}, nil
}

// DigestOf hashes a canonical JSON encoding of state. encoding/json sorts map
// keys, so the digest is independent of iteration order; numbers are normalised
// by the encoder (1 and 1.0 both render as 1).
func DigestOf(state map[string]any) (string, error) {
	raw, err := json.Marshal(state)
	if err != nil {
		return "", fmt.Errorf("hardstate: 规范化状态失败: %w", err)
	}
	sum := sha256.Sum256(raw)
	return "sha256:" + hex.EncodeToString(sum[:]), nil
}

// SameValue reports whether two state values are JSON-equivalent, so a value
// read back from disk compares equal to the in-memory original.
func SameValue(a, b any) bool {
	left, leftErr := json.Marshal(a)
	right, rightErr := json.Marshal(b)
	return leftErr == nil && rightErr == nil && string(left) == string(right)
}

// lookupPath resolves one dotted path inside entity data.
func lookupPath(data map[string]any, path string) (any, bool) {
	var current any = data
	for _, part := range strings.Split(path, ".") {
		mapping, ok := current.(map[string]any)
		if !ok {
			return nil, false
		}
		current, ok = mapping[part]
		if !ok {
			return nil, false
		}
	}
	return current, true
}

// flattenInto writes value into target under prefix, recursing into nested
// maps. A nil value is kept so a cleared field still differs from a missing one.
func flattenInto(target map[string]any, prefix string, value any) {
	switch typed := value.(type) {
	case map[string]any:
		for key, child := range typed {
			flattenInto(target, prefix+"."+key, child)
		}
	default:
		target[prefix] = value
	}
}

// validate.go is the fail-closed package checker: every state value must come
// from the same snapshot, and every memory fragment must be reconstructible.
package compensation

import (
	"fmt"

	"github.com/RuiCat/Neko/internal/hardstate"
	"github.com/RuiCat/Neko/internal/journal"
)

// Validate checks a package against the snapshot it claims to derive from.
func Validate(pkg journal.Compensation, snapshot hardstate.Snapshot) error {
	if pkg.SnapshotID == "" || pkg.SnapshotID != snapshot.SnapshotID {
		return fmt.Errorf("compensation: 包 %q 与快照 %q 不匹配", pkg.SnapshotID, snapshot.SnapshotID)
	}
	if pkg.TurnID != "" && pkg.TurnID != snapshot.TurnID {
		return fmt.Errorf("compensation: turn %q 与快照 %q 不匹配", pkg.TurnID, snapshot.TurnID)
	}
	if pkg.StateSource != STATE_SOURCE {
		return fmt.Errorf("compensation: state_source 必须是 %q，得到 %q", STATE_SOURCE, pkg.StateSource)
	}
	for key, value := range pkg.State {
		snapshotValue, ok := snapshot.State[key]
		if !ok {
			return fmt.Errorf("compensation: 状态字段 %q 不在快照中", key)
		}
		if !hardstate.SameValue(value, snapshotValue) {
			return fmt.Errorf("compensation: 状态字段 %q 与快照不一致", key)
		}
	}
	retrievedWeights := map[string]float64{}
	for _, fragment := range pkg.Memory {
		if fragment.SourceSnapshotID == "" {
			return fmt.Errorf("compensation: 记忆片段缺少 source_snapshot_id: %q", fragment.Content)
		}
		if fragment.Type != "retrieved" && fragment.Type != "reconstructed" {
			return fmt.Errorf("compensation: 记忆片段类型非法: %q", fragment.Type)
		}
		if fragment.Weight < 0 || fragment.Weight > 1 {
			return fmt.Errorf("compensation: 记忆片段权重 %v 超出 [0,1]", fragment.Weight)
		}
		if fragment.Type == "retrieved" {
			current, seen := retrievedWeights[fragment.SourceSnapshotID]
			if !seen || fragment.Weight < current {
				retrievedWeights[fragment.SourceSnapshotID] = fragment.Weight
			}
		}
	}
	for _, fragment := range pkg.Memory {
		if fragment.Type != "reconstructed" {
			continue
		}
		if retrieved, ok := retrievedWeights[fragment.SourceSnapshotID]; ok && fragment.Weight >= retrieved {
			return fmt.Errorf("compensation: reconstructed 片段权重 %v 不低于同源 retrieved %v", fragment.Weight, retrieved)
		}
	}
	return nil
}

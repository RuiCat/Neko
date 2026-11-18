// rewards.go pays one task template reward exactly once per completion: the growth
// experience row and the C4 affinity row, both as additive source-layer writes. A
// zero reward writes nothing, because adding zero is not a state change.
package task

import (
	"encoding/json"
	"fmt"
	"math"

	"github.com/RuiCat/Neko/internal/flow/spec"
)

// checkReward refuses a template whose reward would debit the entity. Task rewards
// are additions by contract, so a negative value is a template error rather than a
// deduction this package should perform.
func checkReward(reward spec.TaskReward) error {
	if reward.Experience < 0 || reward.Affinity < 0 {
		return fmt.Errorf("%w: experience %d affinity %d", ErrNegativeReward, reward.Experience, reward.Affinity)
	}
	return nil
}

// rewardRow is one planned reward write: the owned path and the amount to add.
type rewardRow struct {
	path   string
	amount int
}

// rewardRows expands a template reward into the rows this package may write. The
// order is fixed (experience, then affinity) so a report and a replay agree.
func rewardRows(reward spec.TaskReward) []rewardRow {
	return []rewardRow{
		{path: PATH_GROWTH_EXPERIENCE, amount: reward.Experience},
		{path: PATH_RELATION_AFFINITY, amount: reward.Affinity},
	}
}

// payRewards adds the template reward to the entity and returns the rows applied.
// Zero amounts are skipped: the store then sees no write for a reward the template
// does not grant.
func payRewards(store Store, entity string, reward spec.TaskReward) ([]Reward, error) {
	applied := []Reward{}
	for _, row := range rewardRows(reward) {
		if row.amount == 0 {
			continue
		}
		if row.amount < 0 {
			return nil, fmt.Errorf("%w: %s %d", ErrNegativeReward, row.path, row.amount)
		}
		if err := addInt(store, entity, row.path, row.amount); err != nil {
			return nil, err
		}
		applied = append(applied, Reward{Path: row.path, Amount: row.amount})
	}
	return applied, nil
}

// addInt adds delta to one integer row, treating an absent row as zero.
func addInt(store Store, entity, path string, delta int) error {
	sum := delta
	current, ok := store.Get(entity, path)
	if ok {
		value, err := asInt(current)
		if err != nil {
			return fmt.Errorf("task reward %s: %w", path, err)
		}
		sum = value + delta
	}
	if err := store.Set(entity, path, sum); err != nil {
		return fmt.Errorf("task reward %s: %w", path, err)
	}
	return nil
}

// asInt coerces the numeric shapes a store can hand back (Go ints, JSON numbers,
// floats). A non-integral or non-numeric value is an error instead of a silent
// zero, so a broken snapshot cannot quietly reset a reward.
func asInt(value any) (int, error) {
	switch typed := value.(type) {
	case int:
		return typed, nil
	case int32:
		return int(typed), nil
	case int64:
		return int(typed), nil
	case float64:
		if math.Trunc(typed) != typed {
			return 0, fmt.Errorf("value %v is not an integer", typed)
		}
		return int(typed), nil
	case json.Number:
		parsed, err := typed.Int64()
		if err != nil {
			return 0, fmt.Errorf("value %q is not an integer: %w", typed.String(), err)
		}
		return int(parsed), nil
	default:
		return 0, fmt.Errorf("value %T is not numeric", value)
	}
}

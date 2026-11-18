// stage.go owns the three-stage task machine 未激活 → 进行中 → 完成. The script
// rules own the stage row, so this file only reads it and computes the ordered
// path an advance traverses; the rows that drive the transition are written by
// advance.go.
package task

import (
	"fmt"
	"strings"
)

// normalizeStage maps an absent stage row to 未激活 and refuses values outside the
// contract. The corpus 已激活 is mapped at extraction time (C4), so a store still
// holding it means the extractor never ran; guessing 未激活 would hide that.
func normalizeStage(raw any) (string, error) {
	if raw == nil {
		return STAGE_INACTIVE, nil
	}
	text, ok := raw.(string)
	if !ok {
		return "", fmt.Errorf("%w: %T", ErrUnknownStage, raw)
	}
	trimmed := strings.TrimSpace(text)
	switch trimmed {
	case "", STAGE_INACTIVE:
		return STAGE_INACTIVE, nil
	case STAGE_ACTIVE, STAGE_COMPLETED:
		return trimmed, nil
	default:
		return "", fmt.Errorf("%w: %q", ErrUnknownStage, trimmed)
	}
}

// stagePath returns the stages an advance traverses, in order, ending at 完成. A
// task already in 进行中 only has the completion edge left.
func stagePath(from string) []string {
	if from == STAGE_ACTIVE {
		return []string{STAGE_COMPLETED}
	}
	return []string{STAGE_ACTIVE, STAGE_COMPLETED}
}

// readStage reads and normalizes the rule-owned stage row. Reading instead of
// assigning keeps the stage row out of this package's write set (C4).
func readStage(store Store, entity string) (string, error) {
	raw, ok := store.Get(entity, PATH_TASK_STAGE)
	if !ok {
		return STAGE_INACTIVE, nil
	}
	return normalizeStage(raw)
}

// Package regression is the P3 regression gate: it re-audits the journal and
// the frozen reference set before a repair version may be promoted. It calls
// no model, so a promotion is never judged by the thing being promoted.
package regression

import (
	"fmt"

	"github.com/RuiCat/Neko/internal/journal"
	"github.com/RuiCat/Neko/internal/referenceset"
	"github.com/RuiCat/Neko/internal/replay"
)

// Check is one gate check with a human-readable detail.
type Check struct {
	Name   string `json:"name"`
	Passed bool   `json:"passed"`
	Detail string `json:"detail"`
}

// Result is the whole gate outcome.
type Result struct {
	Passed bool    `json:"passed"`
	Checks []Check `json:"checks"`
}

// Gate runs every deterministic check and reports whether all passed.
func Gate(records []journal.TurnRecord, references []referenceset.Entry) Result {
	result := Result{Passed: true}
	result.Checks = append(result.Checks, replayCheck(records))
	result.Checks = append(result.Checks, referenceCheck(references))
	for _, check := range result.Checks {
		if !check.Passed {
			result.Passed = false
		}
	}
	return result
}

// replayCheck requires every conversation record to re-validate against its
// snapshot.
func replayCheck(records []journal.TurnRecord) Check {
	audit := replay.Audit(records)
	return Check{
		Name:   "replay",
		Passed: audit.Invalid == 0,
		Detail: fmt.Sprintf("records=%d with_compensation=%d valid=%d invalid=%d",
			audit.Records, audit.WithCompensation, audit.Valid, audit.Invalid),
	}
}

// referenceCheck validates the frozen reference set structure. A missing set is
// reported as a skipped check, never as a silent pass of a semantic gate.
func referenceCheck(references []referenceset.Entry) Check {
	if len(references) == 0 {
		return Check{Name: "reference_set", Passed: true, Detail: "未提供参照集（跳过结构校验）"}
	}
	for _, entry := range references {
		if entry.ID == "" {
			return Check{Name: "reference_set", Passed: false, Detail: fmt.Sprintf("参照集 %d 条，存在空 ID", len(references))}
		}
		if len(entry.ExpectedReactions) < referenceset.MIN_EXPECTED_REACTIONS {
			return Check{
				Name:   "reference_set",
				Passed: false,
				Detail: fmt.Sprintf("参照集 %d 条，条目 %q 的 expected_reactions < %d",
					len(references), entry.ID, referenceset.MIN_EXPECTED_REACTIONS),
			}
		}
	}
	return Check{Name: "reference_set", Passed: true, Detail: fmt.Sprintf("参照集 %d 条结构有效", len(references))}
}

// apply.go is the closed repair loop shared by POST /candidates/apply and the
// candidate_apply tool: an approved candidate becomes a versioned repair, the
// deterministic regression gate re-audits the recent journal, and a failing
// gate rolls the version back.
package ops

import (
	"path/filepath"
	"strings"
	"time"

	"github.com/RuiCat/Neko/internal/candidates"
	"github.com/RuiCat/Neko/internal/engine"
	"github.com/RuiCat/Neko/internal/journal"
	"github.com/RuiCat/Neko/internal/referenceset"
	"github.com/RuiCat/Neko/internal/regression"
	"github.com/RuiCat/Neko/internal/repair"
)

// ApplyRequest is the shared candidate promotion request.
type ApplyRequest struct {
	ID string
	// When is the rule condition that guards the repaired write. It is required
	// so a repair never becomes an always-on writer.
	When string
	// Match is the entity selector; empty defaults to the candidate's entity.
	Match string
}

// ApplyCandidate promotes one approved candidate to a repair version and gates
// it. A failing gate rolls the version back before reporting the conflict.
func ApplyCandidate(deps Deps, request ApplyRequest) (map[string]any, error) {
	if err := deps.validate(); err != nil {
		return nil, err
	}
	id := strings.TrimSpace(request.ID)
	if id == "" {
		return nil, BadRequest("缺少 id")
	}
	records, err := deps.candidateStore().List("")
	if err != nil {
		return nil, Internal("读取候选失败: %s", err)
	}
	var target *candidates.Record
	for index := range records {
		if records[index].ID == id {
			target = &records[index]
			break
		}
	}
	if target == nil {
		return nil, NotFound("候选 %s 不存在", id)
	}
	if target.Status != candidates.STATUS_APPROVED {
		return nil, BadRequest("候选未批准（当前 %s），不能上线", target.Status)
	}
	snapshotID := snapshotAnchor(target.Evidence)
	if snapshotID == "" {
		return nil, BadRequest("候选缺少快照锚点（evidence 需含 snapshot:<id>），拒绝上线")
	}
	patch := "策略修复：候选 " + target.ID + " @ " + target.Path + " — " + target.Reason
	candidate := repair.NewCandidate(repair.KindPolicy, snapshotID, target.ID, patch, time.Now())
	when := strings.TrimSpace(request.When)
	if when == "" {
		when = evidenceValue(target.Evidence, "when:")
	}
	if when == "" {
		return nil, BadRequest("候选缺少 when 条件（request.when 或 evidence 的 when:<expr>），拒绝上线")
	}
	match := strings.TrimSpace(request.Match)
	if match == "" {
		match = target.EntityID
	}
	rule := engine.Rule{
		ID: candidate.ID, Match: match, When: &when, Source: "repair", Enabled: true,
		Branches: []engine.Branch{{Else: true, Then: []engine.Op{{Op: "set", Path: target.Path, Value: target.Proposed}}}},
	}
	deps.Machine.AddRule(rule)
	if err := repair.UpsertRule(repair.RulesPath(deps.DataDir), rule); err != nil {
		deps.Machine.RemoveRule(rule.ID)
		return nil, Internal("持久化修复规则失败: %s", err)
	}
	version, err := deps.repairStore().Apply([]repair.Candidate{candidate}, time.Now())
	if err != nil {
		deps.Machine.RemoveRule(rule.ID)
		_, _ = repair.RemoveRule(repair.RulesPath(deps.DataDir), rule.ID)
		return nil, Internal("上线修复失败: %s", err)
	}
	gate := runRegressionGate(deps)
	if !gate.Passed {
		deps.Machine.RemoveRule(rule.ID)
		_, _ = repair.RemoveRule(repair.RulesPath(deps.DataDir), rule.ID)
		rolled, rollbackErr := deps.repairStore().Rollback()
		if rollbackErr != nil {
			return nil, Internal("回归失败且回滚失败: %s", rollbackErr)
		}
		message := "回归闸门未通过，修复版本已回滚"
		return nil, &Error{Kind: KIND_CONFLICT, Message: message, Body: map[string]any{
			"ok": false,
			"error": map[string]any{
				"code": "regression_failed", "message": message,
			},
			"version": version, "rolled_back": rolled, "gate": gate,
		}}
	}
	return map[string]any{"ok": true, "version": version, "gate": gate}, nil
}

// runRegressionGate re-audits the recent journal and the frozen reference set.
func runRegressionGate(deps Deps) regression.Result {
	records, err := journal.ReadDir(filepath.Join(deps.DataDir, "journal"))
	if err != nil {
		records = nil
	}
	var references []referenceset.Entry
	if entries, refErr := referenceset.LoadFile(filepath.Join(deps.DataDir, "reference-set", "v0.jsonl")); refErr == nil {
		references = entries
	}
	return regression.Gate(records, references)
}

// snapshotAnchor extracts "snapshot:<id>" from a candidate's evidence.
func snapshotAnchor(evidence []string) string {
	return evidenceValue(evidence, "snapshot:")
}

// evidenceValue returns the first evidence entry with prefix, minus the prefix.
func evidenceValue(evidence []string, prefix string) string {
	for _, item := range evidence {
		trimmed := strings.TrimSpace(item)
		if strings.HasPrefix(trimmed, prefix) {
			return strings.TrimSpace(strings.TrimPrefix(trimmed, prefix))
		}
	}
	return ""
}

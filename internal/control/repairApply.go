// repairApply.go is the closed loop: an approved candidate becomes a versioned
// repair, the deterministic regression gate re-audits the recent journal, and a
// failing gate rolls the version back.
package control

import (
	"bytes"
	"encoding/json"
	"net/http"
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

// applyCandidateRequest is the POST /candidates/apply body.
type applyCandidateRequest struct {
	ID string `json:"id"`
	// When is the rule condition that guards the repaired write. It is required
	// so a repair never becomes an always-on writer.
	When string `json:"when,omitempty"`
	// Match is the entity selector; empty defaults to the candidate's entity.
	Match string `json:"match,omitempty"`
}

// repairStore returns the shared repair version file under the data directory.
func (s *Server) repairStore() *repair.Store {
	return repair.NewStore(filepath.Join(s.dataDir, "repairs", "versions.json"))
}

// handleApplyCandidate promotes one approved candidate to a repair version and
// gates it. A failing gate rolls the version back before answering.
func (s *Server) handleApplyCandidate(r *http.Request) response {
	raw, err := readBody(r)
	if err != nil {
		return badRequest(err.Error())
	}
	request := applyCandidateRequest{}
	if len(bytes.TrimSpace(raw)) > 0 {
		if err := json.Unmarshal(raw, &request); err != nil {
			return badRequest("请求体不是合法 JSON: " + err.Error())
		}
	}
	id := strings.TrimSpace(request.ID)
	if id == "" {
		return badRequest("缺少 id")
	}
	records, err := s.candidateStore().List("")
	if err != nil {
		return internalError("读取候选失败: " + err.Error())
	}
	var target *candidates.Record
	for index := range records {
		if records[index].ID == id {
			target = &records[index]
			break
		}
	}
	if target == nil {
		return notFound("候选 " + id + " 不存在")
	}
	if target.Status != candidates.STATUS_APPROVED {
		return badRequest("候选未批准（当前 " + target.Status + "），不能上线")
	}
	snapshotID := snapshotAnchor(target.Evidence)
	if snapshotID == "" {
		return badRequest("候选缺少快照锚点（evidence 需含 snapshot:<id>），拒绝上线")
	}
	patch := "策略修复：候选 " + target.ID + " @ " + target.Path + " — " + target.Reason
	candidate := repair.NewCandidate(repair.KindPolicy, snapshotID, target.ID, patch, time.Now())
	when := strings.TrimSpace(request.When)
	if when == "" {
		when = evidenceValue(target.Evidence, "when:")
	}
	if when == "" {
		return badRequest("候选缺少 when 条件（request.when 或 evidence 的 when:<expr>），拒绝上线")
	}
	match := strings.TrimSpace(request.Match)
	if match == "" {
		match = target.EntityID
	}
	rule := engine.Rule{
		ID: candidate.ID, Match: match, When: &when, Source: "repair", Enabled: true,
		Branches: []engine.Branch{{Else: true, Then: []engine.Op{{Op: "set", Path: target.Path, Value: target.Proposed}}}},
	}
	s.runtime.Machine.AddRule(rule)
	if err := repair.UpsertRule(repair.RulesPath(s.dataDir), rule); err != nil {
		s.runtime.Machine.RemoveRule(rule.ID)
		return internalError("持久化修复规则失败: " + err.Error())
	}
	version, err := s.repairStore().Apply([]repair.Candidate{candidate}, time.Now())
	if err != nil {
		s.runtime.Machine.RemoveRule(rule.ID)
		_, _ = repair.RemoveRule(repair.RulesPath(s.dataDir), rule.ID)
		return internalError("上线修复失败: " + err.Error())
	}
	gate := s.runRegressionGate()
	if !gate.Passed {
		s.runtime.Machine.RemoveRule(rule.ID)
		_, _ = repair.RemoveRule(repair.RulesPath(s.dataDir), rule.ID)
		rolled, rollbackErr := s.repairStore().Rollback()
		if rollbackErr != nil {
			return internalError("回归失败且回滚失败: " + rollbackErr.Error())
		}
		return response{status: http.StatusConflict, body: map[string]any{
			"ok": false,
			"error": map[string]any{
				"code": "regression_failed", "message": "回归闸门未通过，修复版本已回滚",
			},
			"version": version, "rolled_back": rolled, "gate": gate,
		}}
	}
	return response{status: http.StatusOK, body: map[string]any{
		"ok": true, "version": version, "gate": gate,
	}}
}

// handleRollbackRepair rolls back the newest live repair version and removes
// the engine rules that version installed.
func (s *Server) handleRollbackRepair(r *http.Request) response {
	version, err := s.repairStore().Rollback()
	if err != nil {
		return internalError(err.Error())
	}
	for _, ruleID := range version.Applied {
		s.runtime.Machine.RemoveRule(ruleID)
		_, _ = repair.RemoveRule(repair.RulesPath(s.dataDir), ruleID)
	}
	return response{status: http.StatusOK, body: map[string]any{"ok": true, "rolled_back": version}}
}

// runRegressionGate re-audits the recent journal and the frozen reference set.
func (s *Server) runRegressionGate() regression.Result {
	records, err := journal.ReadDir(filepath.Join(s.dataDir, "journal"))
	if err != nil {
		records = nil
	}
	var references []referenceset.Entry
	if entries, refErr := referenceset.LoadFile(filepath.Join(s.dataDir, "reference-set", "v0.jsonl")); refErr == nil {
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

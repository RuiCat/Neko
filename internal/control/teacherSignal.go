// teacherSignal.go receives the C7 teacher signal: a semantic gate hint, affect
// deltas, transition candidates and an optional rebuild request. A signal is
// not truth: every candidate goes through the read-only C3 judge and only
// `uncovered` ones enter the review queue. Nothing is applied here.
package control

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/RuiCat/Neko/internal/candidates"
	"github.com/RuiCat/Neko/internal/diagnostics"
	"github.com/RuiCat/Neko/internal/engine"
)

// teacherSignal is the POST /teacher-signal body (C7).
type teacherSignal struct {
	SemanticGate         string             `json:"semantic_gate,omitempty"`
	AffectDelta          map[string]float64 `json:"affect_delta,omitempty"`
	TransitionCandidates []engine.Candidate `json:"transition_candidates,omitempty"`
	Rebuild              *teacherRebuild    `json:"rebuild,omitempty"`
}

// teacherRebuild is the optional compensation rebuild request carried by a
// signal. v1 records it; it never bypasses the snapshot/projection contract.
type teacherRebuild struct {
	SnapshotID string   `json:"snapshot_id,omitempty"`
	Memory     []string `json:"memory,omitempty"`
	Note       string   `json:"note,omitempty"`
}

// handleTeacherSignal judges every candidate and queues the uncovered ones.
func (s *Server) handleTeacherSignal(r *http.Request) response {
	raw, err := readBody(r)
	if err != nil {
		return badRequest(err.Error())
	}
	signal := teacherSignal{}
	if err := json.Unmarshal(raw, &signal); err != nil {
		s.recordTeacherMissing("请求体不是合法 JSON: " + err.Error())
		return badRequest("请求体不是合法 JSON: " + err.Error())
	}
	queue := append([]engine.Candidate{}, signal.TransitionCandidates...)
	for path, delta := range signal.AffectDelta {
		if delta == 0 || strings.TrimSpace(path) == "" {
			continue
		}
		queue = append(queue, engine.Candidate{
			EntityID: s.entityID, Path: path, Proposed: delta,
			Source: "teacher-affect", Evidence: []string{fmt.Sprintf("affect_delta=%g", delta)},
		})
	}
	legal, illegal, uncovered, queued := 0, 0, 0, 0
	for _, candidate := range queue {
		verdict, err := s.runtime.Machine.Judge(candidate)
		if err != nil {
			return internalError("裁决失败: " + err.Error())
		}
		switch verdict.Status {
		case engine.VERDICT_LEGAL:
			legal++
		case engine.VERDICT_ILLEGAL:
			illegal++
			s.recordTeacherRejected(candidate, verdict.Reason)
		case engine.VERDICT_UNCOVERED:
			uncovered++
			if _, err := s.candidateStore().Append(candidates.Record{
				EntityID: candidate.EntityID, Path: candidate.Path,
				Proposed: candidate.Proposed, Source: candidate.Source,
				Evidence: candidate.Evidence, Reason: verdict.Reason,
			}); err != nil {
				return internalError("候选入队失败: " + err.Error())
			}
			queued++
		}
	}
	return response{status: http.StatusOK, body: map[string]any{
		"ok":            true,
		"semantic_gate": signal.SemanticGate,
		"candidates":    len(queue),
		"legal":         legal,
		"illegal":       illegal,
		"uncovered":     uncovered,
		"queued":        queued,
		"rebuild":       signal.Rebuild != nil,
	}}
}

// recordTeacherMissing records a malformed or missing teacher signal without
// blocking a turn.
func (s *Server) recordTeacherMissing(detail string) {
	if s.runtime.Diagnostics == nil {
		return
	}
	s.runtime.Diagnostics.Add(diagnostics.Record{
		Code: "teacher_signal_missing", Severity: diagnostics.SEVERITY_WARNING,
		Source: "control", Message: detail,
	})
}

// recordTeacherRejected records a candidate the judge classified as illegal.
func (s *Server) recordTeacherRejected(candidate engine.Candidate, reason string) {
	if s.runtime.Diagnostics == nil {
		return
	}
	s.runtime.Diagnostics.Add(diagnostics.Record{
		Code: "teacher_signal_rejected", Severity: diagnostics.SEVERITY_INFO,
		Source: "control", Entity: candidate.EntityID, Type: "candidate",
		Message: "教师信号候选被判为 illegal: " + reason,
	})
}

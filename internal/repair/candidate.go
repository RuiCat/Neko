// candidate.go turns repairable triage findings into versioned repair
// candidates. Preference samples and findings without a snapshot never become
// candidates (constraint #9 and the plan's "no snapshot, no repair").
package repair

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/RuiCat/Neko/internal/triage"
)

// Kind is the layer a repair touches. Weight training is explicitly out of
// scope; every patch is a rule, policy or prompt version bump.
type Kind string

// The three repair layers.
const (
	KindRule   Kind = "rule"
	KindPolicy Kind = "policy"
	KindPrompt Kind = "prompt"
)

// Candidate is one proposed repair with a traceable negative example.
type Candidate struct {
	ID               string    `json:"id"`
	Kind             Kind      `json:"kind"`
	SourceSnapshotID string    `json:"source_snapshot_id"`
	NegativeTurnID   string    `json:"negative_turn_id"`
	Patch            string    `json:"patch"`
	Hash             string    `json:"hash"`
	CreatedAt        time.Time `json:"created_at"`
}

// Rejection records why one finding stayed out of the repair queue.
type Rejection struct {
	TurnID string `json:"turn_id"`
	Reason string `json:"reason"`
}

// Build maps findings to candidates. Consistency drift is repaired at the
// policy layer, wrong answers at the rule layer; preference samples and
// snapshot-less findings are rejected.
func Build(findings []triage.Finding, now time.Time) ([]Candidate, []Rejection) {
	candidates := []Candidate{}
	rejections := []Rejection{}
	for _, finding := range findings {
		if finding.Class == triage.ClassPreference {
			rejections = append(rejections, Rejection{TurnID: finding.TurnID, Reason: "偏好样本不进修复队列"})
			continue
		}
		if finding.SnapshotID == "" {
			rejections = append(rejections, Rejection{TurnID: finding.TurnID, Reason: "无快照来源，拒收"})
			continue
		}
		kind := KindPolicy
		if finding.Class == triage.ClassFact {
			kind = KindRule
		}
		patch := fmt.Sprintf("%s 修复：%s @ %s — %s", kind, finding.Kind, finding.TurnID, finding.Detail)
		candidates = append(candidates, NewCandidate(kind, finding.SnapshotID, finding.TurnID, patch, now))
	}
	return candidates, rejections
}

// NewCandidate builds one candidate with a stable ID and hash. It is shared by
// the triage-driven Build and the apply executor so an approved queue entry and
// a scanned finding produce the same identity.
func NewCandidate(kind Kind, snapshotID, turnID, patch string, now time.Time) Candidate {
	hash := shortHash(fmt.Sprintf("%s|%s|%s|%s", kind, snapshotID, turnID, patch))
	return Candidate{
		ID:               "repair_" + hash,
		Kind:             kind,
		SourceSnapshotID: snapshotID,
		NegativeTurnID:   turnID,
		Patch:            patch,
		Hash:             hash,
		CreatedAt:        now,
	}
}

// shortHash returns the first 16 hex characters of the SHA-256 of text.
func shortHash(text string) string {
	sum := sha256.Sum256([]byte(text))
	return hex.EncodeToString(sum[:])[:16]
}

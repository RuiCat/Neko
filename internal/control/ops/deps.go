// Package ops holds the operational control-plane actions shared by the HTTP
// control plane and the MCP tools, so nightly decay, judging, the candidate
// queue and the repair loop have exactly one implementation.
package ops

import (
	"fmt"
	"path/filepath"
	"time"

	"github.com/RuiCat/Neko/internal/candidates"
	"github.com/RuiCat/Neko/internal/engine"
	"github.com/RuiCat/Neko/internal/entity"
	"github.com/RuiCat/Neko/internal/expert"
	"github.com/RuiCat/Neko/internal/repair"
)

// DEFAULT_DECAY_FACTOR halves every weight per nightly pass.
const DEFAULT_DECAY_FACTOR = 0.5

// Kind classifies one operation failure so each caller maps it onto its own
// error vocabulary (an HTTP status or an MCP tool error).
type Kind string

// Failure kinds shared by the HTTP and MCP surfaces.
const (
	KIND_BAD_REQUEST Kind = "bad_request"
	KIND_NOT_FOUND   Kind = "not_found"
	KIND_CONFLICT    Kind = "conflict"
	KIND_INTERNAL    Kind = "internal"
)

// Error is one classified failure. Body carries the complete response body for
// failures that need structured detail (currently the regression-gate conflict).
type Error struct {
	Kind    Kind
	Message string
	Body    map[string]any
}

// Error implements the error interface.
func (e *Error) Error() string { return e.Message }

// BadRequest reports a malformed or incomplete request.
func BadRequest(format string, args ...any) *Error {
	return &Error{Kind: KIND_BAD_REQUEST, Message: fmt.Sprintf(format, args...)}
}

// NotFound reports a missing candidate or repair version.
func NotFound(format string, args ...any) *Error {
	return &Error{Kind: KIND_NOT_FOUND, Message: fmt.Sprintf(format, args...)}
}

// Internal reports a failed store, persistence or engine call.
func Internal(format string, args ...any) *Error {
	return &Error{Kind: KIND_INTERNAL, Message: fmt.Sprintf(format, args...)}
}

// Deps are the assembled stores one operation acts on.
type Deps struct {
	Store         *entity.Store
	Machine       *engine.StateMachine
	Experts       *expert.Set
	DataDir       string
	CandidatesTTL time.Duration
}

// validate rejects a dependency set that was never wired, so a zero Deps fails
// with an actionable error instead of a nil panic.
func (d Deps) validate() *Error {
	if d.Store == nil || d.Machine == nil || d.DataDir == "" {
		return Internal("运维依赖未配置（需要 store/machine/data-dir）")
	}
	return nil
}

// candidateStore returns the append-only candidate queue file.
func (d Deps) candidateStore() *candidates.Store {
	return candidates.NewStore(filepath.Join(d.DataDir, "candidates.jsonl"))
}

// repairStore returns the repair version history file.
func (d Deps) repairStore() *repair.Store {
	return repair.NewStore(filepath.Join(d.DataDir, "repairs", "versions.json"))
}

// candidateTTL returns the configured candidate TTL or the package default.
func (d Deps) candidateTTL() time.Duration {
	if d.CandidatesTTL > 0 {
		return d.CandidatesTTL
	}
	return candidates.DEFAULT_TTL
}

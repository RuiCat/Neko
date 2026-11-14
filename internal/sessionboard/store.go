// store.go owns every session board behind one mutex and applies the capacity
// rules: region LRU, workspace board count, board bytes and store bytes. A
// write validates, applies to a candidate copy, persists the whole store and
// only then replaces memory (K5 persistence-before-commit).
package sessionboard

import (
	"fmt"
	"sync"
	"time"
)

const (
	// MAX_WORKSPACE_BOARDS bounds the number of workspace boards in the store (K6).
	MAX_WORKSPACE_BOARDS = 128
	// MAX_STORE_BYTES bounds the serialized store file (K6).
	MAX_STORE_BYTES = 4 * 1024 * 1024
)

const (
	// CODE_BAD_REQUEST marks a malformed scope, workspace, batch or cursor (K5).
	CODE_BAD_REQUEST = "bad_request"
	// CODE_FORBIDDEN marks a cross-workspace request (K2/K9).
	CODE_FORBIDDEN = "forbidden"
	// CODE_BOARD_TOO_LARGE marks one board past 256 KiB (K9).
	CODE_BOARD_TOO_LARGE = "board_too_large"
	// CODE_WORKSPACE_LIMIT marks the 129th workspace board (K9).
	CODE_WORKSPACE_LIMIT = "workspace_limit"
	// CODE_REGION_LIMIT is reserved for a no-eviction store variant (K9).
	CODE_REGION_LIMIT = "region_limit"
	// CODE_STORE_TOO_LARGE marks a store past 4 MiB (K9).
	CODE_STORE_TOO_LARGE = "store_too_large"
	// CODE_PERSIST_FAILED marks a failed atomic save; memory stays unchanged (K5/K6).
	CODE_PERSIST_FAILED = "persist_failed"
)

// Error is one contract failure carrying the response code the control plane
// reports (K9); the message may prefix the finer K5 subcode.
type Error struct {
	Code    string
	Message string
}

// Error implements the error interface.
func (e *Error) Error() string { return e.Message }

// fail builds one contract failure.
func fail(code, message string) *Error {
	return &Error{Code: code, Message: message}
}

// Store is the concurrent owner of the global board and every workspace board
// (K1/K6). One mutex covers read-modify-persist, so readers never observe a
// half-applied batch and versions grow monotonically (K12/C14).
type Store struct {
	mu         sync.Mutex
	path       string
	global     Board
	workspaces map[string]*Board
	// now is the clock; tests pin it, production uses time.Now.
	now func() time.Time
}

// NewStore returns an empty store that persists to path (K6).
func NewStore(path string) *Store {
	state := emptyState()
	return &Store{path: path, global: state.Global, workspaces: state.Workspaces, now: time.Now}
}

// Load reads the store file into memory. A missing file leaves the empty store
// in place; a corrupt one returns the error and still leaves the empty store,
// so the caller logs it and keeps serving (K6 fail-soft).
func (s *Store) Load() error {
	state, err := loadStore(s.path)
	if err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.global = state.Global
	s.workspaces = state.Workspaces
	return nil
}

// ReadQuery is one GET /boards request (K4). Since nil means "no cursor": the
// response then omits deltas and only carries the live board view.
type ReadQuery struct {
	Scope     string
	Workspace string
	Caller    Caller
	Since     *int
}

// ReadResult is the snapshot one read returns (K4).
type ReadResult struct {
	Board     Board
	Deltas    []Delta
	Truncated bool
}

// Read returns a copied board snapshot plus the deltas after the cursor (K4).
// It never mutates state: an unknown workspace is an empty board, not an error.
func (s *Store) Read(query ReadQuery) (ReadResult, error) {
	if !validScope(query.Scope) {
		return ReadResult{}, fail(CODE_BAD_REQUEST, "invalid_scope: scope 必须是 workspace 或 global")
	}
	if query.Scope == SCOPE_WORKSPACE && query.Workspace == "" {
		return ReadResult{}, fail(CODE_BAD_REQUEST, "workspace 不能为空")
	}
	if query.Since != nil && *query.Since < 0 {
		return ReadResult{}, fail(CODE_BAD_REQUEST, "since 必须是非负整数")
	}
	if err := authorizeWorkspaceRead(query.Scope, query.Workspace, query.Caller); err != nil {
		return ReadResult{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	board := s.boardView(query.Scope, query.Workspace)
	if board == nil {
		empty := EmptyBoard(query.Scope, query.Workspace)
		board = &empty
	}
	result := ReadResult{Board: board.liveView()}
	if query.Since != nil {
		result.Deltas = board.deltasSince(*query.Since)
		result.Truncated = *query.Since > 0 && *query.Since < board.MinRetainedVersion
	}
	return result, nil
}

// WriteRequest is one POST /boards body (K5). Scope/Workspace/Caller are filled
// by the bridge; there is deliberately no region field (K2/K7).
type WriteRequest struct {
	Scope     string
	Workspace string
	Caller    Caller
	Ops       []Op
}

// WriteResult is one accepted batch's outcome (K5).
type WriteResult struct {
	Changed   bool
	Version   int
	UpdatedAt string
	Trimmed   []string
	Evicted   []string
}

// Write validates and applies one batch under the store lock (K5): the
// candidate board is persisted first and only then becomes the live board, so
// a failed save returns persist_failed with memory unchanged. A batch with no
// net change neither bumps a version nor touches the disk.
func (s *Store) Write(request WriteRequest) (WriteResult, error) {
	if !validScope(request.Scope) {
		return WriteResult{}, fail(CODE_BAD_REQUEST, "invalid_scope: scope 必须是 workspace 或 global")
	}
	if request.Scope == SCOPE_WORKSPACE && request.Workspace == "" {
		return WriteResult{}, fail(CODE_BAD_REQUEST, "workspace 不能为空")
	}
	if request.Caller.SessionID == "" {
		return WriteResult{}, fail(CODE_BAD_REQUEST, "session 不能为空")
	}
	if err := authorizeWorkspaceWrite(request.Scope, request.Workspace, request.Caller); err != nil {
		return WriteResult{}, err
	}
	target, err := resolveTarget(request.Scope, request.Caller)
	if err != nil {
		return WriteResult{}, err
	}
	ops, err := prepareOps(request.Ops)
	if err != nil {
		return WriteResult{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	board, exists := s.boardCopy(request.Scope, request.Workspace)
	if !exists && len(s.workspaces) >= MAX_WORKSPACE_BOARDS {
		return WriteResult{}, fail(CODE_WORKSPACE_LIMIT,
			fmt.Sprintf("工作区板已达 %d 个上限", MAX_WORKSPACE_BOARDS))
	}
	now := s.now()
	changed, trimmed := board.applyOps(target, request.Caller.SessionID, ops, now)
	if !changed {
		return WriteResult{Changed: false, Version: board.Version}, nil
	}
	evicted := board.enforceRegionCap()
	size, err := board.encodeSize()
	if err != nil {
		return WriteResult{}, fail(CODE_PERSIST_FAILED, "序列化候选板失败: "+err.Error())
	}
	if size > MAX_BOARD_BYTES {
		return WriteResult{}, fail(CODE_BOARD_TOO_LARGE,
			fmt.Sprintf("单板序列化超过 %d 字节上限", MAX_BOARD_BYTES))
	}
	payload, err := s.encodeCandidate(request.Scope, request.Workspace, board, now)
	if err != nil {
		return WriteResult{}, fail(CODE_PERSIST_FAILED, err.Error())
	}
	if len(payload) > MAX_STORE_BYTES {
		return WriteResult{}, fail(CODE_STORE_TOO_LARGE,
			fmt.Sprintf("会话黑板存储超过 %d 字节上限", MAX_STORE_BYTES))
	}
	if err := writeFileAtomic(s.path, payload); err != nil {
		return WriteResult{}, fail(CODE_PERSIST_FAILED, "保存会话黑板失败: "+err.Error())
	}
	s.commit(request.Scope, request.Workspace, board)
	return WriteResult{Changed: true, Version: board.Version, UpdatedAt: board.UpdatedAt,
		Trimmed: trimmed, Evicted: evicted}, nil
}

// boardView returns the stored board a read addresses, or nil when the board
// has never been written (K4: unknown workspace is an empty board).
func (s *Store) boardView(scope, workspace string) *Board {
	if scope == SCOPE_GLOBAL {
		return &s.global
	}
	return s.workspaces[workspace]
}

// boardCopy returns a deep copy of the addressed board so a candidate write can
// be applied without touching live state (K5).
func (s *Store) boardCopy(scope, workspace string) (*Board, bool) {
	if scope == SCOPE_GLOBAL {
		copied := s.global.clone()
		return copied, true
	}
	if stored, exists := s.workspaces[workspace]; exists {
		return stored.clone(), true
	}
	empty := newBoard(SCOPE_WORKSPACE, workspace)
	return &empty, false
}

// encodeCandidate serializes the store as it would look after the candidate
// board is committed (K6); the map copy keeps the live state untouched.
func (s *Store) encodeCandidate(scope, workspace string, board *Board, at time.Time) ([]byte, error) {
	if scope == SCOPE_GLOBAL {
		return encodeState(*board, s.workspaces, at)
	}
	candidate := make(map[string]*Board, len(s.workspaces)+1)
	for key, stored := range s.workspaces {
		candidate[key] = stored
	}
	candidate[workspace] = board
	return encodeState(s.global, candidate, at)
}

// commit replaces the live board with the persisted candidate (K5).
func (s *Store) commit(scope, workspace string, board *Board) {
	if scope == SCOPE_GLOBAL {
		s.global = *board
		return
	}
	s.workspaces[workspace] = board
}

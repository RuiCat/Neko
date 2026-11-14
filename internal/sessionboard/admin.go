// admin.go is the panel's admin write and workspace listing over the session
// boards (K13/K14): the panel names the board and the region explicitly, every
// entry is stamped by "面板", and the listing is a summary snapshot with no
// entry contents.
package sessionboard

import (
	"fmt"
	"sort"
)

const (
	// PANEL_BY is the fixed entry author of a panel write (K13): the panel has
	// no session identity, so its writes are attributed to the admin surface.
	PANEL_BY = "面板"
)

// PanelWriteRequest is one POST /boards/panel body (K13): unlike K5 the panel
// addresses the board and the region explicitly instead of deriving them from a
// caller, and it may create a region (owner kind/id fixed by the region id).
type PanelWriteRequest struct {
	Scope     string
	Workspace string
	Region    string
	Ops       []Op
}

// WorkspaceSummary is one row of GET /boards/workspaces (K14): the workspace
// key, the board version and update stamp, and the region count.
type WorkspaceSummary struct {
	Workspace string `json:"workspace"`
	Version   int    `json:"version"`
	// Regions is the region count, not a list (K14 revision 2026-11-10 16:16):
	// region details are read through GET /boards so the listing stays a light
	// index.
	Regions   int    `json:"regions"`
	UpdatedAt string `json:"updated_at"`
}

// WritePanel applies one admin batch to the named region (K13). The region is
// created on first write with the owner kind/id fixed by the scope. The
// candidate is persisted before it commits and a batch with no net change
// neither bumps a version nor touches the disk, exactly like K5.
func (s *Store) WritePanel(request PanelWriteRequest) (WriteResult, error) {
	target, err := panelTarget(request)
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
	if !exists && request.Scope == SCOPE_WORKSPACE && len(s.workspaces) >= MAX_WORKSPACE_BOARDS {
		return WriteResult{}, fail(CODE_WORKSPACE_LIMIT,
			fmt.Sprintf("工作区板已达 %d 个上限", MAX_WORKSPACE_BOARDS))
	}
	now := s.now()
	changed, trimmed := board.applyOps(target, PANEL_BY, ops, now)
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

// ListWorkspaces returns a copied summary of every workspace board, sorted by
// workspace key ascending (K14). No boards is an empty slice, never nil, and a
// caller can never alias the live store.
func (s *Store) ListWorkspaces() []WorkspaceSummary {
	s.mu.Lock()
	defer s.mu.Unlock()
	summaries := make([]WorkspaceSummary, 0, len(s.workspaces))
	for key, board := range s.workspaces {
		summaries = append(summaries, WorkspaceSummary{Workspace: key, Version: board.Version,
			Regions: len(board.Regions), UpdatedAt: board.UpdatedAt})
	}
	sort.Slice(summaries, func(i, j int) bool { return summaries[i].Workspace < summaries[j].Workspace })
	return summaries
}

// panelTarget derives the region one panel write addresses (K13): scope=workspace
// requires workspace and region (owner kind=session); scope=global defaults the
// region to the workspace key and rejects any other region (owner kind=workspace).
func panelTarget(request PanelWriteRequest) (Target, error) {
	if !validScope(request.Scope) {
		return Target{}, fail(CODE_BAD_REQUEST, "invalid_scope: scope 必须是 workspace 或 global")
	}
	if request.Workspace == "" {
		return Target{}, fail(CODE_BAD_REQUEST, "workspace 不能为空")
	}
	if request.Scope == SCOPE_WORKSPACE {
		if request.Region == "" {
			return Target{}, fail(CODE_BAD_REQUEST, "region 不能为空")
		}
		return Target{RegionID: request.Region,
			Owner: Owner{Kind: OWNER_SESSION, ID: request.Region}}, nil
	}
	region := request.Region
	if region == "" {
		region = request.Workspace
	}
	if region != request.Workspace {
		return Target{}, fail(CODE_BAD_REQUEST, "global 板 region 必须等于 workspace")
	}
	return Target{RegionID: region, Owner: Owner{Kind: OWNER_WORKSPACE, ID: region}}, nil
}

// ownership.go is the K2 permission matrix: the writable region and the read
// authorization of one caller are always derived from scope + caller, never
// from a request parameter.
package sessionboard

const (
	// OWNER_SESSION marks a region owned by one DSH session (workspace board).
	OWNER_SESSION = "session"
	// OWNER_WORKSPACE marks a region owned by one workspace (global board).
	OWNER_WORKSPACE = "workspace"
)

// Caller is the bridge-injected identity of one request (K7). The bridge fills
// it from exec.agent; it is deliberately not a tool parameter the model can set.
type Caller struct {
	SessionID    string
	WorkspaceKey string
}

// Target is the one region a write addresses, derived from scope + caller (K2).
type Target struct {
	RegionID string
	Owner    Owner
}

// resolveTarget derives the writable region of one caller (K2): a workspace
// board gives one region per session, the global board one region per
// workspace. There is no request field that names a region, so a write cannot
// address somebody else's region.
func resolveTarget(scope string, caller Caller) (Target, error) {
	switch scope {
	case SCOPE_WORKSPACE:
		if caller.SessionID == "" {
			return Target{}, fail(CODE_BAD_REQUEST, "session 不能为空")
		}
		return Target{RegionID: caller.SessionID, Owner: Owner{Kind: OWNER_SESSION, ID: caller.SessionID}}, nil
	case SCOPE_GLOBAL:
		if caller.WorkspaceKey == "" {
			return Target{}, fail(CODE_BAD_REQUEST, "workspace 不能为空")
		}
		return Target{RegionID: caller.WorkspaceKey,
			Owner: Owner{Kind: OWNER_WORKSPACE, ID: caller.WorkspaceKey}}, nil
	default:
		return Target{}, fail(CODE_BAD_REQUEST, "invalid_scope: scope 必须是 workspace 或 global")
	}
}

// authorizeWorkspaceRead enforces K2's workspace read rule: the caller's
// workspace must be the board's own workspace. The HTTP layer fills the caller
// workspace from the same query field (the bridge is trusted, C8/D2), so the
// guard is exercised by direct store callers and future identity sources.
func authorizeWorkspaceRead(scope, workspace string, caller Caller) error {
	if scope != SCOPE_WORKSPACE {
		return nil
	}
	if caller.WorkspaceKey != workspace {
		return fail(CODE_FORBIDDEN, "forbidden: 不能读取其它工作区的黑板")
	}
	return nil
}

// authorizeWorkspaceWrite enforces K2's workspace write rule: a write may only
// touch the caller's own workspace board, and the region inside it is derived
// from the session.
func authorizeWorkspaceWrite(scope, workspace string, caller Caller) error {
	if scope != SCOPE_WORKSPACE {
		return nil
	}
	if caller.WorkspaceKey != workspace {
		return fail(CODE_FORBIDDEN, "forbidden: 不能写入其它工作区的黑板")
	}
	return nil
}

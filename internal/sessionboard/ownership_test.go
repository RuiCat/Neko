// ownership_test.go is the K2/C11 scope × caller × op matrix: every cell states
// whether it succeeds or which error code it returns.
package sessionboard

import (
	"strconv"
	"strings"
	"testing"
)

// scopeCase is one matrix cell: the request, the operation and its expected
// outcome. wantCode "" means success.
type scopeCase struct {
	name      string
	scope     string
	workspace string
	caller    Caller
	op        string
	wantCode  string
	wantNote  string
}

// TestScopeMatrix covers workspace|global × 本区|同工作区他区|他工作区 ×
// read|write|delete|clear (C11): the target region is always derived from the
// caller, and a cross-workspace request is forbidden.
func TestScopeMatrix(t *testing.T) {
	own := newCaller("session-a", "/ws/a")
	otherSession := newCaller("session-b", "/ws/a")
	otherWorkspace := newCaller("session-b", "/ws/b")
	globalCaller := newCaller("session-g", "")

	cases := []scopeCase{
		{name: "workspace read own", scope: SCOPE_WORKSPACE, workspace: "/ws/a", caller: own, op: "read"},
		{name: "workspace read other workspace", scope: SCOPE_WORKSPACE, workspace: "/ws/a",
			caller: otherWorkspace, op: "read", wantCode: CODE_FORBIDDEN},
		{name: "workspace read unknown workspace", scope: SCOPE_WORKSPACE, workspace: "/ws/none",
			caller: newCaller("session-x", "/ws/none"), op: "read"},
		{name: "workspace read missing workspace", scope: SCOPE_WORKSPACE, workspace: "",
			caller: newCaller("session-a", ""), op: "read", wantCode: CODE_BAD_REQUEST},
		{name: "workspace write own region", scope: SCOPE_WORKSPACE, workspace: "/ws/a",
			caller: own, op: "write", wantNote: "region=session-a,owner=session"},
		{name: "workspace write same workspace other session", scope: SCOPE_WORKSPACE,
			workspace: "/ws/a", caller: otherSession, op: "write", wantNote: "region=session-b,owner=session"},
		{name: "workspace write other workspace", scope: SCOPE_WORKSPACE, workspace: "/ws/b",
			caller: own, op: "write", wantCode: CODE_FORBIDDEN},
		{name: "workspace write missing session", scope: SCOPE_WORKSPACE, workspace: "/ws/a",
			caller: newCaller("", "/ws/a"), op: "write", wantCode: CODE_BAD_REQUEST},
		{name: "workspace delete own region", scope: SCOPE_WORKSPACE, workspace: "/ws/a",
			caller: own, op: "delete"},
		{name: "workspace delete other workspace", scope: SCOPE_WORKSPACE, workspace: "/ws/b",
			caller: own, op: "delete", wantCode: CODE_FORBIDDEN},
		{name: "workspace clear own region", scope: SCOPE_WORKSPACE, workspace: "/ws/a",
			caller: own, op: "clear"},
		{name: "workspace clear other workspace", scope: SCOPE_WORKSPACE, workspace: "/ws/b",
			caller: own, op: "clear", wantCode: CODE_FORBIDDEN},
		{name: "global read as workspace caller", scope: SCOPE_GLOBAL, caller: own, op: "read"},
		{name: "global read as anonymous caller", scope: SCOPE_GLOBAL, caller: Caller{}, op: "read"},
		{name: "global write own workspace region", scope: SCOPE_GLOBAL, workspace: "/ws/a",
			caller: own, op: "write", wantNote: "region=/ws/a,owner=workspace"},
		{name: "global write other workspace region", scope: SCOPE_GLOBAL, workspace: "/ws/b",
			caller: otherWorkspace, op: "write", wantNote: "region=/ws/b,owner=workspace"},
		{name: "global write missing workspace", scope: SCOPE_GLOBAL, caller: globalCaller,
			op: "write", wantCode: CODE_BAD_REQUEST},
		{name: "global delete own workspace region", scope: SCOPE_GLOBAL, workspace: "/ws/a",
			caller: own, op: "delete"},
		{name: "invalid scope read", scope: "planet", caller: own, op: "read", wantCode: CODE_BAD_REQUEST},
		{name: "invalid scope write", scope: "planet", caller: own, op: "write", wantCode: CODE_BAD_REQUEST},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			store := newTestStore(t)
			seedMatrixBoard(t, store)
			note, err := runMatrixOp(store, testCase)
			if testCase.wantCode != "" {
				if err == nil {
					t.Fatalf("期望 %s，实际成功", testCase.wantCode)
				}
				if code := errorCodeOf(t, err); code != testCase.wantCode {
					t.Fatalf("code = %q, want %q (%v)", code, testCase.wantCode, err)
				}
				if read := readBoard(t, store, SCOPE_WORKSPACE, "/ws/a", nil); read.Board.Version != 1 {
					t.Fatalf("被拒绝的请求改了板: version=%d", read.Board.Version)
				}
				return
			}
			if err != nil {
				t.Fatalf("期望成功，实际 %v", err)
			}
			if testCase.wantNote != "" && note != testCase.wantNote {
				t.Fatalf("region = %q, want %q", note, testCase.wantNote)
			}
		})
	}
}

// seedMatrixBoard writes one key as session-a of workspace /ws/a, so every
// matrix cell starts from the same version-1 board.
func seedMatrixBoard(t *testing.T, store *Store) {
	t.Helper()
	mustWrite(t, store, setRequest(t, "/ws/a", "session-a", map[string]string{"任务.当前": "写规划"}))
}

// runMatrixOp executes one matrix cell and returns a note describing the
// derived region for write cases.
func runMatrixOp(store *Store, testCase scopeCase) (string, error) {
	ops := []Op{{Op: OP_SET, Key: "矩阵.键", Value: rawValue("值")}}
	switch testCase.op {
	case "read":
		_, err := store.Read(ReadQuery{Scope: testCase.scope, Workspace: testCase.workspace,
			Caller: testCase.caller})
		return "", err
	case "delete":
		ops = []Op{{Op: OP_DELETE, Key: "任务.当前"}}
	case "clear":
		ops = []Op{{Op: OP_CLEAR}}
	}
	_, err := store.Write(WriteRequest{Scope: testCase.scope, Workspace: testCase.workspace,
		Caller: testCase.caller, Ops: ops})
	if err != nil {
		return "", err
	}
	if testCase.wantNote == "" {
		return "", nil
	}
	regionID, ownerKind := writtenRegion(store, testCase)
	return "region=" + regionID + ",owner=" + ownerKind, nil
}

// writtenRegion reports the region the matrix case actually wrote, derived from
// the caller, never from a request field.
func writtenRegion(store *Store, testCase scopeCase) (string, string) {
	scope, workspace := testCase.scope, testCase.workspace
	if scope == SCOPE_GLOBAL {
		workspace = ""
	}
	read := readBoardForCaller(store, scope, workspace, testCase.caller)
	last := read.Board.Regions[len(read.Board.Regions)-1]
	return last.ID, last.Owner.Kind
}

// readBoardForCaller reads one board with an explicit caller.
func readBoardForCaller(store *Store, scope, workspace string, caller Caller) ReadResult {
	result, err := store.Read(ReadQuery{Scope: scope, Workspace: workspace, Caller: caller})
	if err != nil {
		panic("矩阵读取失败: " + err.Error())
	}
	return result
}

// TestSameWorkspaceSharesGlobalRegion covers K2's global rule: two sessions of
// one workspace write the same region, a third workspace cannot touch it.
func TestSameWorkspaceSharesGlobalRegion(t *testing.T) {
	store := newTestStore(t)
	first := WriteRequest{Scope: SCOPE_GLOBAL, Workspace: "/ws/a",
		Caller: newCaller("session-a", "/ws/a"),
		Ops:    []Op{{Op: OP_SET, Key: "项目.状态", Value: rawValue("进行中")}}}
	mustWrite(t, store, first)
	second := WriteRequest{Scope: SCOPE_GLOBAL, Workspace: "/ws/a",
		Caller: newCaller("session-b", "/ws/a"),
		Ops:    []Op{{Op: OP_SET, Key: "任务.当前", Value: rawValue("写实现")}}}
	mustWrite(t, store, second)
	third := WriteRequest{Scope: SCOPE_GLOBAL, Workspace: "/ws/b",
		Caller: newCaller("session-c", "/ws/b"),
		Ops:    []Op{{Op: OP_SET, Key: "项目.状态", Value: rawValue("阻塞")}}}
	mustWrite(t, store, third)

	read := readBoard(t, store, SCOPE_GLOBAL, "", nil)
	if len(read.Board.Regions) != 2 {
		t.Fatalf("regions = %d, want 2", len(read.Board.Regions))
	}
	for _, region := range read.Board.Regions {
		switch region.Owner.ID {
		case "/ws/a":
			if len(region.Entries) != 2 {
				t.Fatalf("/ws/a 条目 = %d, want 2（区内会话共写一区）", len(region.Entries))
			}
		case "/ws/b":
			if len(region.Entries) != 1 {
				t.Fatalf("/ws/b 条目 = %d, want 1", len(region.Entries))
			}
		default:
			t.Fatalf("未知区域 = %+v", region)
		}
	}
}

// TestWriteDerivesRegionOnly covers K2's "no region parameter": a caller's
// session/workspace always decides the region and no request field can name
// another one (the write request type has no region field at all).
func TestWriteDerivesRegionOnly(t *testing.T) {
	store := newTestStore(t)
	mustWrite(t, store, setRequest(t, "/ws/a", "session-a", map[string]string{"a": "1"}))
	read := readBoard(t, store, SCOPE_WORKSPACE, "/ws/a", nil)
	if len(read.Board.Regions) != 1 || read.Board.Regions[0].ID != "session-a" {
		t.Fatalf("regions = %+v", read.Board.Regions)
	}
	// A same-workspace second session gets its own region instead of writing into
	// session-a's region.
	mustWrite(t, store, setRequest(t, "/ws/a", "session-b", map[string]string{"b": "2"}))
	read = readBoard(t, store, SCOPE_WORKSPACE, "/ws/a", nil)
	if len(read.Board.Regions) != 2 {
		t.Fatalf("regions = %d, want 2", len(read.Board.Regions))
	}
	for _, region := range read.Board.Regions {
		if region.ID == "session-a" && len(region.Entries) != 1 || strings.Contains(region.ID, "session-b") && len(region.Entries) != 1 {
			t.Fatalf("region %s entries = %d", region.ID, len(region.Entries))
		}
	}
}

// TestGlobalRegionIDsAreWorkspaceKeys pins the global region id: one workspace
// key, not a session id (K1).
func TestGlobalRegionIDsAreWorkspaceKeys(t *testing.T) {
	store := newTestStore(t)
	for index := 0; index < 2; index++ {
		workspace := "/ws/" + strconv.Itoa(index)
		mustWrite(t, store, WriteRequest{Scope: SCOPE_GLOBAL, Workspace: workspace,
			Caller: newCaller("session-"+strconv.Itoa(index), workspace),
			Ops:    []Op{{Op: OP_SET, Key: "k", Value: rawValue("v")}}})
	}
	read := readBoard(t, store, SCOPE_GLOBAL, "", nil)
	for _, region := range read.Board.Regions {
		if !strings.HasPrefix(region.ID, "/ws/") {
			t.Fatalf("global 区 id = %q, want 工作区键", region.ID)
		}
	}
}

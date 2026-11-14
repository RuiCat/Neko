// admin_test.go covers the panel admin surface: explicit workspace/region
// writes stamped by "面板", the global board's workspace-key region, the
// validation matrix, capacity and the workspace listing (K13/K14).
package sessionboard

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"
)

// panelSet builds one panel write of one string key.
func panelSet(scope, workspace, region, key, value string) PanelWriteRequest {
	return PanelWriteRequest{Scope: scope, Workspace: workspace, Region: region,
		Ops: []Op{{Op: OP_SET, Key: key, Value: rawValue(value)}}}
}

// mustPanelWrite applies one panel batch and fails the test on error.
func mustPanelWrite(t *testing.T, store *Store, request PanelWriteRequest) WriteResult {
	t.Helper()
	result, err := store.WritePanel(request)
	if err != nil {
		t.Fatalf("panel write: %v", err)
	}
	return result
}

// TestPanelWrite covers the workspace-board admin write (K13): an explicit
// region id creates the region with a session owner, entries carry by="面板",
// an unchanged rewrite neither bumps a version nor saves, clear tombstones the
// region and the board reloads from disk.
func TestPanelWrite(t *testing.T) {
	store := newTestStore(t)
	result := mustPanelWrite(t, store, panelSet(SCOPE_WORKSPACE, "/ws/a", "panel-region", "任务.当前", "写规划"))
	if !result.Changed || result.Version != 1 || result.UpdatedAt == "" {
		t.Fatalf("write result = %+v", result)
	}

	read := readBoard(t, store, SCOPE_WORKSPACE, "/ws/a", nil)
	if read.Board.Version != 1 || len(read.Board.Regions) != 1 {
		t.Fatalf("board = %+v", read.Board)
	}
	region := read.Board.Regions[0]
	if region.ID != "panel-region" || region.Owner.Kind != OWNER_SESSION || region.Owner.ID != "panel-region" {
		t.Fatalf("region = %+v", region)
	}
	entry := region.Entries[0]
	if entry.By != PANEL_BY || entry.To != "写规划" || entry.Deleted {
		t.Fatalf("entry = %+v", entry)
	}

	// Rewriting the same value is not a net change: no version bump, no save.
	before, err := os.ReadFile(store.path)
	if err != nil {
		t.Fatalf("读文件: %v", err)
	}
	same := mustPanelWrite(t, store, panelSet(SCOPE_WORKSPACE, "/ws/a", "panel-region", "任务.当前", "写规划"))
	if same.Changed || same.Version != 1 {
		t.Fatalf("同值重写 = %+v", same)
	}
	after, err := os.ReadFile(store.path)
	if err != nil {
		t.Fatalf("读文件: %v", err)
	}
	if string(before) != string(after) {
		t.Fatal("无净变化却写盘了")
	}

	// A panel write may add a second region to an existing board.
	second := mustPanelWrite(t, store, panelSet(SCOPE_WORKSPACE, "/ws/a", "panel-region-2", "k", "v"))
	if !second.Changed || second.Version != 2 || len(readBoard(t, store, SCOPE_WORKSPACE, "/ws/a", nil).Board.Regions) != 2 {
		t.Fatalf("新区写入 = %+v", second)
	}

	// clear is a single-op batch that tombstones every live key of the region.
	cleared := mustPanelWrite(t, store, PanelWriteRequest{Scope: SCOPE_WORKSPACE,
		Workspace: "/ws/a", Region: "panel-region", Ops: []Op{{Op: OP_CLEAR}}})
	if !cleared.Changed || cleared.Version != 3 {
		t.Fatalf("clear = %+v", cleared)
	}
	two := 2
	deltas := readBoard(t, store, SCOPE_WORKSPACE, "/ws/a", &two).Deltas
	if len(deltas) != 1 || !deltas[0].Deleted || deltas[0].By != PANEL_BY {
		t.Fatalf("clear deltas = %+v", deltas)
	}

	// The panel board persists like any other board (K6).
	reloaded := NewStore(store.path)
	if err := reloaded.Load(); err != nil {
		t.Fatalf("load: %v", err)
	}
	reloadedRead := readBoard(t, reloaded, SCOPE_WORKSPACE, "/ws/a", nil)
	if reloadedRead.Board.Version != 3 || len(reloadedRead.Board.Regions) != 2 {
		t.Fatalf("重载板 = %+v", reloadedRead.Board)
	}
}

// TestPanelWriteGlobalRegion covers the global board's admin write (K13): the
// region defaults to the workspace key, an explicit equal region is accepted,
// the owner is fixed to workspace and any other region is rejected.
func TestPanelWriteGlobalRegion(t *testing.T) {
	store := newTestStore(t)
	result := mustPanelWrite(t, store, panelSet(SCOPE_GLOBAL, "/ws/a", "", "项目.状态", "进行中"))
	if !result.Changed || result.Version != 1 {
		t.Fatalf("global write = %+v", result)
	}
	read := readBoard(t, store, SCOPE_GLOBAL, "", nil)
	if read.Board.Scope != SCOPE_GLOBAL || read.Board.ScopeID != GLOBAL_SCOPE_ID || len(read.Board.Regions) != 1 {
		t.Fatalf("global board = %+v", read.Board)
	}
	region := read.Board.Regions[0]
	if region.ID != "/ws/a" || region.Owner.Kind != OWNER_WORKSPACE || region.Owner.ID != "/ws/a" {
		t.Fatalf("global region = %+v", region)
	}
	if region.Entries[0].By != PANEL_BY {
		t.Fatalf("global entry by = %q", region.Entries[0].By)
	}

	// An explicit region equal to the workspace key is accepted and does not
	// change the owner.
	second := mustPanelWrite(t, store, panelSet(SCOPE_GLOBAL, "/ws/a", "/ws/a", "项目.状态", "暂停"))
	if !second.Changed || second.Version != 2 {
		t.Fatalf("显式同区写入 = %+v", second)
	}
	region = readBoard(t, store, SCOPE_GLOBAL, "", nil).Board.Regions[0]
	if region.Owner.Kind != OWNER_WORKSPACE || region.Owner.ID != "/ws/a" {
		t.Fatalf("owner 被改了 = %+v", region.Owner)
	}

	// Any other region on the global board is a bad request and changes nothing.
	_, err := store.WritePanel(panelSet(SCOPE_GLOBAL, "/ws/a", "/ws/b", "k", "v"))
	if code := errorCodeOf(t, err); code != CODE_BAD_REQUEST {
		t.Fatalf("code = %q, want %q", code, CODE_BAD_REQUEST)
	}
	if !strings.Contains(err.Error(), "region 必须等于 workspace") {
		t.Fatalf("message = %q, want 含 %q", err.Error(), "region 必须等于 workspace")
	}
	if version := readBoard(t, store, SCOPE_GLOBAL, "", nil).Board.Version; version != 2 {
		t.Fatalf("被拒绝的批改了版本: %d", version)
	}
}

// TestPanelWriteValidation covers the K13 request matrix: every rejected shape
// reports bad_request with the subcode in the message and never changes state.
func TestPanelWriteValidation(t *testing.T) {
	store := newTestStore(t)
	validOps := panelSet(SCOPE_WORKSPACE, "/ws/a", "region-a", "k", "v").Ops
	cases := []struct {
		name    string
		request PanelWriteRequest
		want    string
	}{
		{"invalid scope", PanelWriteRequest{Scope: "planet", Workspace: "/ws/a",
			Region: "region-a", Ops: validOps}, "invalid_scope"},
		{"missing workspace", PanelWriteRequest{Scope: SCOPE_WORKSPACE,
			Region: "region-a", Ops: validOps}, "workspace 不能为空"},
		{"missing region", PanelWriteRequest{Scope: SCOPE_WORKSPACE,
			Workspace: "/ws/a", Ops: validOps}, "region 不能为空"},
		{"global region mismatch", PanelWriteRequest{Scope: SCOPE_GLOBAL,
			Workspace: "/ws/a", Region: "/ws/b", Ops: validOps}, "region 必须等于 workspace"},
		{"empty ops", PanelWriteRequest{Scope: SCOPE_WORKSPACE,
			Workspace: "/ws/a", Region: "region-a"}, "invalid_ops"},
		{"clear with others", PanelWriteRequest{Scope: SCOPE_WORKSPACE,
			Workspace: "/ws/a", Region: "region-a",
			Ops: []Op{{Op: OP_CLEAR}, {Op: OP_SET, Key: "k", Value: rawValue("v")}}}, "invalid_ops"},
		{"unknown op", PanelWriteRequest{Scope: SCOPE_WORKSPACE,
			Workspace: "/ws/a", Region: "region-a", Ops: []Op{{Op: "add", Key: "k"}}}, "invalid_ops"},
		{"too many ops", PanelWriteRequest{Scope: SCOPE_WORKSPACE,
			Workspace: "/ws/a", Region: "region-a", Ops: tooManyOps()}, "too_many_ops"},
		{"blank key", PanelWriteRequest{Scope: SCOPE_WORKSPACE,
			Workspace: "/ws/a", Region: "region-a",
			Ops: []Op{{Op: OP_SET, Key: "  ", Value: rawValue("v")}}}, "invalid_key"},
		{"newline key", PanelWriteRequest{Scope: SCOPE_WORKSPACE,
			Workspace: "/ws/a", Region: "region-a",
			Ops: []Op{{Op: OP_SET, Key: "a\nb", Value: rawValue("v")}}}, "invalid_key"},
		{"oversize value", PanelWriteRequest{Scope: SCOPE_WORKSPACE,
			Workspace: "/ws/a", Region: "region-a",
			Ops: []Op{{Op: OP_SET, Key: "k", Value: rawValue(strings.Repeat("x", MAX_VALUE_BYTES))}}}, "invalid_value"},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			_, err := store.WritePanel(testCase.request)
			if err == nil {
				t.Fatal("非法请求被接受")
			}
			if code := errorCodeOf(t, err); code != CODE_BAD_REQUEST {
				t.Fatalf("code = %q, want %q", code, CODE_BAD_REQUEST)
			}
			if !strings.Contains(err.Error(), testCase.want) {
				t.Fatalf("message = %q, want 含 %q", err.Error(), testCase.want)
			}
		})
	}
	if read := readBoard(t, store, SCOPE_WORKSPACE, "/ws/a", nil); read.Board.Version != 0 {
		t.Fatalf("被拒绝的批改了状态: version=%d", read.Board.Version)
	}
	if summaries := store.ListWorkspaces(); len(summaries) != 0 {
		t.Fatalf("被拒绝的批建了工作区板: %+v", summaries)
	}
}

// TestPanelCapacity covers the K3/K9 capacity rules on the panel path: the 65th
// region evicts the oldest one and the 129th workspace board is refused.
func TestPanelCapacity(t *testing.T) {
	store := newTestStore(t)
	var last WriteResult
	for index := 0; index < MAX_REGIONS+1; index++ {
		last = mustPanelWrite(t, store, panelSet(SCOPE_WORKSPACE, "/ws/cap",
			fmt.Sprintf("region-%03d", index), "k", "v"))
	}
	if len(last.Evicted) != 1 || last.Evicted[0] != "region-000" {
		t.Fatalf("淘汰 = %+v, want 最旧的 region-000", last.Evicted)
	}
	board := readBoard(t, store, SCOPE_WORKSPACE, "/ws/cap", nil).Board
	if len(board.Regions) != MAX_REGIONS {
		t.Fatalf("regions = %d, want %d", len(board.Regions), MAX_REGIONS)
	}

	limited := newTestStore(t)
	for index := 0; index < MAX_WORKSPACE_BOARDS; index++ {
		mustPanelWrite(t, limited, panelSet(SCOPE_WORKSPACE, fmt.Sprintf("/ws/%03d", index), "r", "k", "v"))
	}
	_, err := limited.WritePanel(panelSet(SCOPE_WORKSPACE, "/ws/over", "r", "k", "v"))
	if code := errorCodeOf(t, err); code != CODE_WORKSPACE_LIMIT {
		t.Fatalf("code = %q, want %q", code, CODE_WORKSPACE_LIMIT)
	}
}

// TestListWorkspaces covers the K14 listing: workspace boards only, ascending
// order, the region count as a number and a copied snapshot summary.
func TestListWorkspaces(t *testing.T) {
	store := newTestStore(t)
	empty := store.ListWorkspaces()
	if empty == nil || len(empty) != 0 {
		t.Fatalf("空列表 = %v", empty)
	}
	mustPanelWrite(t, store, panelSet(SCOPE_WORKSPACE, "/ws/b", "reg-b", "k", "v"))
	mustPanelWrite(t, store, panelSet(SCOPE_WORKSPACE, "/ws/a", "reg-y", "k", "v"))
	mustPanelWrite(t, store, panelSet(SCOPE_WORKSPACE, "/ws/a", "reg-x", "k2", "v2"))
	mustPanelWrite(t, store, panelSet(SCOPE_GLOBAL, "/ws/c", "", "g", "1"))

	summaries := store.ListWorkspaces()
	if len(summaries) != 2 {
		t.Fatalf("工作区数 = %d, want 2（global 不列出）", len(summaries))
	}
	if summaries[0].Workspace != "/ws/a" || summaries[1].Workspace != "/ws/b" {
		t.Fatalf("顺序 = %q, %q", summaries[0].Workspace, summaries[1].Workspace)
	}
	first := summaries[0]
	if first.Version != 2 || first.UpdatedAt == "" {
		t.Fatalf("摘要 = %+v", first)
	}
	if first.Regions != 2 || summaries[1].Regions != 1 {
		t.Fatalf("区域数量 = %d, %d, want 2, 1", first.Regions, summaries[1].Regions)
	}

	// The wire shape carries regions as a number and no entry contents (K14).
	raw, err := json.Marshal(summaries)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	decoded := []map[string]any{}
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if count, ok := decoded[0]["regions"].(float64); !ok || count != 2 {
		t.Fatalf("regions 不是数字: %v", decoded[0]["regions"])
	}
	if strings.Contains(string(raw), `"entries"`) {
		t.Fatalf("列表暴露了 entry 内容: %s", raw)
	}

	// The answer is a copy: mutating it cannot reach the live board.
	summaries[0].Regions = 99
	if again := store.ListWorkspaces(); again[0].Regions != 2 {
		t.Fatalf("列表不是快照: live region 数 = %d", again[0].Regions)
	}
}

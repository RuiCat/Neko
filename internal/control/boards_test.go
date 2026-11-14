// boards_test.go covers the /boards routes: batch body limits, envelopes, the
// read/write round trip, empty boards and the identity-derived region (K4/K5/K9).
package control

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/RuiCat/Neko/internal/sessionboard"
)

// boardBody builds one POST /boards body.
func boardBody(t *testing.T, scope, workspace, session string, ops []map[string]any) []byte {
	t.Helper()
	body, err := json.Marshal(map[string]any{"scope": scope, "workspace": workspace,
		"session": session, "ops": ops})
	if err != nil {
		t.Fatalf("marshal body: %v", err)
	}
	return body
}

// setOps builds one map-shaped set operation.
func setOps(key, value string) map[string]any {
	return map[string]any{"op": "set", "key": key, "value": value}
}

// boardPath builds one GET /boards target with URL-encoded workspace.
func boardPath(scope, workspace string) string {
	target := "/boards?scope=" + scope
	if workspace != "" {
		target += "&workspace=" + strings.ReplaceAll(workspace, "/", "%2F")
	}
	return target
}

// TestWriteBatchMaxBody covers C3: the largest legal batch (16 ops × 2048-byte
// values) stays below the control plane's 64 KiB body limit and is accepted.
func TestWriteBatchMaxBody(t *testing.T) {
	env := newTestEnv(t, Options{})
	value := strings.Repeat("x", sessionboard.MAX_VALUE_BYTES-2)
	ops := make([]map[string]any, 0, sessionboard.MAX_OPS)
	for index := 0; index < sessionboard.MAX_OPS; index++ {
		ops = append(ops, setOps("批量."+strconv.Itoa(index), value))
	}
	body := boardBody(t, "workspace", "/ws/body", "session-body", ops)
	if len(body) >= MAX_BODY_BYTES {
		t.Fatalf("最大合法批 %d 字节，超过控制面上限 %d", len(body), MAX_BODY_BYTES)
	}
	recorder := env.call(t, http.MethodPost, "/boards", body)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
	payload := decodeBody(t, recorder)
	if payload["changed"] != true {
		t.Fatalf("changed = %v", payload["changed"])
	}
	if payload["version"] != float64(1) {
		t.Fatalf("version = %v", payload["version"])
	}
	read := decodeBody(t, env.call(t, http.MethodGet, boardPath("workspace", "/ws/body"), nil))
	board := read["board"].(map[string]any)
	regions := board["regions"].([]any)
	if len(regions) != 1 {
		t.Fatalf("regions = %d", len(regions))
	}
	entries := regions[0].(map[string]any)["entries"].([]any)
	if len(entries) != sessionboard.MAX_OPS {
		t.Fatalf("entries = %d, want %d", len(entries), sessionboard.MAX_OPS)
	}
	for _, entry := range entries {
		if len(entry.(map[string]any)["to"].(string)) != sessionboard.MAX_VALUE_BYTES-2 {
			t.Fatalf("值被截断")
		}
	}
}

// TestErrorEnvelope covers C4: every failure reuses the {ok:false,error:{code,
// message}} envelope with the registered code and HTTP status (K9).
func TestErrorEnvelope(t *testing.T) {
	env := newTestEnv(t, Options{})
	cases := []struct {
		name   string
		method string
		target string
		body   []byte
		status int
		code   string
	}{
		{name: "wrong method", method: http.MethodPut, target: "/boards",
			status: http.StatusMethodNotAllowed, code: CODE_METHOD_NOT_ALLOWED},
		{name: "missing scope", method: http.MethodGet, target: "/boards",
			status: http.StatusBadRequest, code: CODE_BAD_REQUEST},
		{name: "invalid scope", method: http.MethodGet, target: "/boards?scope=planet",
			status: http.StatusBadRequest, code: CODE_BAD_REQUEST},
		{name: "missing workspace", method: http.MethodGet, target: "/boards?scope=workspace",
			status: http.StatusBadRequest, code: CODE_BAD_REQUEST},
		{name: "negative since", method: http.MethodGet,
			target: "/boards?scope=workspace&workspace=%2Fws&since=-1",
			status: http.StatusBadRequest, code: CODE_BAD_REQUEST},
		{name: "invalid json", method: http.MethodPost, target: "/boards", body: []byte("{"),
			status: http.StatusBadRequest, code: CODE_BAD_REQUEST},
		{name: "missing session", method: http.MethodPost, target: "/boards",
			body:   boardBody(t, "workspace", "/ws/a", "", []map[string]any{setOps("k", "v")}),
			status: http.StatusBadRequest, code: CODE_BAD_REQUEST},
		{name: "too many ops", method: http.MethodPost, target: "/boards",
			body:   boardBody(t, "workspace", "/ws/a", "session-a", manyOps()),
			status: http.StatusBadRequest, code: CODE_BAD_REQUEST},
		{name: "invalid key", method: http.MethodPost, target: "/boards",
			body:   boardBody(t, "workspace", "/ws/a", "session-a", []map[string]any{setOps("a\nb", "v")}),
			status: http.StatusBadRequest, code: CODE_BAD_REQUEST},
		{name: "oversize value", method: http.MethodPost, target: "/boards",
			body: boardBody(t, "workspace", "/ws/a", "session-a",
				[]map[string]any{setOps("k", strings.Repeat("x", sessionboard.MAX_VALUE_BYTES))}),
			status: http.StatusBadRequest, code: CODE_BAD_REQUEST},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			recorder := env.call(t, testCase.method, testCase.target, testCase.body)
			if recorder.Code != testCase.status {
				t.Fatalf("status = %d, want %d (%s)", recorder.Code, testCase.status, recorder.Body.String())
			}
			payload := decodeBody(t, recorder)
			if payload["ok"] != false {
				t.Fatalf("ok = %v, want false", payload["ok"])
			}
			if code := errorCode(t, payload); code != testCase.code {
				t.Fatalf("code = %q, want %q", code, testCase.code)
			}
			if message, _ := payload["error"].(map[string]any)["message"].(string); message == "" {
				t.Fatal("error.message 为空")
			}
		})
	}
}

// manyOps builds a batch above the op cap.
func manyOps() []map[string]any {
	ops := make([]map[string]any, 0, sessionboard.MAX_OPS+1)
	for index := 0; index <= sessionboard.MAX_OPS; index++ {
		ops = append(ops, setOps("k"+strconv.Itoa(index), "v"))
	}
	return ops
}

// TestBoardsPersistFailure covers the 500 branch: a failed save maps to
// persist_failed and the read view stays empty (K5/K6).
func TestBoardsPersistFailure(t *testing.T) {
	env := newTestEnv(t, Options{})
	blocked := filepath.Join(t.TempDir(), "blocked")
	if err := os.Mkdir(blocked, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	env.runtime.SessionBoards = sessionboard.NewStore(blocked)
	recorder := env.call(t, http.MethodPost, "/boards",
		boardBody(t, "workspace", "/ws/a", "session-a", []map[string]any{setOps("k", "v")}))
	if recorder.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500 (%s)", recorder.Code, recorder.Body.String())
	}
	payload := decodeBody(t, recorder)
	if code := errorCode(t, payload); code != CODE_PERSIST_FAILED {
		t.Fatalf("code = %q, want %q", code, CODE_PERSIST_FAILED)
	}
	read := decodeBody(t, env.call(t, http.MethodGet, boardPath("workspace", "/ws/a"), nil))
	if read["board"].(map[string]any)["version"] != float64(0) {
		t.Fatalf("落盘失败后板被改了: %v", read["board"])
	}
}

// TestBoardsForbiddenEnvelope covers the 403 branch of the error mapping (K2).
func TestBoardsForbiddenEnvelope(t *testing.T) {
	resp := boardErrorResponse(&sessionboard.Error{Code: sessionboard.CODE_FORBIDDEN, Message: "x"})
	if resp.status != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", resp.status)
	}
	payload, ok := resp.body["error"].(map[string]any)
	if !ok || payload["code"] != CODE_FORBIDDEN || resp.body["ok"] != false {
		t.Fatalf("信封 = %v", resp.body)
	}
}

// TestBoardsRoundTrip covers one write/read cycle, the no-change answer and the
// identity-derived region: an extra "region" field cannot redirect a write (K2).
func TestBoardsRoundTrip(t *testing.T) {
	env := newTestEnv(t, Options{})
	body, err := json.Marshal(map[string]any{"scope": "workspace", "workspace": "/ws/a",
		"session": "session-a", "region": "victim",
		"ops": []map[string]any{setOps("任务.当前", "写规划")}})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	recorder := env.call(t, http.MethodPost, "/boards", body)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d (%s)", recorder.Code, recorder.Body.String())
	}
	payload := decodeBody(t, recorder)
	if payload["changed"] != true || payload["version"] != float64(1) || payload["updated_at"] == "" {
		t.Fatalf("写响应 = %v", payload)
	}
	if trimmed, ok := payload["trimmed"].([]any); !ok || len(trimmed) != 0 {
		t.Fatalf("trimmed = %v", payload["trimmed"])
	}
	if evicted, ok := payload["evicted"].([]any); !ok || len(evicted) != 0 {
		t.Fatalf("evicted = %v", payload["evicted"])
	}

	read := decodeBody(t, env.call(t, http.MethodGet, boardPath("workspace", "/ws/a")+"&since=0", nil))
	if read["ok"] != true || read["since"] != float64(0) || read["truncated"] != false {
		t.Fatalf("读响应 = %v", read)
	}
	regions := read["board"].(map[string]any)["regions"].([]any)
	if len(regions) != 1 || regions[0].(map[string]any)["id"] != "session-a" {
		t.Fatalf("region 由 session 推导，不能是请求里的 region: %v", regions)
	}
	entry := regions[0].(map[string]any)["entries"].([]any)[0].(map[string]any)
	if entry["key"] != "任务.当前" || entry["to"] != "写规划" || entry["by"] != "session-a" {
		t.Fatalf("entry = %v", entry)
	}
	deltas, ok := read["deltas"].([]any)
	if !ok || len(deltas) != 1 {
		t.Fatalf("deltas = %v", read["deltas"])
	}

	// A full read omits deltas entirely (K4: no since -> deltas omitted).
	full := decodeBody(t, env.call(t, http.MethodGet, boardPath("workspace", "/ws/a"), nil))
	if _, exists := full["deltas"]; exists {
		t.Fatalf("无 since 时不应返回 deltas: %v", full)
	}

	// Same value again is not a change: no version bump (K5).
	same := decodeBody(t, env.call(t, http.MethodPost, "/boards",
		boardBody(t, "workspace", "/ws/a", "session-a", []map[string]any{setOps("任务.当前", "写规划")})))
	if same["changed"] != false || same["version"] != float64(1) {
		t.Fatalf("同值重写 = %v", same)
	}

	// A delete writes a tombstone that shows up in deltas only (K1/K4).
	deleted := decodeBody(t, env.call(t, http.MethodPost, "/boards",
		boardBody(t, "workspace", "/ws/a", "session-a",
			[]map[string]any{{"op": "delete", "key": "任务.当前"}})))
	if deleted["changed"] != true || deleted["version"] != float64(2) {
		t.Fatalf("删除 = %v", deleted)
	}
	after := decodeBody(t, env.call(t, http.MethodGet, boardPath("workspace", "/ws/a")+"&since=1", nil))
	deltas = after["deltas"].([]any)
	if len(deltas) != 1 || deltas[0].(map[string]any)["deleted"] != true {
		t.Fatalf("tombstone delta = %v", deltas)
	}
}

// TestBoardsEmptyAndGlobal covers the empty-board answer and the global board's
// workspace-owned region (K4/K1).
func TestBoardsEmptyAndGlobal(t *testing.T) {
	env := newTestEnv(t, Options{})
	empty := decodeBody(t, env.call(t, http.MethodGet, boardPath("workspace", "/ws/unknown")+"&since=0", nil))
	board := empty["board"].(map[string]any)
	if board["version"] != float64(0) || len(board["regions"].([]any)) != 0 {
		t.Fatalf("空板 = %v", board)
	}
	if deltas, ok := empty["deltas"].([]any); !ok || len(deltas) != 0 {
		t.Fatalf("空板 deltas = %v", empty["deltas"])
	}

	recorder := env.call(t, http.MethodPost, "/boards",
		boardBody(t, "global", "/ws/a", "session-a", []map[string]any{setOps("项目.状态", "进行中")}))
	if recorder.Code != http.StatusOK {
		t.Fatalf("global status = %d (%s)", recorder.Code, recorder.Body.String())
	}
	read := decodeBody(t, env.call(t, http.MethodGet, boardPath("global", ""), nil))
	board = read["board"].(map[string]any)
	if board["scope"] != "global" || board["scope_id"] != "*" {
		t.Fatalf("global board = %v", board)
	}
	region := board["regions"].([]any)[0].(map[string]any)
	if region["id"] != "/ws/a" {
		t.Fatalf("global region id = %v, want 工作区键", region["id"])
	}
	owner := region["owner"].(map[string]any)
	if owner["kind"] != "workspace" || owner["id"] != "/ws/a" {
		t.Fatalf("global owner = %v", owner)
	}
}

// panelBody builds one POST /boards/panel body (K13).
func panelBody(t *testing.T, scope, workspace, region string, ops []map[string]any) []byte {
	t.Helper()
	body, err := json.Marshal(map[string]any{"scope": scope, "workspace": workspace,
		"region": region, "ops": ops})
	if err != nil {
		t.Fatalf("marshal panel body: %v", err)
	}
	return body
}

// TestPanelRoute covers POST /boards/panel end to end (K13): the explicit region
// is created, entries carry by="面板", a no-change rewrite keeps the version, a
// clear tombstones the region and other methods are refused.
func TestPanelRoute(t *testing.T) {
	env := newTestEnv(t, Options{})
	body := panelBody(t, "workspace", "/ws/panel", "session-other",
		[]map[string]any{setOps("任务.当前", "面板写入")})
	recorder := env.call(t, http.MethodPost, "/boards/panel", body)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
	payload := decodeBody(t, recorder)
	if payload["ok"] != true || payload["changed"] != true || payload["version"] != float64(1) ||
		payload["updated_at"] == "" {
		t.Fatalf("panel 写响应 = %v", payload)
	}
	if trimmed, ok := payload["trimmed"].([]any); !ok || len(trimmed) != 0 {
		t.Fatalf("trimmed = %v", payload["trimmed"])
	}
	if evicted, ok := payload["evicted"].([]any); !ok || len(evicted) != 0 {
		t.Fatalf("evicted = %v", payload["evicted"])
	}

	read := decodeBody(t, env.call(t, http.MethodGet, boardPath("workspace", "/ws/panel"), nil))
	regions := read["board"].(map[string]any)["regions"].([]any)
	if len(regions) != 1 || regions[0].(map[string]any)["id"] != "session-other" {
		t.Fatalf("panel 区域 = %v", regions)
	}
	entry := regions[0].(map[string]any)["entries"].([]any)[0].(map[string]any)
	if entry["by"] != sessionboard.PANEL_BY {
		t.Fatalf("by = %v, want %q", entry["by"], sessionboard.PANEL_BY)
	}

	// Same value again is not a change: no version bump, no updated_at (K13/K5).
	same := decodeBody(t, env.call(t, http.MethodPost, "/boards/panel", body))
	if same["changed"] != false || same["version"] != float64(1) {
		t.Fatalf("同值重写 = %v", same)
	}
	if _, exists := same["updated_at"]; exists {
		t.Fatalf("无净变化不应返回 updated_at: %v", same)
	}

	// clear is the panel's clear-region op.
	clearBody := panelBody(t, "workspace", "/ws/panel", "session-other",
		[]map[string]any{{"op": "clear"}})
	cleared := decodeBody(t, env.call(t, http.MethodPost, "/boards/panel", clearBody))
	if cleared["changed"] != true || cleared["version"] != float64(2) {
		t.Fatalf("clear = %v", cleared)
	}
	after := decodeBody(t, env.call(t, http.MethodGet, boardPath("workspace", "/ws/panel"), nil))
	entries := after["board"].(map[string]any)["regions"].([]any)[0].(map[string]any)["entries"].([]any)
	if len(entries) != 0 {
		t.Fatalf("clear 后 live 条目 = %v", entries)
	}

	// Wrong method: 405 with the registered envelope and Allow header (K13).
	wrong := env.call(t, http.MethodGet, "/boards/panel", nil)
	if wrong.Code != http.StatusMethodNotAllowed || wrong.Header().Get("Allow") != http.MethodPost {
		t.Fatalf("405 = %d allow=%q", wrong.Code, wrong.Header().Get("Allow"))
	}
	if code := errorCode(t, decodeBody(t, wrong)); code != CODE_METHOD_NOT_ALLOWED {
		t.Fatalf("405 code = %q", code)
	}
}

// TestPanelRouteValidation covers the K13 error matrix on the route: every bad
// shape is a 400 with the subcode in the message and no state change, a missing
// store is 500 internal and a failed save is 500 persist_failed.
func TestPanelRouteValidation(t *testing.T) {
	env := newTestEnv(t, Options{})
	cases := []struct {
		name string
		body []byte
		want string
	}{
		{"invalid scope", panelBody(t, "planet", "/ws/a", "region-a",
			[]map[string]any{setOps("k", "v")}), "invalid_scope"},
		{"missing workspace", panelBody(t, "workspace", "", "region-a",
			[]map[string]any{setOps("k", "v")}), "workspace 不能为空"},
		{"missing region", panelBody(t, "workspace", "/ws/a", "",
			[]map[string]any{setOps("k", "v")}), "region 不能为空"},
		{"global region mismatch", panelBody(t, "global", "/ws/a", "/ws/b",
			[]map[string]any{setOps("k", "v")}), "region 必须等于 workspace"},
		{"empty ops", panelBody(t, "workspace", "/ws/a", "region-a", nil), "invalid_ops"},
		{"invalid json", []byte("{"), "合法 JSON"},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			recorder := env.call(t, http.MethodPost, "/boards/panel", testCase.body)
			if recorder.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
			}
			payload := decodeBody(t, recorder)
			if payload["ok"] != false {
				t.Fatalf("ok = %v, want false", payload["ok"])
			}
			if code := errorCode(t, payload); code != CODE_BAD_REQUEST {
				t.Fatalf("code = %q, want %q", code, CODE_BAD_REQUEST)
			}
			message, _ := payload["error"].(map[string]any)["message"].(string)
			if !strings.Contains(message, testCase.want) {
				t.Fatalf("message = %q, want 含 %q", message, testCase.want)
			}
		})
	}
	if read := decodeBody(t, env.call(t, http.MethodGet, boardPath("workspace", "/ws/a"), nil)); read["board"].(map[string]any)["version"] != float64(0) {
		t.Fatalf("被拒绝的请求改了板: %v", read["board"])
	}

	// Without a store the write answers 500 internal, like the other writers.
	env.runtime.SessionBoards = nil
	noStore := env.call(t, http.MethodPost, "/boards/panel",
		panelBody(t, "workspace", "/ws/a", "region-a", []map[string]any{setOps("k", "v")}))
	if noStore.Code != http.StatusInternalServerError {
		t.Fatalf("无 store status = %d", noStore.Code)
	}
	if code := errorCode(t, decodeBody(t, noStore)); code != CODE_INTERNAL {
		t.Fatalf("无 store code = %q, want %q", code, CODE_INTERNAL)
	}

	// A failed save maps to 500 persist_failed (K13/K9).
	blocked := filepath.Join(t.TempDir(), "blocked")
	if err := os.Mkdir(blocked, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	env.runtime.SessionBoards = sessionboard.NewStore(blocked)
	failed := env.call(t, http.MethodPost, "/boards/panel",
		panelBody(t, "workspace", "/ws/a", "region-a", []map[string]any{setOps("k", "v")}))
	if failed.Code != http.StatusInternalServerError {
		t.Fatalf("落盘失败 status = %d", failed.Code)
	}
	if code := errorCode(t, decodeBody(t, failed)); code != CODE_PERSIST_FAILED {
		t.Fatalf("落盘失败 code = %q, want %q", code, CODE_PERSIST_FAILED)
	}
}

// TestWorkspacesRoute covers GET /boards/workspaces (K14): an empty array
// without boards, the workspace rows ascending with a numeric region count and
// no global board or entry contents.
func TestWorkspacesRoute(t *testing.T) {
	env := newTestEnv(t, Options{})
	empty := decodeBody(t, env.call(t, http.MethodGet, "/boards/workspaces", nil))
	if empty["ok"] != true {
		t.Fatalf("空列表响应 = %v", empty)
	}
	if workspaces, ok := empty["workspaces"].([]any); !ok || len(workspaces) != 0 {
		t.Fatalf("空列表 = %v", empty["workspaces"])
	}

	// Write the workspaces out of order so the route's sort is exercised.
	for _, workspace := range []string{"/ws/b", "/ws/a"} {
		recorder := env.call(t, http.MethodPost, "/boards/panel",
			panelBody(t, "workspace", workspace, "region-x", []map[string]any{setOps("k", "v")}))
		if recorder.Code != http.StatusOK {
			t.Fatalf("写 %s status = %d (%s)", workspace, recorder.Code, recorder.Body.String())
		}
	}
	global := env.call(t, http.MethodPost, "/boards/panel",
		panelBody(t, "global", "/ws/c", "", []map[string]any{setOps("g", "1")}))
	if global.Code != http.StatusOK {
		t.Fatalf("global status = %d (%s)", global.Code, global.Body.String())
	}

	payload := decodeBody(t, env.call(t, http.MethodGet, "/boards/workspaces", nil))
	workspaces := payload["workspaces"].([]any)
	if len(workspaces) != 2 {
		t.Fatalf("工作区数 = %d, want 2（global 不列出）", len(workspaces))
	}
	first := workspaces[0].(map[string]any)
	if first["workspace"] != "/ws/a" || first["version"] != float64(1) ||
		first["regions"] != float64(1) || first["updated_at"] == "" {
		t.Fatalf("首行 = %v", first)
	}
	if _, exists := first["entries"]; exists {
		t.Fatalf("列表暴露了 entry 内容: %v", first)
	}

	// A second region raises the numeric region count and the board version.
	second := env.call(t, http.MethodPost, "/boards/panel",
		panelBody(t, "workspace", "/ws/a", "region-y", []map[string]any{setOps("k2", "v2")}))
	if second.Code != http.StatusOK {
		t.Fatalf("第二区 status = %d (%s)", second.Code, second.Body.String())
	}
	updated := decodeBody(t, env.call(t, http.MethodGet, "/boards/workspaces", nil))
	first = updated["workspaces"].([]any)[0].(map[string]any)
	if first["version"] != float64(2) || first["regions"] != float64(2) {
		t.Fatalf("更新后首行 = %v", first)
	}

	// Wrong method: 405 with Allow: GET (K14).
	wrong := env.call(t, http.MethodPost, "/boards/workspaces", nil)
	if wrong.Code != http.StatusMethodNotAllowed || wrong.Header().Get("Allow") != http.MethodGet {
		t.Fatalf("405 = %d allow=%q", wrong.Code, wrong.Header().Get("Allow"))
	}
}

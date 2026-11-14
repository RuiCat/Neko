// store_test.go covers the store behavior: empty boards, write/read round
// trips, no-net-change writes, persistence-before-commit, fail-soft load and
// reload from disk (K4/K5/K6).
package sessionboard

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

// newTestStore returns one store in a temp directory whose clock advances one
// minute per write, so eviction and trim order are deterministic.
func newTestStore(t *testing.T) *Store {
	t.Helper()
	store := NewStore(filepath.Join(t.TempDir(), FILE_NAME))
	base := time.Date(2026, 11, 10, 12, 0, 0, 0, time.FixedZone("CST", 8*60*60))
	step := 0
	store.now = func() time.Time {
		step++
		return base.Add(time.Duration(step) * time.Minute)
	}
	return store
}

// newCaller builds one bridge-injected caller.
func newCaller(session, workspace string) Caller {
	return Caller{SessionID: session, WorkspaceKey: workspace}
}

// rawValue quotes one string into a JSON value.
func rawValue(value string) json.RawMessage {
	return json.RawMessage(strconv.Quote(value))
}

// setRequest builds a workspace write request with one set op per key/value.
func setRequest(t *testing.T, workspace, session string, pairs map[string]string) WriteRequest {
	t.Helper()
	ops := make([]Op, 0, len(pairs))
	for key, value := range pairs {
		ops = append(ops, Op{Op: OP_SET, Key: key, Value: rawValue(value)})
	}
	return WriteRequest{Scope: SCOPE_WORKSPACE, Workspace: workspace,
		Caller: newCaller(session, workspace), Ops: ops}
}

// mustWrite applies one request and fails the test on error.
func mustWrite(t *testing.T, store *Store, request WriteRequest) WriteResult {
	t.Helper()
	result, err := store.Write(request)
	if err != nil {
		t.Fatalf("write: %v", err)
	}
	return result
}

// readBoard reads one scope through the store with an optional cursor.
func readBoard(t *testing.T, store *Store, scope, workspace string, since *int) ReadResult {
	t.Helper()
	result, err := store.Read(ReadQuery{Scope: scope, Workspace: workspace,
		Caller: newCaller("session-a", workspace), Since: since})
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	return result
}

// TestStoreEmptyBoard covers the unknown-workspace answer: a version-0 board
// with no regions, and an empty (never omitted) deltas list when since is set.
func TestStoreEmptyBoard(t *testing.T) {
	store := newTestStore(t)
	result := readBoard(t, store, SCOPE_WORKSPACE, "/ws/unknown", nil)
	if result.Board.Version != 0 || len(result.Board.Regions) != 0 || result.Board.ScopeID != "/ws/unknown" {
		t.Fatalf("board = %+v", result.Board)
	}
	if result.Deltas != nil || result.Truncated {
		t.Fatalf("deltas = %v, truncated = %v", result.Deltas, result.Truncated)
	}
	zero := 0
	withSince := readBoard(t, store, SCOPE_WORKSPACE, "/ws/unknown", &zero)
	if withSince.Deltas == nil || len(withSince.Deltas) != 0 {
		t.Fatalf("deltas = %v", withSince.Deltas)
	}
	global := readBoard(t, store, SCOPE_GLOBAL, "", nil)
	if global.Board.Scope != SCOPE_GLOBAL || global.Board.ScopeID != GLOBAL_SCOPE_ID {
		t.Fatalf("global board = %+v", global.Board)
	}
}

// TestStoreWriteReadRoundTrip covers one workspace write followed by a full
// read and a delta read (K1/K4/K5).
func TestStoreWriteReadRoundTrip(t *testing.T) {
	store := newTestStore(t)
	result := mustWrite(t, store, setRequest(t, "/ws/a", "session-a", map[string]string{"任务.当前": "写规划"}))
	if !result.Changed || result.Version != 1 || result.UpdatedAt == "" {
		t.Fatalf("write result = %+v", result)
	}
	read := readBoard(t, store, SCOPE_WORKSPACE, "/ws/a", nil)
	if read.Board.Version != 1 || len(read.Board.Regions) != 1 {
		t.Fatalf("board = %+v", read.Board)
	}
	region := read.Board.Regions[0]
	if region.ID != "session-a" || region.Owner.Kind != OWNER_SESSION || len(region.Entries) != 1 {
		t.Fatalf("region = %+v", region)
	}
	entry := region.Entries[0]
	if entry.Key != "任务.当前" || entry.To != "写规划" || entry.By != "session-a" || entry.Version != 1 {
		t.Fatalf("entry = %+v", entry)
	}
	zero := 0
	deltas := readBoard(t, store, SCOPE_WORKSPACE, "/ws/a", &zero)
	if len(deltas.Deltas) != 1 || deltas.Truncated {
		t.Fatalf("deltas = %+v truncated=%v", deltas.Deltas, deltas.Truncated)
	}
	one := 1
	if got := readBoard(t, store, SCOPE_WORKSPACE, "/ws/a", &one); len(got.Deltas) != 0 {
		t.Fatalf("游标已到最新仍有 deltas: %+v", got.Deltas)
	}
}

// TestStoreNoNetChangeKeepsFile covers the no-change path: no version bump and
// no disk write at all (K5).
func TestStoreNoNetChangeKeepsFile(t *testing.T) {
	store := newTestStore(t)
	request := setRequest(t, "/ws/a", "session-a", map[string]string{"任务.当前": "写规划"})
	mustWrite(t, store, request)
	path := store.path
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("读文件: %v", err)
	}
	second := mustWrite(t, store, request)
	if second.Changed || second.Version != 1 {
		t.Fatalf("同值重写 = %+v", second)
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("读文件: %v", err)
	}
	if string(before) != string(after) {
		t.Fatal("无净变化却写盘了")
	}
}

// TestStorePersistFailureKeepsMemory covers persistence-before-commit: a failed
// save reports persist_failed and leaves the live board untouched (K5/K6).
func TestStorePersistFailureKeepsMemory(t *testing.T) {
	blocked := filepath.Join(t.TempDir(), "blocked")
	if err := os.Mkdir(blocked, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	store := NewStore(blocked)
	_, err := store.Write(setRequest(t, "/ws/a", "session-a", map[string]string{"k": "v"}))
	code := errorCodeOf(t, err)
	if code != CODE_PERSIST_FAILED {
		t.Fatalf("code = %q, want %q", code, CODE_PERSIST_FAILED)
	}
	read := readBoard(t, store, SCOPE_WORKSPACE, "/ws/a", nil)
	if read.Board.Version != 0 || len(read.Board.Regions) != 0 {
		t.Fatalf("落盘失败后内存被改了: %+v", read.Board)
	}
}

// errorCodeOf extracts the contract code from one store error.
func errorCodeOf(t *testing.T, err error) string {
	t.Helper()
	boardErr, ok := err.(*Error)
	if !ok {
		t.Fatalf("错误类型 = %T, want *Error: %v", err, err)
	}
	return boardErr.Code
}

// TestReload covers the persisted shape: a new store loads the same boards, a
// missing file is empty and a corrupt file is fail-soft empty (K6).
func TestReload(t *testing.T) {
	path := filepath.Join(t.TempDir(), FILE_NAME)
	store := NewStore(path)
	base := time.Date(2026, 11, 10, 12, 0, 0, 0, time.FixedZone("CST", 8*60*60))
	step := 0
	store.now = func() time.Time { step++; return base.Add(time.Duration(step) * time.Minute) }
	mustWrite(t, store, setRequest(t, "/ws/a", "session-a", map[string]string{"a": "1"}))
	mustWrite(t, store, setRequest(t, "/ws/b", "session-b", map[string]string{"b": "2"}))
	globalRequest := WriteRequest{Scope: SCOPE_GLOBAL, Workspace: "/ws/a",
		Caller: newCaller("session-a", "/ws/a"),
		Ops:    []Op{{Op: OP_SET, Key: "g", Value: rawValue("3")}}}
	globalResult := mustWrite(t, store, globalRequest)
	if globalResult.Version != 1 {
		t.Fatalf("global version = %d", globalResult.Version)
	}

	reloaded := NewStore(path)
	if err := reloaded.Load(); err != nil {
		t.Fatalf("load: %v", err)
	}
	for _, workspace := range []string{"/ws/a", "/ws/b"} {
		read := readBoard(t, reloaded, SCOPE_WORKSPACE, workspace, nil)
		if read.Board.Version != 1 || len(read.Board.Regions) != 1 {
			t.Fatalf("重载 %s 板 = %+v", workspace, read.Board)
		}
	}
	global := readBoard(t, reloaded, SCOPE_GLOBAL, "", nil)
	if global.Board.Version != 1 || len(global.Board.Regions) != 1 ||
		global.Board.Regions[0].Owner.Kind != OWNER_WORKSPACE {
		t.Fatalf("重载 global 板 = %+v", global.Board)
	}

	missing := NewStore(filepath.Join(t.TempDir(), FILE_NAME))
	if err := missing.Load(); err != nil {
		t.Fatalf("缺失文件应视为空板: %v", err)
	}
	if read := readBoard(t, missing, SCOPE_GLOBAL, "", nil); read.Board.Version != 0 {
		t.Fatalf("缺失文件版本 = %d", read.Board.Version)
	}

	corruptPath := filepath.Join(t.TempDir(), FILE_NAME)
	if err := os.WriteFile(corruptPath, []byte("{不是 JSON"), 0o644); err != nil {
		t.Fatalf("写损坏文件: %v", err)
	}
	corrupt := NewStore(corruptPath)
	if err := corrupt.Load(); err == nil {
		t.Fatal("损坏文件应返回 error（调用方 fail-soft 记日志）")
	}
	if read := readBoard(t, corrupt, SCOPE_GLOBAL, "", nil); read.Board.Version != 0 {
		t.Fatalf("损坏文件后版本 = %d, want 0", read.Board.Version)
	}
	// After a failed load the store still works: fail-soft means serving, not dead.
	mustWrite(t, corrupt, setRequest(t, "/ws/a", "session-a", map[string]string{"k": "v"}))
}

// TestStoreWriteValidation covers the K5 batch rules: every rejected shape
// reports bad_request with the K5 subcode in the message prefix.
func TestStoreWriteValidation(t *testing.T) {
	store := newTestStore(t)
	valid := setRequest(t, "/ws/a", "session-a", map[string]string{"k": "v"})
	cases := []struct {
		name    string
		request WriteRequest
		want    string
	}{
		{"empty ops", WriteRequest{Scope: SCOPE_WORKSPACE, Workspace: "/ws/a",
			Caller: newCaller("session-a", "/ws/a")}, "invalid_ops"},
		{"clear with others", WriteRequest{Scope: SCOPE_WORKSPACE, Workspace: "/ws/a",
			Caller: newCaller("session-a", "/ws/a"),
			Ops:    []Op{{Op: OP_CLEAR}, {Op: OP_SET, Key: "k", Value: rawValue("v")}}}, "invalid_ops"},
		{"unknown op", WriteRequest{Scope: SCOPE_WORKSPACE, Workspace: "/ws/a",
			Caller: newCaller("session-a", "/ws/a"), Ops: []Op{{Op: "add", Key: "k"}}}, "invalid_ops"},
		{"too many ops", WriteRequest{Scope: SCOPE_WORKSPACE, Workspace: "/ws/a",
			Caller: newCaller("session-a", "/ws/a"), Ops: tooManyOps()}, "too_many_ops"},
		{"blank key", WriteRequest{Scope: SCOPE_WORKSPACE, Workspace: "/ws/a",
			Caller: newCaller("session-a", "/ws/a"),
			Ops:    []Op{{Op: OP_SET, Key: "  ", Value: rawValue("v")}}}, "invalid_key"},
		{"newline key", WriteRequest{Scope: SCOPE_WORKSPACE, Workspace: "/ws/a",
			Caller: newCaller("session-a", "/ws/a"),
			Ops:    []Op{{Op: OP_SET, Key: "a\nb", Value: rawValue("v")}}}, "invalid_key"},
		{"long key", WriteRequest{Scope: SCOPE_WORKSPACE, Workspace: "/ws/a",
			Caller: newCaller("session-a", "/ws/a"),
			Ops:    []Op{{Op: OP_SET, Key: strings.Repeat("k", MAX_KEY_CHARS+1), Value: rawValue("v")}}}, "invalid_key"},
		{"missing value", WriteRequest{Scope: SCOPE_WORKSPACE, Workspace: "/ws/a",
			Caller: newCaller("session-a", "/ws/a"), Ops: []Op{{Op: OP_SET, Key: "k"}}}, "invalid_value"},
		{"oversize value", WriteRequest{Scope: SCOPE_WORKSPACE, Workspace: "/ws/a",
			Caller: newCaller("session-a", "/ws/a"),
			Ops:    []Op{{Op: OP_SET, Key: "k", Value: rawValue(strings.Repeat("x", MAX_VALUE_BYTES))}}}, "invalid_value"},
		{"invalid scope", WriteRequest{Scope: "planet", Workspace: "/ws/a",
			Caller: newCaller("session-a", "/ws/a"), Ops: valid.Ops}, "invalid_scope"},
		{"missing session", WriteRequest{Scope: SCOPE_WORKSPACE, Workspace: "/ws/a",
			Caller: newCaller("", "/ws/a"), Ops: valid.Ops}, "session 不能为空"},
		{"missing workspace", WriteRequest{Scope: SCOPE_WORKSPACE,
			Caller: newCaller("session-a", ""), Ops: valid.Ops}, "workspace 不能为空"},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			_, err := store.Write(testCase.request)
			if err == nil {
				t.Fatal("非法批被接受")
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
}

// tooManyOps builds one batch above the op cap.
func tooManyOps() []Op {
	ops := make([]Op, 0, MAX_OPS+1)
	for index := 0; index <= MAX_OPS; index++ {
		ops = append(ops, Op{Op: OP_SET, Key: "k" + strconv.Itoa(index), Value: rawValue("v")})
	}
	return ops
}

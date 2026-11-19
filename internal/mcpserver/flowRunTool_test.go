// flowRunTool_test.go covers flow_run: the JSON contract, the zero-write
// rejection, the single settlement replay and the state-gate serialization.
package mcpserver

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/RuiCat/Neko/internal/engine"
	"github.com/RuiCat/Neko/internal/entity"
	"github.com/RuiCat/Neko/internal/events"
	"github.com/RuiCat/Neko/internal/flow/condition"
	"github.com/RuiCat/Neko/internal/function"
	"github.com/RuiCat/Neko/internal/registry"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// flowFixture is one temp data dir holding the cropped K1 document, one actor
// and one kitchen material.
type flowFixture struct {
	server  *Server
	store   *entity.Store
	dataDir string
}

// flowFixtureDocument is the cropped m1 artifact: 吃饭 consumes 食物 at the
// kitchen, carries one data-layer effect and a task template. The task stage and
// every reward stay with taskRules.neko, so flow_run only writes data rows.
func flowFixtureDocument() map[string]any {
	return map[string]any{
		"schemaVersion": 1,
		"flows": []any{map[string]any{
			"id":       "吃饭",
			"source":   map[string]any{"file": "fixture/behavior.neko", "line": 1},
			"scene":    "S001-KT",
			"requires": []any{"食物"},
			"cost":     0,
			"consume":  []any{"食物"},
			"produce":  []any{},
			"steps":    []any{"摆好碗筷", "一口一口吃饭"},
			"effects": []any{map[string]any{
				"path":         "标记.饥饿",
				"kind":         "bool",
				"value":        false,
				"source":       map[string]any{"file": "fixture/behavior.neko", "line": 40},
				"target":       "标记.饥饿",
				"targetReason": "状态内核同名字段",
			}},
			"intents": []any{map[string]any{"action": "吃", "target": "食物"}},
		}},
		"taskTemplates": []any{map[string]any{
			"id":        "吃饭",
			"source":    map[string]any{"file": "fixture/task.neko", "line": 2},
			"stages":    []any{"未激活", "进行中", "完成"},
			"condition": map[string]any{"type": "行为", "example": "吃饭"},
			"reward":    map[string]any{"experience": 5, "affinity": 2},
			"dialog":    map[string]any{"activate": "", "inProgress": "", "complete": ""},
		}},
	}
}

// writeFixtureJSON writes one fixture file, creating its directory.
func writeFixtureJSON(t *testing.T, path string, value any) {
	t.Helper()
	raw, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		t.Fatalf("marshal fixture %s: %v", path, err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("create fixture dir: %v", err)
	}
	if err := os.WriteFile(path, raw, 0o644); err != nil {
		t.Fatalf("write fixture %s: %v", path, err)
	}
}

// newFlowFixture builds the temp data dir, the material template, the cropped
// flow spec and the two entities the flow needs.
func newFlowFixture(t *testing.T) *flowFixture {
	t.Helper()
	dataDir := t.TempDir()
	writeFixtureJSON(t, filepath.Join(dataDir, "templates", "素材.json"), map[string]any{
		"名称": "", "所属场景": "", "数量": 1,
	})
	writeFixtureJSON(t, filepath.Join(dataDir, FLOW_SPEC_FILE), flowFixtureDocument())
	store := entity.NewStore(dataDir)
	if err := store.Load(); err != nil {
		t.Fatalf("load store: %v", err)
	}
	if _, err := store.Create("猫猫", "角色基础", map[string]any{
		"位置": map[string]any{"场景": "S001-KT"},
		"交易": map[string]any{"货币": 100},
		"代谢": map[string]any{"胃内容": 300},
		"标记": map[string]any{"饥饿": true},
	}, nil); err != nil {
		t.Fatalf("create actor: %v", err)
	}
	if _, err := store.Create("素材:S001-KT@食物", "素材", map[string]any{
		"名称": "食物", "所属场景": "S001-KT", "数量": 1,
	}, nil); err != nil {
		t.Fatalf("create material: %v", err)
	}
	machine := engine.New(store)
	reg, err := registry.New(store, machine, filepath.Join(t.TempDir(), "scripts.json"), t.TempDir())
	if err != nil {
		t.Fatalf("registry: %v", err)
	}
	server := New(store, machine, reg)
	server.SetDataDir(dataDir)
	return &flowFixture{server: server, store: store, dataDir: dataDir}
}

// callFlowTool invokes flow_run through a real MCP session and decodes the body.
func callFlowTool(t *testing.T, fixture *flowFixture, arguments map[string]any) flowRunResult {
	t.Helper()
	session, cancel := connect(t, fixture.server)
	defer cancel()
	ctx, stop := context.WithTimeout(context.Background(), 10*time.Second)
	defer stop()
	result, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "flow_run", Arguments: arguments})
	if err != nil {
		t.Fatalf("call flow_run: %v", err)
	}
	if len(result.Content) == 0 {
		t.Fatal("flow_run returned no content")
	}
	text := result.Content[0].(*mcp.TextContent).Text
	var body flowRunResult
	if err := json.Unmarshal([]byte(text), &body); err != nil {
		t.Fatalf("flow_run is not JSON: %v (%s)", err, text)
	}
	return body
}

// requireBodyOK asserts one successful body.
func requireBodyOK(t *testing.T, body flowRunResult) {
	t.Helper()
	if !body.OK {
		t.Fatalf("flow_run not ok: %+v", body)
	}
	if body.Error != "" {
		t.Fatalf("ok body carries error: %s", body.Error)
	}
	if body.Completion == nil {
		t.Fatal("ok body has no completion")
	}
	if body.Verdict == nil || !body.Verdict.OK {
		t.Fatalf("ok body has no passing verdict: %+v", body.Verdict)
	}
}

// requireIntRow asserts one integer entity row.
func requireIntRow(t *testing.T, ent *entity.Entity, path string, want int) {
	t.Helper()
	value, err := ent.Get(path)
	if err != nil {
		t.Fatalf("%s: %v", path, err)
	}
	if value != want {
		t.Fatalf("%s = %v (%T), want %d", path, value, value, want)
	}
}

// readPersistedData reads one saved entity's data from its entities file.
func readPersistedData(t *testing.T, path, entityID string) map[string]any {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	var document struct {
		Entities map[string]struct {
			Data map[string]any `json:"data"`
		} `json:"entities"`
	}
	if err := json.Unmarshal(raw, &document); err != nil {
		t.Fatalf("decode %s: %v", path, err)
	}
	record, ok := document.Entities[entityID]
	if !ok {
		t.Fatalf("%s has no entity %q", path, entityID)
	}
	return record.Data
}

// persistedPath resolves one dotted path inside persisted entity data.
func persistedPath(data map[string]any, path string) (any, bool) {
	var current any = data
	for _, key := range strings.Split(path, ".") {
		object, ok := current.(map[string]any)
		if !ok {
			return nil, false
		}
		current, ok = object[key]
		if !ok {
			return nil, false
		}
	}
	return current, true
}

// requirePersistedNumber asserts one numeric row read back from disk.
func requirePersistedNumber(t *testing.T, data map[string]any, path string, want float64) {
	t.Helper()
	value, ok := persistedPath(data, path)
	if !ok {
		t.Fatalf("persisted %s is missing", path)
	}
	number, ok := value.(float64)
	if !ok || number != want {
		t.Fatalf("persisted %s = %v (%T), want %v", path, value, value, want)
	}
}

// requireNoFile asserts a path was never written.
func requireNoFile(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("%s exists after a rejected call: %v", path, err)
	}
}

// ledgerDocument is the on-disk ledger view the replay assertions decode.
type ledgerDocument struct {
	Version     int               `json:"version"`
	Settlements []json.RawMessage `json:"settlements"`
}

// TestFlowRunSettlesAndPersists covers the tool registration, the full
// judge → step → settle → task walk and the persisted rows.
func TestFlowRunSettlesAndPersists(t *testing.T) {
	fixture := newFlowFixture(t)
	session := mustFlowSession(t, fixture)
	if !hasTool(session, "flow_run") {
		t.Fatal("flow_run not registered")
	}

	body := callFlowTool(t, fixture, map[string]any{"entity": "猫猫", "flow": "吃饭"})
	t.Logf("flow_run body: %s", jsonString(body))
	requireBodyOK(t, body)
	if body.Completion.FlowID != "吃饭" || body.Completion.Entity != "猫猫" {
		t.Fatalf("completion = %+v", body.Completion)
	}
	if body.Completion.Result != "完成" {
		t.Fatalf("completion result = %q", body.Completion.Result)
	}
	if len(body.Report.Changes) == 0 {
		t.Fatal("report has no change rows")
	}

	actor := fixture.store.Get("猫猫")
	requireIntRow(t, actor, "代谢.胃内容", 300)
	requireIntRow(t, actor, "任务.进度", 100)
	if value := actor.GetOrNone("成长.经验"); value != nil && value != 0 {
		t.Fatalf("成长.经验 = %v, want no reward from flow_run", value)
	}
	if value := actor.GetOrNone("关系.数值.好感度"); value != nil && value != 0 {
		t.Fatalf("关系.数值.好感度 = %v, want no reward from flow_run", value)
	}
	if current, err := actor.Get("任务.当前"); err != nil || current != "吃饭" {
		t.Fatalf("任务.当前 = %v (%v), want 吃饭", current, err)
	}
	if material := fixture.store.Get("素材:S001-KT@食物"); material == nil {
		t.Fatal("consumed material entity disappeared")
	} else {
		requireIntRow(t, material, "数量", 1)
	}

	public := readPersistedData(t, filepath.Join(fixture.dataDir, "entities.json"), "素材:S001-KT@食物")
	requirePersistedNumber(t, public, "数量", 1)
	user := readPersistedData(t, filepath.Join(fixture.dataDir, "user", "entities.json"), "猫猫")
	requirePersistedNumber(t, user, "代谢.胃内容", 300)
	requirePersistedNumber(t, user, "任务.进度", 100)
}

// TestFlowRunUnknownFlow refuses a flow absent from the spec without writing.
func TestFlowRunUnknownFlow(t *testing.T) {
	fixture := newFlowFixture(t)
	before := fixture.store.Get("猫猫").Snapshot()
	body := callFlowTool(t, fixture, map[string]any{"entity": "猫猫", "flow": "买水"})
	if body.OK {
		t.Fatalf("unknown flow succeeded: %+v", body)
	}
	if !strings.Contains(body.Error, "买水") || !strings.Contains(body.Error, FLOW_SPEC_FILE) {
		t.Fatalf("unknown-flow error = %q", body.Error)
	}
	if !reflect.DeepEqual(before, fixture.store.Get("猫猫").Snapshot()) {
		t.Fatal("unknown flow mutated the actor")
	}
	requireNoFile(t, filepath.Join(fixture.dataDir, "entities.json"))
	requireNoFile(t, filepath.Join(fixture.dataDir, "flow-ledger.json"))
}

// TestFlowRunMissingSpec reports the placement path and writes nothing.
func TestFlowRunMissingSpec(t *testing.T) {
	fixture := newFlowFixture(t)
	specPath := filepath.Join(fixture.dataDir, FLOW_SPEC_FILE)
	if err := os.Remove(specPath); err != nil {
		t.Fatalf("remove spec: %v", err)
	}
	body := callFlowTool(t, fixture, map[string]any{"entity": "猫猫", "flow": "吃饭"})
	if body.OK || body.Error == "" {
		t.Fatalf("missing spec body = %+v", body)
	}
	if !strings.Contains(body.Error, specPath) || !strings.Contains(body.Error, "放到") {
		t.Fatalf("missing-spec error does not name the placement path: %q", body.Error)
	}
	requireNoFile(t, filepath.Join(fixture.dataDir, "entities.json"))
	requireNoFile(t, filepath.Join(fixture.dataDir, "flow-ledger.json"))
}

// TestFlowRunRejectsWithoutWrites covers the judgment rejection: the body keeps
// the reasons and the entity, the item and the disk stay untouched.
func TestFlowRunRejectsWithoutWrites(t *testing.T) {
	fixture := newFlowFixture(t)
	if !fixture.store.Delete("素材:S001-KT@食物") {
		t.Fatal("fixture material missing")
	}
	before := fixture.store.Get("猫猫").Snapshot()
	body := callFlowTool(t, fixture, map[string]any{"entity": "猫猫", "flow": "吃饭"})
	if body.OK {
		t.Fatalf("rejected flow reported ok: %+v", body)
	}
	if body.Error != "" {
		t.Fatalf("judgment rejection should carry verdict reasons, not error: %q", body.Error)
	}
	if body.Verdict == nil || body.Verdict.OK || len(body.Verdict.Reasons) == 0 {
		t.Fatalf("verdict = %+v", body.Verdict)
	}
	if body.Verdict.Reasons[0].Code != condition.REASON_MISSING_ITEM {
		t.Fatalf("reason code = %q", body.Verdict.Reasons[0].Code)
	}
	if body.Completion != nil {
		t.Fatalf("rejected body carries results: %+v", body)
	}
	if !reflect.DeepEqual(before, fixture.store.Get("猫猫").Snapshot()) {
		t.Fatal("judgment rejection mutated the actor")
	}
	requireNoFile(t, filepath.Join(fixture.dataDir, "entities.json"))
	requireNoFile(t, filepath.Join(fixture.dataDir, "flow-ledger.json"))
}

// TestFlowRunReplaysOnce covers idempotency: the second call returns the recorded
// completion, writes nothing and does not pay the reward twice.
func TestFlowRunReplaysOnce(t *testing.T) {
	fixture := newFlowFixture(t)
	first := fixture.server.RunFlow("猫猫", "吃饭", "")
	requireBodyOK(t, first)
	afterFirst := fixture.store.Get("猫猫").Snapshot()
	userPath := filepath.Join(fixture.dataDir, "user", "entities.json")
	publicPath := filepath.Join(fixture.dataDir, "entities.json")
	userFirst := readFixtureBytes(t, userPath)
	publicFirst := readFixtureBytes(t, publicPath)

	second := fixture.server.RunFlow("猫猫", "吃饭", "")
	if !second.OK {
		t.Fatalf("replay failed: %+v", second)
	}
	if second.Completion == nil || *second.Completion != *first.Completion {
		t.Fatalf("replay completion = %+v, first = %+v", second.Completion, first.Completion)
	}
	if second.Report == nil || len(second.Report.Changes) != 0 || len(second.Report.Errors) != 0 {
		t.Fatalf("replay report = %+v", second.Report)
	}
	if second.Verdict == nil {
		t.Fatal("replay body has no verdict")
	}
	if !reflect.DeepEqual(afterFirst, fixture.store.Get("猫猫").Snapshot()) {
		t.Fatal("replay mutated the actor")
	}
	if !bytes.Equal(userFirst, readFixtureBytes(t, userPath)) || !bytes.Equal(publicFirst, readFixtureBytes(t, publicPath)) {
		t.Fatal("replay rewrote the entity files")
	}
	if value := fixture.store.Get("猫猫").GetOrNone("成长.经验"); value != nil && value != 0 {
		t.Fatalf("成长.经验 = %v, want no replay reward", value)
	}

	ledger := readLedgerDocument(t, filepath.Join(fixture.dataDir, FLOW_LEDGER_FILE))
	if len(ledger.Settlements) != 1 {
		t.Fatalf("ledger settlements = %d, want 1", len(ledger.Settlements))
	}
}

// TestFlowRunWaitsForStateGate proves the tool body runs under WithStateWrite:
// a held gate blocks flow_run until it is released, and the call then settles.
func TestFlowRunWaitsForStateGate(t *testing.T) {
	fixture := newFlowFixture(t)
	release := make(chan struct{})
	gateHeld := make(chan struct{})
	gateDone := make(chan error, 1)
	go func() {
		gateDone <- fixture.server.WithStateWrite(func() error {
			close(gateHeld)
			<-release
			return nil
		})
	}()
	<-gateHeld

	runDone := make(chan flowRunResult, 1)
	go func() { runDone <- fixture.server.RunFlow("猫猫", "吃饭", "") }()
	select {
	case body := <-runDone:
		close(release)
		t.Fatalf("flow_run ran while the state gate was held: %+v", body)
	case <-time.After(100 * time.Millisecond):
	}

	close(release)
	if err := <-gateDone; err != nil {
		t.Fatalf("state gate: %v", err)
	}
	select {
	case body := <-runDone:
		requireBodyOK(t, body)
	case <-time.After(5 * time.Second):
		t.Fatal("flow_run did not finish after the gate was released")
	}
}

// mustFlowSession connects one MCP session to the fixture server.
func mustFlowSession(t *testing.T, fixture *flowFixture) *mcp.ClientSession {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	serverTransport, clientTransport := mcp.NewInMemoryTransports()
	go func() { _ = fixture.server.MCPServer().Run(ctx, serverTransport) }()
	client := mcp.NewClient(&mcp.Implementation{Name: "flow-test-client", Version: "0.1.0"}, nil)
	session, err := client.Connect(ctx, clientTransport, nil)
	if err != nil {
		cancel()
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(cancel)
	return session
}

// hasTool reports whether the session lists a tool by name.
func hasTool(session *mcp.ClientSession, name string) bool {
	tools, err := session.ListTools(context.Background(), nil)
	if err != nil {
		return false
	}
	for _, tool := range tools.Tools {
		if tool.Name == name {
			return true
		}
	}
	return false
}

// readFixtureBytes reads one file for byte-identity assertions.
func readFixtureBytes(t *testing.T, path string) []byte {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return raw
}

// readLedgerDocument decodes the flow ledger written by the tool.
func readLedgerDocument(t *testing.T, path string) ledgerDocument {
	t.Helper()
	var document ledgerDocument
	if err := json.Unmarshal(readFixtureBytes(t, path), &document); err != nil {
		t.Fatalf("decode ledger: %v", err)
	}
	return document
}

// TestFlowRunThenTick proves the corrected split: flow_run writes only the
// interaction/task data rows, and the following .neko tick owns the computed
// 代谢.胃内容 and the task stage (taskRules.neko).
func TestFlowRunThenTick(t *testing.T) {
	dataDir := t.TempDir()
	writeFixtureJSON(t, filepath.Join(dataDir, "templates", "素材.json"), map[string]any{
		"名称": "", "所属场景": "", "数量": 1, "描述": "",
		"持有人": "", "食物": map[string]any{"可食": false},
	})
	writeFixtureJSON(t, filepath.Join(dataDir, FLOW_SPEC_FILE), flowFixtureDocument())
	store := entity.NewStore(dataDir)
	if err := store.Load(); err != nil {
		t.Fatalf("load store: %v", err)
	}
	if _, err := store.Create("猫猫", "角色基础", map[string]any{
		"基础": map[string]any{"名字": "猫猫", "代谢修正": 1000},
		"位置": map[string]any{"场景": "S001-KT"},
		"代谢": map[string]any{"胃内容": 999, "流逝分钟": 1, "基础代谢": 1200},
		"器官性状": map[string]any{"胃": map[string]any{"内容": []any{
			map[string]any{"能量": 300, "水分": 100, "重量": 100, "温度": 37},
		}}},
		"任务": map[string]any{"当前": "", "进度": 0, "阶段": "未激活", "完成标记": false},
		"标记": map[string]any{"饥饿": true},
	}, nil); err != nil {
		t.Fatalf("create actor: %v", err)
	}
	if _, err := store.Create("素材:S001-KT@食物", "素材", map[string]any{
		"名称": "食物", "所属场景": "S001-KT", "数量": 1, "持有人": "猫猫",
		"食物": map[string]any{"可食": true},
	}, nil); err != nil {
		t.Fatalf("create material: %v", err)
	}
	machine := engine.New(store)
	funcs := function.New(store)
	machine.SetFuncCaller(funcs)
	system := events.NewSystem(store, funcs)
	reg, err := registry.New(store, machine, filepath.Join(dataDir, "scripts.json"), filepath.Join("..", "..", "scripts"))
	if err != nil {
		t.Fatalf("registry: %v", err)
	}
	reg.SetFunctionSink(system)
	if _, err := reg.BootstrapFromSources(); err != nil {
		t.Fatalf("bootstrap scripts: %v", err)
	}
	server := New(store, machine, reg)
	server.SetDataDir(dataDir)

	body := server.RunFlow("猫猫", "吃饭", "")
	requireBodyOK(t, body)
	for _, change := range body.Report.Changes {
		if strings.HasPrefix(change.Path, "代谢.") || strings.HasPrefix(change.Path, "行为.") || strings.HasPrefix(change.Path, "睡眠.") {
			t.Fatalf("flow_run wrote a computed row: %+v", change)
		}
	}
	actor := store.Get("猫猫")
	before := actor.GetOrNone("代谢.胃内容")
	if before != 999 {
		t.Fatalf("pre-tick 代谢.胃内容 = %v, want 999 (flow_run must not compute it)", before)
	}
	if stage := actor.GetOrNone("任务.阶段"); stage != "未激活" {
		t.Fatalf("pre-tick 任务.阶段 = %v, want 未激活 (flow_run only writes data rows)", stage)
	}
	if action := actor.GetOrNone("交互.动作"); action != "吃" {
		t.Fatalf("交互.动作 = %v, want 吃", action)
	}
	if target := actor.GetOrNone("交互.目标"); target != "食物" {
		t.Fatalf("交互.目标 = %v, want 食物", target)
	}

	machine.Tick("")
	after := actor.GetOrNone("代谢.胃内容")
	if after == before {
		t.Fatalf("tick did not own 代谢.胃内容: still %v", after)
	}
	if stage := actor.GetOrNone("任务.阶段"); stage == "未激活" || stage == "" {
		t.Fatalf("taskRules did not advance 任务.阶段: %v", stage)
	}
}

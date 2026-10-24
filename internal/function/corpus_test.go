// Coverage tests that run the real .neko function bodies from scripts:
// every corpus definition is parsed by neko.Parse, registered and executed.
package function

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/ruicat/astrbox/internal/entity"
	"github.com/ruicat/astrbox/internal/neko"
)

// corpusDir is the production script directory relative to this package.
const corpusDir = "../../scripts"

// corpusScripts lists the seven scripts that contain the 15 corpus functions.
var corpusScripts = []string{
	"cat_body_state.neko",
	"cat_core_v2.neko",
	"cat_daily.neko",
	"cat_emote.neko",
	"cat_season.neko",
	"scene_assets.neko",
	"scene_move.neko",
}

// loadCorpusFuncs parses one corpus script and returns its @函数 definitions.
func loadCorpusFuncs(t *testing.T, script string) []neko.FuncDef {
	t.Helper()
	raw, err := os.ReadFile(corpusFilePath(t, script))
	if err != nil {
		t.Fatalf("读取语料 %s 失败: %v", script, err)
	}
	ast := neko.Parse(string(raw))
	if len(ast.FuncDefs) == 0 {
		t.Fatalf("%s 未解析出 @函数", script)
	}
	return ast.FuncDefs
}

// corpusFilePath finds a script by basename under the (nested) corpus dir.
func corpusFilePath(t *testing.T, name string) string {
	t.Helper()
	var found string
	err := filepath.WalkDir(corpusDir, func(path string, entry os.DirEntry, err error) error {
		if err != nil || entry.IsDir() {
			return err
		}
		if entry.Name() == name {
			found = path
		}
		return nil
	})
	if err != nil || found == "" {
		t.Fatalf("找不到语料脚本 %s", name)
	}
	return found
}

// newExecutorFrom registers every function found in one corpus script.
func newExecutorFrom(t *testing.T, store *entity.Store, script string) *Executor {
	t.Helper()
	executor := New(store)
	for _, definition := range loadCorpusFuncs(t, script) {
		if err := executor.Register(definition); err != nil {
			t.Fatalf("注册 %s 失败: %v", definition.Name, err)
		}
	}
	return executor
}

// newCorpusNPC creates a 猫猫 entity carrying the fields the corpus writes.
func newCorpusNPC(t *testing.T) (*entity.Store, *entity.Entity) {
	t.Helper()
	store := entity.NewStore(t.TempDir())
	store.AddTemplate("NPC", map[string]any{
		"身体": map[string]any{"体力": 75},
		"状态": map[string]any{"身体": "活跃", "触发": ""},
		"标记": map[string]any{},
		"位置": map[string]any{"场景": "", "可达": []any{}, "上次场景": ""},
		"携带": []any{},
	})
	cat, err := store.Create("猫猫", "NPC", nil, nil)
	if err != nil {
		t.Fatalf("创建 猫猫 失败: %v", err)
	}
	return store, cat
}

// mustCall runs a function and fails the test on an execution error.
func mustCall(t *testing.T, executor *Executor, name string, args []any, current *entity.Entity) any {
	t.Helper()
	value, err := executor.Call(name, args, current)
	if err != nil {
		t.Fatalf("调用 %s(%v) 失败: %v", name, args, err)
	}
	return value
}

// assertField checks one entity field against an expected value.
func assertField(t *testing.T, target *entity.Entity, path string, want any) {
	t.Helper()
	got := target.GetOrNone(path)
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("%s.%s = %#v, 期望 %#v", target.ID, path, got, want)
	}
}

// TestCorpusDefinitionInventory proves all 15 corpus definitions are loadable.
func TestCorpusDefinitionInventory(t *testing.T) {
	total := 0
	names := map[string]bool{}
	for _, script := range corpusScripts {
		for _, definition := range loadCorpusFuncs(t, script) {
			total++
			names[definition.Name] = true
		}
	}
	if total != 15 {
		t.Fatalf("语料函数定义总数 = %d, 期望 15", total)
	}
	// 体力管理 / 饱食管理 are defined in both cat_body_state and cat_core_v2.
	if len(names) != 13 {
		t.Fatalf("唯一函数名 = %d, 期望 13: %v", len(names), names)
	}
}

// TestBodyStateFunctions covers 体力管理, 饱食管理 and 水分管理.
func TestBodyStateFunctions(t *testing.T) {
	store, cat := newCorpusNPC(t)
	executor := newExecutorFrom(t, store, "cat_body_state.neko")

	mustCall(t, executor, "体力管理", []any{-25}, cat)
	assertField(t, cat, "状态.身体", "疲惫")
	assertField(t, cat, "标记.疲劳", true)

	mustCall(t, executor, "饱食管理", []any{-35}, cat)
	assertField(t, cat, "标记.饥饿", true)

	mustCall(t, executor, "水分管理", []any{-25}, cat)
	assertField(t, cat, "标记.口渴", true)
}

// TestCoreV2Functions covers 体力管理, 饱食管理 and 危险度 from cat_core_v2.
func TestCoreV2Functions(t *testing.T) {
	store, cat := newCorpusNPC(t)
	executor := newExecutorFrom(t, store, "cat_core_v2.neko")

	mustCall(t, executor, "体力管理", []any{-35}, cat)
	assertField(t, cat, "状态.身体", "昏厥")

	if err := cat.Set("标记.饥饿", true); err != nil {
		t.Fatalf("设置 标记.饥饿 失败: %v", err)
	}
	mustCall(t, executor, "饱食管理", []any{35}, cat)
	assertField(t, cat, "标记.饥饿", false)

	cases := []struct {
		energy int
		want   int
	}{{10, 100}, {30, 50}, {80, 0}}
	for _, item := range cases {
		got := mustCall(t, executor, "危险度", []any{item.energy}, cat)
		if got != item.want {
			t.Errorf("危险度(%d) = %#v, 期望 %d", item.energy, got, item.want)
		}
	}
}

// TestDailyTimeFunction covers 时段判断 including the afternoon branch.
//
// 任务书原示例 "…T13:00" ⇒ "下午" 与真实语料不符：cat_daily.neko 的时段判断
// 对 10..14 时返回 "上午"，14..18 时才返回 "下午"。按 Lead 裁决以真实语料为准，
// 故 13:00 断言 "上午"，另用 15:00 覆盖 "下午" 分支（未改语料与函数体）。
func TestDailyTimeFunction(t *testing.T) {
	store, cat := newCorpusNPC(t)
	executor := newExecutorFrom(t, store, "cat_daily.neko")

	cases := []struct {
		stamp string
		want  string
	}{
		{"2026-04-01T15:00", "下午"},
		{"2026-04-01T07:00", "清晨"},
		{"2026-04-01T11:00", "上午"},
		{"2026-04-01T13:00", "上午"},
		{"2026-04-01T19:00", "傍晚"},
		{"2026-04-01T23:00", "深夜"},
		{"2026-04-01T02:00", "凌晨"},
	}
	for _, item := range cases {
		got := mustCall(t, executor, "时段判断", []any{item.stamp}, cat)
		if got != item.want {
			t.Errorf("时段判断(%q) = %#v, 期望 %q", item.stamp, got, item.want)
		}
	}
}

// TestEmoteIntentFunction covers 表情意图.
func TestEmoteIntentFunction(t *testing.T) {
	store, cat := newCorpusNPC(t)
	executor := newExecutorFrom(t, store, "cat_emote.neko")

	cases := []struct {
		trigger string
		want    string
	}{
		{"想睡觉", "困"},
		{"今天很开心", "开心"},
		{"好饿想吃零食", "吃货"},
		{"有点委屈", "难过"},
		{"想主人了", "爱"},
		{"普通的一天", "日常"},
	}
	for _, item := range cases {
		got := mustCall(t, executor, "表情意图", []any{item.trigger}, cat)
		if got != item.want {
			t.Errorf("表情意图(%q) = %#v, 期望 %q", item.trigger, got, item.want)
		}
	}
}

// TestSeasonFunction covers 季节判断.
func TestSeasonFunction(t *testing.T) {
	store, cat := newCorpusNPC(t)
	executor := newExecutorFrom(t, store, "cat_season.neko")

	cases := []struct {
		month int
		want  string
	}{{4, "春"}, {7, "夏"}, {10, "秋"}, {1, "冬"}, {12, "冬"}}
	for _, item := range cases {
		got := mustCall(t, executor, "季节判断", []any{item.month}, cat)
		if got != item.want {
			t.Errorf("季节判断(%d) = %#v, 期望 %q", item.month, got, item.want)
		}
	}
}

// sceneFixture is the temporary store used by the scene_assets tests.
type sceneFixture struct {
	store    *entity.Store
	cat      *entity.Entity
	flower   *entity.Entity
	executor *Executor
}

// newSceneFixture builds a cat in 花园 plus one flower there and one elsewhere.
func newSceneFixture(t *testing.T) sceneFixture {
	t.Helper()
	store := entity.NewStore(t.TempDir())
	store.AddTemplate("NPC", map[string]any{
		"位置": map[string]any{"场景": "", "可达": []any{}, "上次场景": ""},
		"携带": []any{},
		"状态": map[string]any{"触发": ""},
	})
	store.AddTemplate("素材", map[string]any{
		"类型": "物品", "名称": "", "所属场景": "", "持有人": "",
		"状态": map[string]any{"可用": true, "描述": ""},
	})
	cat, err := store.Create("猫猫", "NPC", map[string]any{"位置": map[string]any{"场景": "花园"}}, nil)
	if err != nil {
		t.Fatalf("创建 猫猫 失败: %v", err)
	}
	flower, err := store.Create("花1", "素材", map[string]any{"名称": "花1", "所属场景": "花园"}, nil)
	if err != nil {
		t.Fatalf("创建 花1 失败: %v", err)
	}
	if _, err := store.Create("花2", "素材", map[string]any{"名称": "花2", "所属场景": "卧室"}, nil); err != nil {
		t.Fatalf("创建 花2 失败: %v", err)
	}
	return sceneFixture{
		store:    store,
		cat:      cat,
		flower:   flower,
		executor: newExecutorFrom(t, store, "scene_assets.neko"),
	}
}

// TestSceneAssetFunctions covers 可操作, 拾取 and 放下 on a temp store.
func TestSceneAssetFunctions(t *testing.T) {
	fixture := newSceneFixture(t)

	allowed := mustCall(t, fixture.executor, "可操作", []any{"猫猫", "花1"}, fixture.cat)
	if allowed != true {
		t.Fatalf("可操作(猫猫, 花1) = %#v, 期望 true", allowed)
	}
	denied := mustCall(t, fixture.executor, "可操作", []any{"猫猫", "花2"}, fixture.cat)
	if denied != false {
		t.Fatalf("可操作(猫猫, 花2) = %#v, 期望 false", denied)
	}

	picked := mustCall(t, fixture.executor, "拾取", []any{"猫猫", "花1"}, fixture.cat)
	if picked != "拾取了:花1" {
		t.Fatalf("拾取 = %#v", picked)
	}
	assertField(t, fixture.flower, "持有人", "猫猫")
	assertField(t, fixture.flower, "所属场景", "")
	assertField(t, fixture.cat, "携带", []any{"花1"})

	dropped := mustCall(t, fixture.executor, "放下", []any{"猫猫", "花1"}, fixture.cat)
	if dropped != "放下了:花1" {
		t.Fatalf("放下 = %#v", dropped)
	}
	assertField(t, fixture.flower, "持有人", "")
	assertField(t, fixture.flower, "所属场景", "花园")
	assertField(t, fixture.cat, "携带", []any{})
}

// TestSceneAssetMigration covers 素材迁移 across scenes.
func TestSceneAssetMigration(t *testing.T) {
	fixture := newSceneFixture(t)

	result := mustCall(t, fixture.executor, "素材迁移", []any{"猫猫", "花1", "厨房"}, fixture.cat)
	if result != "素材已移动到:厨房" {
		t.Fatalf("素材迁移 = %#v", result)
	}
	assertField(t, fixture.flower, "所属场景", "厨房")
	assertField(t, fixture.flower, "持有人", "")
	assertField(t, fixture.cat, "状态.触发", "移动了素材")

	denied := mustCall(t, fixture.executor, "可操作", []any{"猫猫", "花1"}, fixture.cat)
	if denied != false {
		t.Fatalf("迁移后 可操作 = %#v, 期望 false", denied)
	}
}

// TestSceneMoveFunctions covers 场景移动 and 更新可达.
func TestSceneMoveFunctions(t *testing.T) {
	store, cat := newCorpusNPC(t)
	if err := cat.Set("位置.场景", "花园"); err != nil {
		t.Fatalf("设置 位置.场景 失败: %v", err)
	}
	if err := cat.Set("位置.可达", []any{"厨房"}); err != nil {
		t.Fatalf("设置 位置.可达 失败: %v", err)
	}
	if _, err := store.Create("花园", "场景", map[string]any{"连接": []any{"厨房", "卧室"}}, nil); err != nil {
		t.Fatalf("创建 花园 失败: %v", err)
	}
	executor := newExecutorFrom(t, store, "scene_move.neko")

	moved := mustCall(t, executor, "场景移动", []any{"厨房"}, cat)
	if moved != "移动成功" {
		t.Fatalf("场景移动 = %#v", moved)
	}
	assertField(t, cat, "位置.场景", "厨房")
	assertField(t, cat, "位置.上次场景", "花园")
	assertField(t, cat, "状态.触发", "移动到了厨房")

	blocked := mustCall(t, executor, "场景移动", []any{"卧室"}, cat)
	if blocked != "无法移动" {
		t.Fatalf("不可达场景移动 = %#v", blocked)
	}

	// The corpus body returns "可达更新:" + 可达 where 可达 is a list; Python
	// raises TypeError there, so the Go interpreter reports the same type error
	// while keeping the assignment it performed before the return.
	if _, err := executor.Call("更新可达", []any{"花园"}, cat); err == nil {
		t.Fatal("更新可达: string + list must fail (Python TypeError parity)")
	}
	assertField(t, cat, "位置.可达", []any{"厨房", "卧室"})
}

// TestExecutorRegistry covers registration lifecycle and error reporting.
func TestExecutorRegistry(t *testing.T) {
	executor := New(nil)
	if err := executor.Register(neko.FuncDef{Name: "加法", Body: "返回 a + b", Params: []neko.Param{{Name: "a"}, {Name: "b"}}}); err != nil {
		t.Fatalf("注册失败: %v", err)
	}
	if !executor.Has("加法") {
		t.Fatal("Has 应为 true")
	}
	value, err := executor.Call("加法", []any{2, 3}, nil)
	if err != nil || value != 5 {
		t.Fatalf("Call = %#v, %v", value, err)
	}
	if err := executor.Register(neko.FuncDef{Name: "坏函数", Body: "if {"}); err == nil {
		t.Fatal("语法错误应返回 error")
	}
	if err := executor.Register(neko.FuncDef{Name: "", Body: "返回 1"}); err == nil {
		t.Fatal("空函数名应返回 error")
	}
	if _, err := executor.Call("不存在", nil, nil); err == nil {
		t.Fatal("未定义函数应返回 error")
	}
	executor.Unregister("加法")
	if executor.Has("加法") {
		t.Fatal("Unregister 后 Has 应为 false")
	}
}

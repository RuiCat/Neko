// fixtures_test.go holds the integration fixtures: a real entity.Store rooted in
// t.TempDir(), the kernel-shaped templates and materials, the flow/task specs of
// the corpus and the assertion helpers. No fake store is used here; every write
// goes through entity.Entity.
package host

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/RuiCat/Neko/internal/entity"
	"github.com/RuiCat/Neko/internal/flow/condition"
	"github.com/RuiCat/Neko/internal/flow/spec"
	"github.com/RuiCat/Neko/internal/flow/step"
)

// Fixture identity shared by the integration walks.
const (
	// testActorID is the entity every flow settles.
	testActorID = "猫猫"
	// testScene is the actor's scene and the kitchen item scene.
	testScene = "S001-KT"
	// testOtherScene holds a same-name material C5 must leave alone.
	testOtherScene = "S008"
)

// npcTemplate is a kernel-shaped actor template: the scene row the judge reads,
// the trade rows settlement requests, and the source rows it writes.
const npcTemplate = `{
  "位置": {"场景": "S001-KT"},
  "交易": {"货币": 100, "成交额": 0, "结果": "", "声誉": 50},
  "代谢": {"胃内容": 0},
  "标记": {"饥饿": true},
  "任务": {"当前": "", "进度": 0, "阶段": "未激活"},
  "成长": {"经验": 0},
  "关系": {"数值": {"好感度": 0, "亲密": 0}}
}`

// readonlyNPCTemplate marks 标记.饥饿 readonly, which is how entity.Set rejects a
// settlement row after an earlier row in the same batch was written.
const readonlyNPCTemplate = `{
  "位置": {"场景": "S001-KT"},
  "交互": {"动作": "", "目标": ""},
  "交易": {"货币": 100, "成交额": 0, "结果": "", "声誉": 50},
  "代谢": {"胃内容": 0},
  "标记": {"饥饿": true},
  "任务": {"当前": "", "进度": 0, "阶段": "未激活"},
  "成长": {"经验": 0},
  "关系": {"数值": {"好感度": 0, "亲密": 0}},
  "__annotations__": {"标记.饥饿": {"readonly": true}}
}`

// materialTemplate is the 素材 kernel instance shape the item view projects.
const materialTemplate = `{
  "类型": "物品",
  "类别": "杂物",
  "名称": "",
  "所属场景": "",
  "描述": "",
  "数量": 1,
  "重量": 100,
  "体积": 100,
  "耐久": 100
}`

// newTestStore writes the templates into a temp directory and loads a real
// entity.Store. The directory is returned so a test can reload from disk.
func newTestStore(t *testing.T, npc string) (*entity.Store, string) {
	t.Helper()
	dir := t.TempDir()
	templates := filepath.Join(dir, "templates")
	if err := os.MkdirAll(templates, 0o755); err != nil {
		t.Fatalf("create templates dir: %v", err)
	}
	writeFixture(t, filepath.Join(templates, "角色基础.json"), npc)
	writeFixture(t, filepath.Join(templates, "素材.json"), materialTemplate)
	store := entity.NewStore(dir)
	if err := store.Load(); err != nil {
		t.Fatalf("load store: %v", err)
	}
	return store, dir
}

// writeFixture writes one template file.
func writeFixture(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", filepath.Base(path), err)
	}
}

// createActor creates the one entity every flow settles.
func createActor(t *testing.T, store *entity.Store) *entity.Entity {
	t.Helper()
	actor, err := store.Create(testActorID, "角色基础", nil, nil)
	if err != nil {
		t.Fatalf("create actor: %v", err)
	}
	return actor
}

// createMaterial creates one material entity with its kernel fields.
func createMaterial(t *testing.T, store *entity.Store, id, name, scene string, quantity int) *entity.Entity {
	t.Helper()
	data := map[string]any{
		condition.KEY_ITEM_NAME:     name,
		condition.KEY_ITEM_SCENE:    scene,
		condition.KEY_ITEM_QUANTITY: quantity,
	}
	material, err := store.Create(id, MATERIAL_TYPE, data, nil)
	if err != nil {
		t.Fatalf("create material %s: %v", name, err)
	}
	return material
}

// reopenStore loads a second store from the same directory, which is how a test
// proves a row reached the disk instead of only the entity in memory.
func reopenStore(t *testing.T, dir string) *entity.Store {
	t.Helper()
	store := entity.NewStore(dir)
	if err := store.Load(); err != nil {
		t.Fatalf("reload store: %v", err)
	}
	return store
}

// cookingFlow is the corpus 做饭 flow: consume 食材 in the kitchen, produce 食物.
func cookingFlow() spec.FlowSpec {
	return spec.FlowSpec{
		ID:       "做饭",
		Scene:    testScene,
		Requires: []string{"食材"},
		Consume:  []string{"食材"},
		Produce:  []spec.ItemSpec{{Type: MATERIAL_TYPE, Name: "食物", Description: "热气腾腾的饭菜"}},
		Steps:    []string{"洗菜切菜", "开火下锅", "翻炒调味", "装盘"},
	}
}

// buyingFlow is the corpus 购买食材 flow: pay 10 and create 食材 in the shop scene.
func buyingFlow() spec.FlowSpec {
	return spec.FlowSpec{
		ID:      "购买食材",
		Scene:   testOtherScene,
		Cost:    10,
		Produce: []spec.ItemSpec{{Type: MATERIAL_TYPE, Name: "食材", Description: "新鲜蔬菜和肉"}},
		Steps:   []string{"挑选", "结账"},
	}
}

// eatingFlow is the corpus 吃饭 flow with one data-layer effect. Computed rows
// such as 代谢.胃内容 stay with the metabolism tick and are not written here.
func eatingFlow() spec.FlowSpec {
	hunger := "标记.饥饿"
	return spec.FlowSpec{
		ID:       "吃饭",
		Scene:    testScene,
		Requires: []string{"食物"},
		Consume:  []string{"食物"},
		Steps:    []string{"摆好碗筷", "一口一口吃饭"},
		Effects: []spec.EffectSpec{
			{Path: "饥饿", Kind: spec.EFFECT_KIND_BOOL, Value: false, Target: &hunger, TargetReason: "测试映射"},
		},
		Intents: []spec.IntentSpec{{Action: "吃", Target: "食物"}},
	}
}

// cookingTask is the task template one 做饭 completion advances.
func cookingTask() spec.TaskTemplateSpec {
	return spec.TaskTemplateSpec{
		ID:     "做饭",
		Stages: []string{"未激活", "进行中", "完成"},
		Reward: spec.TaskReward{Experience: 20, Affinity: 5},
	}
}

// runSteps drives a flow from pending to done one minute per step.
func runSteps(t *testing.T, flow *spec.FlowSpec, executionID string, tick int) step.State {
	t.Helper()
	state, err := step.Start(flow, executionID, tick)
	if err != nil {
		t.Fatalf("step start: %v", err)
	}
	for state.Status != step.StatusDone {
		next, err := step.Next(flow, state, 1)
		if err != nil {
			t.Fatalf("step next: %v", err)
		}
		state = next
	}
	return state
}

// requireInt asserts one entity row reads as want.
func requireInt(t *testing.T, ent *entity.Entity, path string, want int) {
	t.Helper()
	got, err := intValue(ent.GetOrNone(path), path)
	if err != nil {
		t.Fatalf("%s: %v", path, err)
	}
	if got != want {
		t.Fatalf("%s = %d, want %d", path, got, want)
	}
}

// requireText asserts one entity row reads as want.
func requireText(t *testing.T, ent *entity.Entity, path, want string) {
	t.Helper()
	got, ok := ent.GetOrNone(path).(string)
	if !ok || got != want {
		t.Fatalf("%s = %v, want %q", path, got, want)
	}
}

// requireBool asserts one entity flag reads as want.
func requireBool(t *testing.T, ent *entity.Entity, path string, want bool) {
	t.Helper()
	got, ok := ent.GetOrNone(path).(bool)
	if !ok || got != want {
		t.Fatalf("%s = %v, want %v", path, got, want)
	}
}

// itemQuantity sums one material view row set; an empty scene counts every scene.
func itemQuantity(t *testing.T, store *entity.Store, name, scene string) int {
	t.Helper()
	rows, err := NewItemsView(store).View()
	if err != nil {
		t.Fatalf("items view: %v", err)
	}
	total := 0
	for index, raw := range rows {
		row, ok := raw.(map[string]any)
		if !ok {
			t.Fatalf("item row %d: got %T", index, raw)
		}
		if row[condition.KEY_ITEM_NAME] != name {
			continue
		}
		if scene != "" && row[condition.KEY_ITEM_SCENE] != scene {
			continue
		}
		quantity, err := intValue(row[condition.KEY_ITEM_QUANTITY], "quantity")
		if err != nil {
			t.Fatalf("item row %d: %v", index, err)
		}
		total += quantity
	}
	return total
}

// requireItemQuantity asserts the item view holds want of one material.
func requireItemQuantity(t *testing.T, store *entity.Store, name, scene string, want int) {
	t.Helper()
	if got := itemQuantity(t, store, name, scene); got != want {
		t.Fatalf("item %q in scene %q = %d, want %d", name, scene, got, want)
	}
}

// materialEntity finds the material entity of one (name, scene).
func materialEntity(t *testing.T, store *entity.Store, name, scene string) *entity.Entity {
	t.Helper()
	for _, ent := range store.ListAll() {
		if !entity.IsSubtypeOf(ent.Type, MATERIAL_TYPE) {
			continue
		}
		if ent.GetOrNone(condition.KEY_ITEM_NAME) != name {
			continue
		}
		if ent.GetOrNone(condition.KEY_ITEM_SCENE) != scene {
			continue
		}
		return ent
	}
	t.Fatalf("material %q in scene %q not found", name, scene)
	return nil
}

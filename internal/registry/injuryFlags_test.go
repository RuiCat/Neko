// C6 回归：伤害必需器官失效由 器官基础.<器官>.必需 数据驱动。
package registry

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/RuiCat/Neko/internal/engine"
	"github.com/RuiCat/Neko/internal/entity"
	"github.com/RuiCat/Neko/internal/events"
	"github.com/RuiCat/Neko/internal/function"
)

// loadRepoScripts bootstraps every .neko under <repo>/scripts in dependency
// order (@定义 -> @实体 -> @函数 -> 其余) so composition parents exist.
func loadRepoScripts(t *testing.T, store *entity.Store, machine *engine.StateMachine, funcs *function.Executor, ev *events.System, dataDir string) {
	t.Helper()
	root := filepath.Join("..", "..", "scripts")
	var files []string
	err := filepath.Walk(root, func(path string, info os.FileInfo, walkErr error) error {
		if walkErr == nil && !info.IsDir() && strings.HasSuffix(path, ".neko") {
			files = append(files, path)
		}
		return walkErr
	})
	if err != nil {
		t.Fatalf("walk scripts: %v", err)
	}
	classify := func(text string) int {
		switch {
		case strings.Contains(text, "@定义"):
			return 0
		case strings.Contains(text, "@实体"):
			return 1
		case strings.Contains(text, "@函数"):
			return 2
		default:
			return 3
		}
	}
	sort.Slice(files, func(i, j int) bool {
		left, _ := os.ReadFile(files[i])
		right, _ := os.ReadFile(files[j])
		ci, cj := classify(string(left)), classify(string(right))
		if ci != cj {
			return ci < cj
		}
		return files[i] < files[j]
	})
	reg, err := New(store, machine, filepath.Join(dataDir, "scripts.json"), dataDir)
	if err != nil {
		t.Fatalf("registry: %v", err)
	}
	reg.SetFunctionSink(ev)
	for _, path := range files {
		raw, readErr := os.ReadFile(path)
		if readErr != nil {
			t.Fatalf("read %s: %v", path, readErr)
		}
		name := strings.TrimSuffix(filepath.Base(path), ".neko")
		if result := reg.Apply(name, string(raw), false); !result.OK {
			t.Fatalf("apply %s: %+v", path, result.Errors)
		}
	}
}

// TestRequiredOrganFlagsDriveFailure pins C6: the 必需 annotation decides
// whether a zeroed organ kills the entity, including 皮肤 which the old
// hard-coded list missed.
func TestRequiredOrganFlagsDriveFailure(t *testing.T) {
	dataDir := t.TempDir()
	store := entity.NewStore(dataDir)
	machine := engine.New(store)
	funcs := function.New(store)
	ev := events.NewSystem(store, funcs)
	machine.SetFuncCaller(funcs)
	loadRepoScripts(t, store, machine, funcs, ev, dataDir)

	if _, err := store.Create("猫猫", "NPC", map[string]any{"基础": map[string]any{"名字": "猫猫", "年龄": 20}}, nil); err != nil {
		t.Fatalf("create: %v", err)
	}
	ent := store.Get("猫猫")
	failed := func() bool {
		value, err := funcs.Call("伤害必需器官失效", nil, ent)
		if err != nil {
			t.Fatalf("call: %v", err)
		}
		result, _ := value.(bool)
		return result
	}

	if err := ent.Set("器官.脑.完整性", 0); err != nil {
		t.Fatalf("set brain: %v", err)
	}
	if !failed() {
		t.Fatal("必需=true 的脑完整性归零应判失效")
	}
	if err := ent.Set("器官基础.脑.必需", false); err != nil {
		t.Fatalf("set flag: %v", err)
	}
	if failed() {
		t.Fatal("必需=false 的器官不应判失效（未消费 必需 字段）")
	}
	if err := ent.Set("器官基础.脑.必需", true); err != nil {
		t.Fatalf("restore flag: %v", err)
	}
	if err := ent.Set("器官.脑.完整性", 100); err != nil {
		t.Fatalf("restore brain: %v", err)
	}
	if err := ent.Set("器官.皮肤.完整性", 0); err != nil {
		t.Fatalf("set skin: %v", err)
	}
	if !failed() {
		t.Fatal("必需=true 的皮肤完整性归零应判失效（旧硬编码漏项）")
	}
}

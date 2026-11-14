// composition_test.go covers @实体 parent resolution, persistence and the
// strict single-owner validation helpers.
package entity

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCompositionCreateAndPersist(t *testing.T) {
	dir := t.TempDir()
	store := NewStore(dir)
	if err := store.Load(); err != nil {
		t.Fatal(err)
	}
	store.AddTemplate("NPC:猫科", map[string]any{"毛色": "未知"})
	store.AddTemplate("NPC:成年", map[string]any{"年龄": float64(18)})
	store.SetTemplateParents("NPC:猫猫", []string{"NPC:猫科", "NPC:成年"})
	store.AddTemplate("NPC:猫猫", map[string]any{"体型": "幼"})
	for _, typeName := range []string{"NPC:猫科", "NPC:成年", "NPC:猫猫"} {
		if err := store.SaveTemplateFile(typeName); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := os.Stat(filepath.Join(dir, "templates", "NPC_猫猫.parents.json")); err != nil {
		t.Fatalf("parents sidecar missing: %v", err)
	}

	reloaded := NewStore(dir)
	if err := reloaded.Load(); err != nil {
		t.Fatal(err)
	}
	if got := reloaded.GetTemplateParents("NPC:猫猫"); len(got) != 2 || got[0] != "NPC:猫科" {
		t.Fatalf("reloaded parents = %v", got)
	}
	order := reloaded.ResolveOrder("NPC:猫猫")
	if len(order) != 3 || order[len(order)-1] != "NPC:猫猫" {
		t.Fatalf("order = %v", order)
	}

	ent, err := reloaded.Create("猫猫", "NPC:猫猫", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	for path, want := range map[string]any{"毛色": "未知", "年龄": float64(18), "体型": "幼"} {
		got, err := ent.Get(path)
		if err != nil || got != want {
			t.Fatalf("get %s = %v, %v (want %v)", path, got, err, want)
		}
	}
}

func TestValidateCompositionRejectsOwnParentOverlap(t *testing.T) {
	templates := map[string]map[string]any{
		"父": {"身体": map[string]any{"体力": float64(1)}},
	}
	parents := map[string][]string{"子": {"父"}}
	own := map[string]any{"身体": map[string]any{"体力": float64(2)}}
	errs := ValidateComposition("子", own, templates, parents)
	if len(errs) == 0 {
		t.Fatal("expected own/parent overlap error")
	}
}

func TestValidateCompositionRejectsSiblingConflict(t *testing.T) {
	templates := map[string]map[string]any{
		"父A": {"值": float64(1)},
		"父B": {"值": float64(2)},
	}
	parents := map[string][]string{"子": {"父A", "父B"}}
	errs := ValidateComposition("子", map[string]any{"额外": true}, templates, parents)
	if len(errs) == 0 {
		t.Fatal("expected sibling conflict error")
	}
}

func TestValidateCompositionPrefixConflict(t *testing.T) {
	templates := map[string]map[string]any{
		"父A": {"身体": float64(1)},
		"父B": {"身体": map[string]any{"体力": float64(2)}},
	}
	parents := map[string][]string{"子": {"父A", "父B"}}
	errs := ValidateComposition("子", map[string]any{}, templates, parents)
	if len(errs) == 0 {
		t.Fatal("expected scalar-vs-object conflict")
	}
}

func TestCheckDuplicatePathsRejectsSameValue(t *testing.T) {
	existing := map[string]any{"值": float64(1)}
	incoming := map[string]any{"值": float64(1)}
	if errs := CheckDuplicatePaths("类型", existing, incoming); len(errs) == 0 {
		t.Fatal("expected duplicate even with the same value")
	}
}

func TestResolveTemplateOrderRejectsCycle(t *testing.T) {
	templates := map[string]map[string]any{"A": {}, "B": {}}
	parents := map[string][]string{"A": {"B"}, "B": {"A"}}
	if _, err := ResolveTemplateOrder("A", templates, parents); err == nil {
		t.Fatal("expected cycle error")
	}
}

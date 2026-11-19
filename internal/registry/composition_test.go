// composition_test.go covers @实体 apply validation and the delete-dependency
// guard at the registry layer.
package registry

import (
	"path/filepath"
	"testing"

	"github.com/RuiCat/Neko/internal/engine"
	"github.com/RuiCat/Neko/internal/entity"
)

func newCompositionRegistry(t *testing.T) (*Registry, *entity.Store) {
	t.Helper()
	dir := t.TempDir()
	store := entity.NewStore(dir)
	if err := store.Load(); err != nil {
		t.Fatal(err)
	}
	reg, err := New(store, engine.New(store), filepath.Join(dir, "scripts.json"), filepath.Join(dir, "scripts"))
	if err != nil {
		t.Fatal(err)
	}
	return reg, store
}

func applyText(t *testing.T, reg *Registry, name, text string) *ApplyResult {
	t.Helper()
	return reg.Apply(name, text, false)
}

func TestApplyComposedTemplate(t *testing.T) {
	reg, store := newCompositionRegistry(t)
	parent := applyText(t, reg, "cat_family", `@定义 角色基础:猫科 {
  "毛色": "未知"
}`)
	if !parent.OK {
		t.Fatalf("parent apply: %+v", parent.Errors)
	}
	child := applyText(t, reg, "cat_maomao", `@实体 角色基础:猫猫 [角色基础:猫科] {
  "体型": "幼"
}`)
	if !child.OK {
		t.Fatalf("child apply: %+v", child.Errors)
	}
	if got := store.GetTemplateParents("角色基础:猫猫"); len(got) != 1 || got[0] != "角色基础:猫科" {
		t.Fatalf("parents = %v", got)
	}
	ent, err := store.Create("猫猫", "角色基础:猫猫", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got, err := ent.Get("毛色"); err != nil || got != "未知" {
		t.Fatalf("毛色 = %v, %v", got, err)
	}
	if got, err := ent.Get("体型"); err != nil || got != "幼" {
		t.Fatalf("体型 = %v, %v", got, err)
	}
}

func TestApplyRejectsSiblingFieldConflict(t *testing.T) {
	reg, _ := newCompositionRegistry(t)
	if result := applyText(t, reg, "p_a", `@定义 父A { "值": 1 }`); !result.OK {
		t.Fatalf("父A: %+v", result.Errors)
	}
	if result := applyText(t, reg, "p_b", `@定义 父B { "值": 2 }`); !result.OK {
		t.Fatalf("父B: %+v", result.Errors)
	}
	result := applyText(t, reg, "child", `@实体 子 [父A, 父B] { "额外": true }`)
	if result.OK {
		t.Fatal("expected sibling conflict to fail")
	}
}

func TestApplyRejectsChildOverride(t *testing.T) {
	reg, _ := newCompositionRegistry(t)
	if result := applyText(t, reg, "p_a", `@定义 父A { "值": 1 }`); !result.OK {
		t.Fatalf("父A: %+v", result.Errors)
	}
	result := applyText(t, reg, "child", `@实体 子 [父A] { "值": 2 }`)
	if result.OK {
		t.Fatal("expected child override to fail")
	}
}

func TestApplyRejectsDuplicateContribution(t *testing.T) {
	reg, _ := newCompositionRegistry(t)
	if result := applyText(t, reg, "p_a", `@定义 父A { "值": 1 }`); !result.OK {
		t.Fatalf("父A: %+v", result.Errors)
	}
	if result := applyText(t, reg, "child", `@实体 子 [父A] { "体型": "幼" }`); !result.OK {
		t.Fatalf("child: %+v", result.Errors)
	}
	result := applyText(t, reg, "extra", `@定义 子 { "体型": "大" }`)
	if result.OK {
		t.Fatal("expected duplicate contribution to fail")
	}
}

func TestApplyComposedTemplateInOneScript(t *testing.T) {
	reg, store := newCompositionRegistry(t)
	result := applyText(t, reg, "combo", `@定义 父A { "值": 1 }
@实体 子 [父A] { "额外": true }`)
	if !result.OK {
		t.Fatalf("combined apply: %+v", result.Errors)
	}
	if got := store.GetTemplateParents("子"); len(got) != 1 || got[0] != "父A" {
		t.Fatalf("parents = %v", got)
	}
}

func TestParentFieldChangeRevalidatesDescendants(t *testing.T) {
	reg, _ := newCompositionRegistry(t)
	if result := applyText(t, reg, "p_a", `@定义 父A { "值": 1 }`); !result.OK {
		t.Fatalf("父A: %+v", result.Errors)
	}
	if result := applyText(t, reg, "child", `@实体 子 [父A] { "额外": true }`); !result.OK {
		t.Fatalf("child: %+v", result.Errors)
	}
	result := applyText(t, reg, "p_a_more", `@定义 父A { "额外": false }`)
	if result.OK {
		t.Fatal("expected parent change to conflict with the descendant's field")
	}
}

func TestApplyRejectsMissingParent(t *testing.T) {
	reg, _ := newCompositionRegistry(t)
	result := applyText(t, reg, "child", `@实体 子 [不存在] { "值": 1 }`)
	if result.OK {
		t.Fatal("expected missing parent to fail")
	}
}

func TestReapplyCannotDropDependentParent(t *testing.T) {
	reg, store := newCompositionRegistry(t)
	if result := applyText(t, reg, "p_a", `@定义 父A { "值": 1 }`); !result.OK {
		t.Fatalf("父A: %+v", result.Errors)
	}
	if result := applyText(t, reg, "child", `@实体 子 [父A] { "额外": true }`); !result.OK {
		t.Fatalf("child: %+v", result.Errors)
	}
	result := reg.Apply("p_a", `@定义 无关 { "值": 2 }`, true)
	if result.OK {
		t.Fatal("expected re-apply to refuse dropping a parent in use")
	}
	if !store.HasTemplate("父A") {
		t.Fatal("parent template was dropped despite dependents")
	}
}

func TestRemoveRefusesDependentTemplate(t *testing.T) {
	reg, store := newCompositionRegistry(t)
	if result := applyText(t, reg, "p_a", `@定义 父A { "值": 1 }`); !result.OK {
		t.Fatalf("父A: %+v", result.Errors)
	}
	if result := applyText(t, reg, "child", `@实体 子 [父A] { "额外": true }`); !result.OK {
		t.Fatalf("child: %+v", result.Errors)
	}
	result := reg.Remove("p_a")
	if result.OK {
		t.Fatal("expected dependent remove to fail")
	}
	if !store.HasTemplate("父A") {
		t.Fatal("parent template was removed despite dependency")
	}
	if result := reg.Remove("child"); !result.OK {
		t.Fatalf("child remove: %+v", result.Errors)
	}
	if result := reg.Remove("p_a"); !result.OK {
		t.Fatalf("parent remove after child: %+v", result.Errors)
	}
}

func TestRemoveRefusesImplicitChainDependency(t *testing.T) {
	reg, _ := newCompositionRegistry(t)
	if result := applyText(t, reg, "base", `@定义 角色基础 { "值": 1 }`); !result.OK {
		t.Fatalf("角色基础: %+v", result.Errors)
	}
	if result := applyText(t, reg, "child", `@定义 角色基础:猫猫 { "额外": true }`); !result.OK {
		t.Fatalf("child: %+v", result.Errors)
	}
	if result := reg.Remove("base"); result.OK {
		t.Fatal("expected implicit chain dependency to block remove")
	}
}

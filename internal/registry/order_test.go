package registry

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ruicat/astrbox/internal/engine"
	"github.com/ruicat/astrbox/internal/entity"
)

func TestScriptOrderPreserved(t *testing.T) {
	dir := t.TempDir()
	registryPath := filepath.Join(dir, "scripts.json")
	initial := `{"version":1,"scripts":{"b":{"hash":"h","templates":{},"rules":{}},"a":{"hash":"h","templates":{},"rules":{}}}}`
	if err := os.WriteFile(registryPath, []byte(initial), 0o644); err != nil {
		t.Fatal(err)
	}
	store := entity.NewStore(dir)
	if err := store.Load(); err != nil {
		t.Fatal(err)
	}
	reg, err := New(store, engine.New(store), registryPath, filepath.Join(dir, "scripts"))
	if err != nil {
		t.Fatal(err)
	}
	result := reg.Apply("c", `@规则[$] {
  动作: 值 = 1
}`, false, false)
	if !result.OK {
		t.Fatalf("apply: %+v", result.Errors)
	}
	raw, err := os.ReadFile(registryPath)
	if err != nil {
		t.Fatal(err)
	}
	text := string(raw)
	indexB, indexA, indexC := strings.Index(text, `"b"`), strings.Index(text, `"a"`), strings.Index(text, `"c"`)
	if indexB < 0 || indexA < 0 || indexC < 0 || !(indexB < indexA && indexA < indexC) {
		t.Fatalf("order not preserved: b=%d a=%d c=%d\n%s", indexB, indexA, indexC, text)
	}
}

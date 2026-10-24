package registry

import (
	"crypto/sha256"
	"os"
	"path/filepath"
	"testing"

	"github.com/ruicat/astrbox/internal/engine"
	"github.com/ruicat/astrbox/internal/entity"
)

func TestTemplatePersistence(t *testing.T) {
	dir := t.TempDir()
	store := entity.NewStore(dir)
	if err := store.Load(); err != nil {
		t.Fatal(err)
	}
	reg, err := New(store, engine.New(store), filepath.Join(dir, "scripts.json"), filepath.Join(dir, "scripts"))
	if err != nil {
		t.Fatal(err)
	}
	script := `@Data 测试类型 {
  "值": 1
}
@规则[$[测试类型]] {
  动作: 值 = 2
}`
	result := reg.Apply("tpl_script", script, false, false)
	if !result.OK {
		t.Fatalf("apply failed: %+v", result.Errors)
	}
	path := filepath.Join(dir, "templates", "测试类型.json")
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("template file missing after apply: %v", err)
	}

	replay := reg.ReplayAll()
	if len(replay.Failed) != 0 {
		t.Fatalf("replay failed: %v", replay.Failed)
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("template file missing after replay: %v", err)
	}
	if sha256.Sum256(before) != sha256.Sum256(after) {
		t.Fatal("replay rewrote the template file")
	}

	reg.Remove("tpl_script")
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("template file still exists after remove: %v", err)
	}
}

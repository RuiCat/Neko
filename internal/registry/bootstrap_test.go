package registry

import (
	"crypto/sha256"
	"os"
	"path/filepath"
	"testing"

	"github.com/ruicat/astrbox/internal/engine"
	"github.com/ruicat/astrbox/internal/entity"
)

func TestBootstrapFromSources(t *testing.T) {
	dataDir := t.TempDir()
	scriptsDir := t.TempDir()
	for _, item := range [][2]string{{"core/cat_core.neko", "cat_core.neko"}, {"behavior/cat_daily.neko", "cat_daily.neko"}} {
		raw, err := os.ReadFile(filepath.Join("..", "..", "scripts", item[0]))
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(scriptsDir, item[1]), raw, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	before, err := os.ReadFile(filepath.Join(scriptsDir, "cat_core.neko"))
	if err != nil {
		t.Fatal(err)
	}
	store := entity.NewStore(dataDir)
	if err := store.Load(); err != nil {
		t.Fatal(err)
	}
	reg, err := New(store, engine.New(store), filepath.Join(dataDir, "scripts.json"), scriptsDir)
	if err != nil {
		t.Fatal(err)
	}
	if got := len(reg.List()); got != 0 {
		t.Fatalf("fresh registry has %d scripts", got)
	}
	result, err := reg.BootstrapFromSources()
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Replayed) != 2 || len(result.Failed) != 0 {
		t.Fatalf("bootstrap: %+v", result)
	}
	if got := len(reg.List()); got != 2 {
		t.Fatalf("registry scripts = %d", got)
	}
	if _, err := os.Stat(filepath.Join(dataDir, "scripts.json")); err != nil {
		t.Fatalf("registry not persisted: %v", err)
	}
	after, err := os.ReadFile(filepath.Join(scriptsDir, "cat_core.neko"))
	if err != nil {
		t.Fatal(err)
	}
	if sha256.Sum256(before) != sha256.Sum256(after) {
		t.Fatal("bootstrap rewrote the source file")
	}
}

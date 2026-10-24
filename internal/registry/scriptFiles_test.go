package registry

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ruicat/astrbox/internal/engine"
	"github.com/ruicat/astrbox/internal/entity"
)

const baseScript = `@定义 NPC {
  "身体": {"体力": 75}
}
`

func newScriptFileRegistry(t *testing.T) (*Registry, string, string) {
	t.Helper()
	dataDir := t.TempDir()
	scriptsDir := t.TempDir()
	store := entity.NewStore(dataDir)
	if err := store.Load(); err != nil {
		t.Fatalf("load store: %v", err)
	}
	reg, err := New(store, engine.New(store), filepath.Join(dataDir, "scripts.json"), scriptsDir)
	if err != nil {
		t.Fatalf("registry: %v", err)
	}
	return reg, dataDir, scriptsDir
}

func TestReadAndListScript(t *testing.T) {
	reg, _, _ := newScriptFileRegistry(t)
	if result := reg.Apply("base", baseScript, false, true); !result.OK {
		t.Fatalf("apply: %+v", result.Errors)
	}
	view, err := reg.ReadScript("base")
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if view.Source != baseScript {
		t.Fatalf("source mismatch")
	}
	if view.Hash != textHash(baseScript) || view.ActualHash != view.Hash {
		t.Fatalf("hash = %q actual = %q", view.Hash, view.ActualHash)
	}
	items := reg.ListScripts()
	if len(items) != 1 || items[0].Name != "base" || items[0].Templates != 1 {
		t.Fatalf("items = %+v", items)
	}
	if _, err := reg.ReadScript("../escape"); err == nil {
		t.Fatal("path traversal was accepted")
	}
}

func TestReadRejectsSymlink(t *testing.T) {
	reg, dataDir, scriptsDir := newScriptFileRegistry(t)
	outside := filepath.Join(dataDir, "outside.neko")
	if err := os.WriteFile(outside, []byte(baseScript), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(scriptsDir, "evil.neko")); err != nil {
		t.Skipf("symlink unsupported: %v", err)
	}
	if _, err := reg.ReadScript("evil"); err == nil {
		t.Fatal("symlink was accepted")
	}
}

func TestValidateDoesNotMutate(t *testing.T) {
	reg, _, _ := newScriptFileRegistry(t)
	if result := reg.Apply("base", baseScript, false, true); !result.OK {
		t.Fatalf("apply: %+v", result.Errors)
	}
	before := len(reg.engine.Rules())
	validated := reg.ValidateScript("preview", strings.Replace(baseScript, "75", "80", 1), false)
	if !validated.OK {
		t.Fatalf("validate: %+v", validated.Errors)
	}
	if len(reg.engine.Rules()) != before {
		t.Fatal("validate registered rules")
	}
	if _, err := reg.ReadScript("preview"); err == nil {
		t.Fatal("validate wrote a source file")
	}
}

func TestWriteScriptCAS(t *testing.T) {
	reg, _, _ := newScriptFileRegistry(t)
	if result := reg.Apply("base", baseScript, false, true); !result.OK {
		t.Fatalf("apply: %+v", result.Errors)
	}
	oldHash := reg.Get("base").Hash
	changed := strings.Replace(baseScript, "75", "80", 1)

	if result := reg.WriteScript("base", changed, ""); result.OK {
		t.Fatal("write without hash succeeded")
	}
	if result := reg.WriteScript("base", changed, "wrong"); result.OK {
		t.Fatal("write with wrong hash succeeded")
	}
	if result := reg.WriteScript("base", changed, oldHash); !result.OK {
		t.Fatalf("write: %+v", result.Errors)
	}
	view, err := reg.ReadScript("base")
	if err != nil {
		t.Fatal(err)
	}
	if view.Source != changed || view.Hash != textHash(changed) {
		t.Fatalf("source/hash not updated: %q %q", view.Source, view.Hash)
	}
	if result := reg.WriteScript("base", changed, oldHash); result.OK {
		t.Fatal("stale hash was accepted after a successful write")
	}
	if result := reg.WriteScript("fresh", baseScript, ""); !result.OK {
		t.Fatalf("new script write: %+v", result.Errors)
	}
}

func TestWriteScriptInvalidName(t *testing.T) {
	reg, _, _ := newScriptFileRegistry(t)
	result := reg.WriteScript("../bad", baseScript, "")
	if result.OK {
		t.Fatal("invalid name was accepted")
	}
	if len(reg.List()) != 0 {
		t.Fatal("invalid write mutated the registry")
	}
}

func TestWriteScriptRejectsSymlink(t *testing.T) {
	reg, dataDir, scriptsDir := newScriptFileRegistry(t)
	outside := filepath.Join(dataDir, "target.neko")
	if err := os.WriteFile(outside, []byte("original"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(scriptsDir, "evil.neko")); err != nil {
		t.Skipf("symlink unsupported: %v", err)
	}
	result := reg.WriteScript("evil", baseScript, "")
	if result.OK {
		t.Fatal("symlink write succeeded")
	}
	raw, err := os.ReadFile(outside)
	if err != nil {
		t.Fatal(err)
	}
	if string(raw) != "original" {
		t.Fatalf("symlink target overwritten: %q", raw)
	}
}

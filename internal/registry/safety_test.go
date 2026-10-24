package registry

import (
	"path/filepath"
	"testing"

	"github.com/ruicat/astrbox/internal/config"
	"github.com/ruicat/astrbox/internal/engine"
	"github.com/ruicat/astrbox/internal/entity"
)

func TestSafetyRejectsSensitiveScript(t *testing.T) {
	if err := config.Set(config.Config{
		StateGroup: "状态", StateDefinitionField: "状态定义",
		DefaultStateSubgroup: "身体", SafetyEnabled: true,
	}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(config.Reset)

	dir := t.TempDir()
	store := entity.NewStore(dir)
	if err := store.Load(); err != nil {
		t.Fatal(err)
	}
	machine := engine.New(store)
	reg, err := New(store, machine, filepath.Join(dir, "scripts.json"), filepath.Join(dir, "scripts"))
	if err != nil {
		t.Fatal(err)
	}
	sensitive := `@规则[$] {
  动作: 状态.触发 = "高潮"
}`
	result := reg.Apply("bad_script", sensitive, false, false)
	if result.OK {
		t.Fatal("sensitive script was accepted while safety is enabled")
	}
	if len(result.Errors) == 0 || result.Errors[0].Block != "R18G安全检查" {
		t.Fatalf("errors = %+v", result.Errors)
	}
	// Replay path (skipSafety=true) must stay accepted for pre-existing entries.
	if replay := reg.applyScript("bad_script", sensitive, true, true, false); !replay.OK {
		t.Fatalf("replay was blocked: %+v", replay.Errors)
	}
}

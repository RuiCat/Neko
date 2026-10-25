package bootstrap

import (
	"os"
	"path/filepath"
	"testing"
)

func TestNewAssemblesRuntime(t *testing.T) {
	dataDir := t.TempDir()
	scriptsDir := t.TempDir()
	entities := `{"version":1,"entities":{"猫猫":{"type":"NPC","data":{"身体":{"体力":50}}}}}`
	if err := os.WriteFile(filepath.Join(dataDir, "entities.json"), []byte(entities), 0o644); err != nil {
		t.Fatalf("write entities: %v", err)
	}

	runtime, err := New(Options{DataDir: dataDir, ScriptsDir: scriptsDir})
	if err != nil {
		t.Fatalf("bootstrap: %v", err)
	}
	if runtime.Store.Count() != 1 {
		t.Fatalf("entity count = %d, want 1", runtime.Store.Count())
	}
	if runtime.Store.Get("猫猫") == nil {
		t.Fatal("entity 猫猫 missing after load")
	}
	checks := map[string]bool{
		"machine":     runtime.Machine != nil,
		"funcs":       runtime.Funcs != nil,
		"events":      runtime.Events != nil,
		"registry":    runtime.Registry != nil,
		"diagnostics": runtime.Diagnostics != nil,
		"stateLogger": runtime.StateLogger != nil,
	}
	for name, ok := range checks {
		if !ok {
			t.Errorf("%s not wired", name)
		}
	}
	if runtime.DataDir != dataDir || runtime.ScriptsDir != scriptsDir {
		t.Fatalf("dirs = %q/%q, want %q/%q", runtime.DataDir, runtime.ScriptsDir, dataDir, scriptsDir)
	}
	if runtime.Config.StateGroup != "状态" {
		t.Fatalf("default config not applied: %+v", runtime.Config)
	}
	if runtime.NewMCPServer() == nil {
		t.Fatal("NewMCPServer returned nil")
	}
}

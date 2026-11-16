package config

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestDefaultAndSet(t *testing.T) {
	Reset()
	if got := Current().StateGroup; got != "状态" {
		t.Fatalf("default state group = %q", got)
	}
	custom := Config{StateGroup: "状况", StateDefinitionField: "状态定义", DefaultStateSubgroup: "身体"}
	if err := Set(custom); err != nil {
		t.Fatalf("set: %v", err)
	}
	t.Cleanup(Reset)
	if got := Current().StatePath("情绪"); got != "状况.情绪" {
		t.Fatalf("state path = %q", got)
	}
}

func TestSetRejectsInvalidNames(t *testing.T) {
	Reset()
	t.Cleanup(Reset)
	for _, cfg := range []Config{
		{StateGroup: "", StateDefinitionField: "状态定义", DefaultStateSubgroup: "身体"},
		{StateGroup: "状态.子", StateDefinitionField: "状态定义", DefaultStateSubgroup: "身体"},
		{StateGroup: "状态", StateDefinitionField: "状态 定义", DefaultStateSubgroup: "身体"},
	} {
		if err := Set(cfg); err == nil {
			t.Fatalf("expected validation error for %+v", cfg)
		}
	}
}

func TestLoadMergesDefaults(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	if err := os.WriteFile(path, []byte(`{"state_group":"状况"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if cfg.StateGroup != "状况" || cfg.StateDefinitionField != "状态定义" {
		t.Fatalf("merged config = %+v", cfg)
	}
	missing, err := Load(filepath.Join(dir, "missing.json"))
	if err != nil || !reflect.DeepEqual(missing, Default()) {
		t.Fatalf("missing file: %+v, %v", missing, err)
	}
}

// TestLoadReadsEmoteDir keeps the emote library path config-driven.
func TestLoadReadsEmoteDir(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	if err := os.WriteFile(path, []byte(`{"emote_dir":"/srv/emotes"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if cfg.EmoteDir != "/srv/emotes" {
		t.Fatalf("emote_dir = %q, want /srv/emotes", cfg.EmoteDir)
	}
	if Default().EmoteDir != "" {
		t.Fatalf("default emote_dir = %q, want empty (user-configured only)", Default().EmoteDir)
	}
}

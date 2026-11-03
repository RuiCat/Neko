// Tests for contexts.json loading and validation.
package driver

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestLoadConfigMissingFile(t *testing.T) {
	_, err := LoadConfig(filepath.Join(t.TempDir(), "missing.json"))
	if !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("err = %v, want os.ErrNotExist", err)
	}
}

func TestValidateRejectsMissingToken(t *testing.T) {
	cfg := Config{DshURL: "http://127.0.0.1:3080", Contexts: []Spec{{ContextID: "a", SessionID: "s", Cwd: "/tmp"}}}
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected a missing-token error")
	}
}

func TestValidateRejectsBadURL(t *testing.T) {
	cfg := Config{DshURL: "not-a-url", DshToken: "t"}
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected an invalid-url error")
	}
}

func TestValidateRejectsDuplicateSession(t *testing.T) {
	cfg := Config{
		DshURL: "http://127.0.0.1:3080", DshToken: "t",
		Contexts: []Spec{
			{ContextID: "a", SessionID: "same", Cwd: "/tmp"},
			{ContextID: "b", SessionID: "same", Cwd: "/tmp"},
		},
	}
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected a duplicate-session error")
	}
}

func TestLoadConfigAppliesEnvToken(t *testing.T) {
	path := filepath.Join(t.TempDir(), "contexts.json")
	raw := `{"dsh_url":"http://127.0.0.1:3080","dsh_token":"file-token","contexts":[]}`
	if err := os.WriteFile(path, []byte(raw), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	t.Setenv("NEKO_DSH_TOKEN", "env-token")
	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if cfg.DshToken != "env-token" {
		t.Fatalf("token = %q, want env-token", cfg.DshToken)
	}
}

func TestWithDataDirResolvesDefaultRegistry(t *testing.T) {
	cfg := Config{}.WithDataDir("/data/engine")
	want := filepath.Join("/data/engine", "contexts.json")
	if cfg.RegistryPath != want {
		t.Fatalf("registry = %q, want %q", cfg.RegistryPath, want)
	}
	custom := Config{RegistryPath: "/custom/registry.json"}.WithDataDir("/data/engine")
	if custom.RegistryPath != "/custom/registry.json" {
		t.Fatalf("custom registry was rewritten: %q", custom.RegistryPath)
	}
}

func TestValidateRejectsUnknownRouteContext(t *testing.T) {
	cfg := Config{
		DshURL: "http://127.0.0.1:3080", DshToken: "t",
		Contexts: []Spec{{ContextID: "a", SessionID: "s", Cwd: "/tmp"}},
		Routes:   []Route{{Event: "心情变化", ContextID: "missing"}},
	}
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected an unknown-route-context error")
	}
}

func TestValidateRejectsBadRoutePriority(t *testing.T) {
	cfg := Config{
		DshURL: "http://127.0.0.1:3080", DshToken: "t",
		Contexts: []Spec{{ContextID: "a", SessionID: "s", Cwd: "/tmp"}},
		Routes:   []Route{{Event: "心情变化", ContextID: "a", Priority: "high"}},
	}
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected a bad-route-priority error")
	}
}

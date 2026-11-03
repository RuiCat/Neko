// contexts.json loading: the DSH endpoint, credential and character contexts.
package driver

import (
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
)

// DEFAULT_REGISTRY_PATH is where runtime context state is persisted when the
// configuration does not name another file.
const DEFAULT_REGISTRY_PATH = "data/contexts.json"

// Config is the contexts.json document: one DSH Web GUI plus every character
// context Neko may drive.
type Config struct {
	// DshURL is the DSH Web GUI origin, for example http://127.0.0.1:3080.
	DshURL string `json:"dsh_url"`
	// DshToken is the per-process launch token printed by `dsh web`. Prefer the
	// NEKO_DSH_TOKEN environment variable over storing it in a file.
	DshToken string `json:"dsh_token"`
	// RegistryPath persists context status and the last finished turn.
	RegistryPath string `json:"registry_path"`
	// Contexts declares the character contexts to open and drive.
	Contexts []Spec `json:"contexts"`
	// Routes maps .neko events to the contexts they invoke.
	Routes []Route `json:"routes"`
}

// Route declares that one .neko event invokes one context.
type Route struct {
	// Event is the @事件 name that triggers the route.
	Event string `json:"event"`
	// ContextID is the target context.
	ContextID string `json:"context_id"`
	// Priority is normal (default) or urgent.
	Priority string `json:"priority"`
	// Template renders the prompt body; empty uses the default card.
	Template string `json:"template"`
}

// Spec declares one character context: a stable DSH session plus its persona
// preset and the workspace directory the session owns.
type Spec struct {
	// ContextID is Neko's stable identity for this context.
	ContextID string `json:"context_id"`
	// CharacterID is the engine entity that supplies the character state.
	CharacterID string `json:"character_id"`
	// SessionID is the DSH session identity; reusing it resumes the session.
	SessionID string `json:"session_id"`
	// PresetID selects the DSH agent preset that carries the persona.
	PresetID string `json:"preset_id"`
	// Cwd is the workspace directory the session owns.
	Cwd string `json:"cwd"`
	// Entity overrides CharacterID for the state snapshot entity.
	Entity string `json:"entity"`
}

// entityID is the entity the context reads state from.
func (s Spec) entityID() string {
	if strings.TrimSpace(s.Entity) != "" {
		return s.Entity
	}
	return s.CharacterID
}

// LoadConfig reads path, applies NEKO_DSH_URL/NEKO_DSH_TOKEN overrides and
// validates the result. A missing file returns the os.ErrNotExist chain so the
// caller can keep the driver disabled.
func LoadConfig(path string) (Config, error) {
	cfg := Config{RegistryPath: DEFAULT_REGISTRY_PATH}
	raw, err := os.ReadFile(path)
	if err != nil {
		return cfg, err
	}
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return cfg, fmt.Errorf("解析 %s 失败: %w", path, err)
	}
	if value := strings.TrimSpace(os.Getenv("NEKO_DSH_URL")); value != "" {
		cfg.DshURL = value
	}
	if value := os.Getenv("NEKO_DSH_TOKEN"); value != "" {
		cfg.DshToken = value
	}
	if cfg.RegistryPath == "" {
		cfg.RegistryPath = DEFAULT_REGISTRY_PATH
	}
	if err := cfg.Validate(); err != nil {
		return cfg, err
	}
	return cfg, nil
}

// WithDataDir resolves the default registry path against the engine data
// directory; an explicitly configured path is kept as written.
func (c Config) WithDataDir(dataDir string) Config {
	if c.RegistryPath == "" || c.RegistryPath == DEFAULT_REGISTRY_PATH {
		c.RegistryPath = filepath.Join(dataDir, "contexts.json")
	}
	return c
}

// Validate checks the endpoint, the credential and every declared context.
func (c Config) Validate() error {
	parsed, err := url.Parse(c.DshURL)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return fmt.Errorf("dsh_url 必须是 http(s) 绝对地址: %q", c.DshURL)
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return fmt.Errorf("dsh_url 只支持 http/https: %q", c.DshURL)
	}
	if strings.TrimSpace(c.DshToken) == "" {
		return fmt.Errorf("缺少 dsh_token（建议用 NEKO_DSH_TOKEN 环境变量）")
	}
	seenContext := map[string]bool{}
	seenSession := map[string]bool{}
	for index, spec := range c.Contexts {
		if strings.TrimSpace(spec.ContextID) == "" || strings.TrimSpace(spec.SessionID) == "" {
			return fmt.Errorf("contexts[%d] 缺少 context_id 或 session_id", index)
		}
		if strings.TrimSpace(spec.Cwd) == "" {
			return fmt.Errorf("contexts[%d] 缺少 cwd", index)
		}
		if seenContext[spec.ContextID] {
			return fmt.Errorf("context_id 重复: %s", spec.ContextID)
		}
		if seenSession[spec.SessionID] {
			return fmt.Errorf("session_id 重复: %s", spec.SessionID)
		}
		seenContext[spec.ContextID] = true
		seenSession[spec.SessionID] = true
	}
	for index, route := range c.Routes {
		if strings.TrimSpace(route.Event) == "" {
			return fmt.Errorf("routes[%d] 缺少 event", index)
		}
		if !seenContext[route.ContextID] {
			return fmt.Errorf("routes[%d] 指向未声明的 context_id: %q", index, route.ContextID)
		}
		if route.Priority != "" && route.Priority != PRIORITY_NORMAL && route.Priority != PRIORITY_URGENT {
			return fmt.Errorf("routes[%d] priority 只能是 normal 或 urgent", index)
		}
	}
	return nil
}

// Package config holds the domain vocabulary that used to be hard-coded in the
// engine: the state group, the state-definition field and the default subgroup.
// Defaults preserve the original .neko vocabulary; a JSON file may override them.
package config

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"sync/atomic"
)

// Config is the process-wide domain configuration.
type Config struct {
	// StateGroup is the top-level entity field holding machine states (默认 "状态").
	StateGroup string `json:"state_group"`
	// StateDefinitionField declares the allowed values per state group (默认 "状态定义").
	StateDefinitionField string `json:"state_definition_field"`
	// DefaultStateSubgroup is used by the `状态 = "x"` sugar (默认 "身体").
	DefaultStateSubgroup string `json:"default_state_subgroup"`
	// SafetyEnabled turns on the optional R18G word-list scan (默认关闭).
	SafetyEnabled bool `json:"safety_enabled"`
	// HttpToken protects the HTTP transport (empty = no auth).
	HttpToken string `json:"http_token"`
	// GlobalEntities are referenced without the {e} prefix.
	GlobalEntities []string `json:"global_entities"`
	// DefaultScene receives emit operations without an explicit scene.
	DefaultScene string `json:"default_scene"`
	// SceneEventField holds a scene's event list.
	SceneEventField string `json:"scene_event_field"`
	// ClassificationField holds an entity's multi-inheritance labels.
	ClassificationField string `json:"classification_field"`
}

// Default returns the vocabulary used by the bundled data and scripts.
func Default() Config {
	return Config{
		StateGroup:           "状态",
		StateDefinitionField: "状态定义",
		DefaultStateSubgroup: "身体",
		GlobalEntities:       []string{"场景", "世界", "时间", "天气", "全局"},
		DefaultScene:         "场景",
		SceneEventField:      "事件",
		ClassificationField:  "分类",
	}
}

var current atomic.Value

func init() { current.Store(Default()) }

// Current returns the active configuration.
func Current() Config { return current.Load().(Config) }

// Set validates and installs a configuration process-wide.
func Set(cfg Config) error {
	if err := cfg.Validate(); err != nil {
		return err
	}
	current.Store(cfg)
	return nil
}

// Reset restores the defaults (used by tests).
func Reset() { current.Store(Default()) }

// Validate rejects names that cannot be used as dotted path segments.
func (c Config) Validate() error {
	fields := map[string]string{
		"state_group":            c.StateGroup,
		"state_definition_field": c.StateDefinitionField,
		"default_state_subgroup": c.DefaultStateSubgroup,
	}
	for key, value := range fields {
		if strings.TrimSpace(value) == "" {
			return fmt.Errorf("%s 不能为空", key)
		}
		if strings.ContainsAny(value, ".[] ") {
			return fmt.Errorf("%s '%s' 不能包含 . [ ] 或空格", key, value)
		}
	}
	for _, name := range c.GlobalEntities {
		if strings.TrimSpace(name) == "" || strings.ContainsAny(name, ".[] ") {
			return fmt.Errorf("global_entities 含非法名称 '%s'", name)
		}
	}
	return nil
}

// IsGlobalEntity reports whether a reference is a global entity.
func (c Config) IsGlobalEntity(name string) bool {
	for _, candidate := range c.GlobalEntities {
		if candidate == name {
			return true
		}
	}
	return false
}

// Load reads a JSON config file and merges it onto Default(). A missing file is
// not an error: defaults are returned (config is optional).
func Load(path string) (Config, error) {
	cfg := Default()
	if path == "" {
		return cfg, nil
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return cfg, nil
		}
		return cfg, err
	}
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return cfg, fmt.Errorf("解析配置文件失败: %w", err)
	}
	if err := cfg.Validate(); err != nil {
		return cfg, err
	}
	return cfg, nil
}

// StatePath joins a subgroup under the configured state group.
func (c Config) StatePath(subgroup string) string {
	if subgroup == "" {
		return c.StateGroup
	}
	return c.StateGroup + "." + subgroup
}

// NormalizePath rewrites the legacy leading "状态." / "身体." segments to the
// configured vocabulary. Non-legacy paths are returned unchanged.
func (c Config) NormalizePath(path string) string {
	if strings.HasPrefix(path, "状态.") {
		path = c.StateGroup + "." + path[len("状态."):]
	}
	if strings.HasPrefix(path, "身体.") {
		path = c.DefaultStateSubgroup + "." + path[len("身体."):]
	}
	return path
}

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
	// CurrentScenePath is the entity field holding the id of the scene it is in.
	// A scene target that is not a real entity (the default-scene token) resolves
	// through it, so `场景` means "the actor's current scene" (默认 "位置.场景").
	CurrentScenePath string `json:"current_scene_path"`
	// SceneEventField holds a scene's event list.
	SceneEventField string `json:"scene_event_field"`
	// SceneEventLimit keeps only the newest entries of that list; 0 = unbounded.
	// The board only ever projects the newest entry (bounded push).
	SceneEventLimit int `json:"scene_event_limit"`
	// SceneOccupancyField is the scene field holding the ids of the actors in it.
	// Every tick reconciles it from each actor's CurrentScenePath, so a move is a
	// scene change other agents can see (空 = 关闭同步).
	SceneOccupancyField string `json:"scene_occupancy_field"`
	// ClassificationField holds an entity's multi-inheritance labels.
	ClassificationField string `json:"classification_field"`
	// EffectMap maps a free-text effect word to a dotted field path. It is data, not source: empty by default, effects then
	// resolve the word against the target entity's own leaf paths.
	EffectMap map[string]string `json:"effect_map"`
	// AssignMap maps a free-text assignment word ("身体状态") to a dotted path.
	// Data, not source: empty by default, assignments resolve against the entity.
	AssignMap map[string]string `json:"assign_map"`
	// BareFieldMap maps an unqualified script field ("体力") to a dotted path.
	// Data, not source: empty by default, bare fields then stay unqualified.
	BareFieldMap map[string]string `json:"bare_field_map"`
	// HardStatePaths is the field-level hard-state subset outside StateGroup.
	// Data, not source; empty means "no extra paths".
	HardStatePaths []string `json:"hard_state_paths"`
	// DefaultEntity is the entity a session event resolves against when the
	// caller gives none. Data, not source: empty means "no default".
	DefaultEntity string `json:"default_entity"`
	// WorldEntity / WorldTimePath / WorldMonthPath / WorldPhasePath describe the
	// single world-clock entity. Data-overridable; defaults match the bundled
	// world template.
	WorldEntity    string `json:"world_entity"`
	WorldTimePath  string `json:"world_time_path"`
	WorldMonthPath string `json:"world_month_path"`
	WorldPhasePath string `json:"world_phase_path"`
	// Persona is the fallback system persona for the local chat command.
	// Data, not source: empty means a neutral one-line default.
	Persona string `json:"persona"`
}

// Default returns the vocabulary used by the bundled data and scripts.
func Default() Config {
	return Config{
		StateGroup:           "状态",
		StateDefinitionField: "状态定义",
		DefaultStateSubgroup: "身体",
		GlobalEntities:       []string{"场景", "世界", "时间", "天气", "全局"},
		DefaultScene:         "场景",
		CurrentScenePath:     "位置.场景",
		SceneEventField:      "事件",
		SceneEventLimit:      50,
		SceneOccupancyField:  "在场",
		ClassificationField:  "分类",
		EffectMap:            map[string]string{},
		AssignMap:            map[string]string{},
		BareFieldMap:         map[string]string{},
		HardStatePaths:       []string{},
		WorldEntity:          "世界",
		WorldTimePath:        "环境.当前时间",
		WorldMonthPath:       "环境.当前月份",
		WorldPhasePath:       "状态.阶段",
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
	// 新加字段允许留空（老配置与测试里的 Config 字面量）：留空走 Default 的路径。
	if c.CurrentScenePath != "" && invalidPath(c.CurrentScenePath) {
		return fmt.Errorf("current_scene_path '%s' 不是合法的点分路径", c.CurrentScenePath)
	}
	if c.SceneEventLimit < 0 {
		return fmt.Errorf("scene_event_limit 不能为负数")
	}
	if c.SceneOccupancyField != "" && invalidPath(c.SceneOccupancyField) {
		return fmt.Errorf("scene_occupancy_field '%s' 不是合法的点分路径", c.SceneOccupancyField)
	}
	return nil
}

// invalidPath reports whether a dotted path cannot be a field path.
func invalidPath(path string) bool {
	trimmed := strings.TrimSpace(path)
	if trimmed == "" || strings.HasPrefix(trimmed, ".") || strings.HasSuffix(trimmed, ".") {
		return true
	}
	return strings.ContainsAny(trimmed, "[] ") || strings.Contains(trimmed, "..")
}

// ScenePath returns the configured current-scene path; an empty value (old
// config or a Config literal) falls back to the built-in default.
func (c Config) ScenePath() string {
	if strings.TrimSpace(c.CurrentScenePath) == "" {
		return Default().CurrentScenePath
	}
	return strings.TrimSpace(c.CurrentScenePath)
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

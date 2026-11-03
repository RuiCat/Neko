// rules.go persists the engine rules produced by an approved repair so a
// restart keeps the fix and a rollback can take it back out. Rules live under
// <dataDir>/repairs/rules.json.
package repair

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/RuiCat/Neko/internal/engine"
)

// RulesPath returns the repaired-rule file for one data directory.
func RulesPath(dataDir string) string {
	return filepath.Join(dataDir, "repairs", "rules.json")
}

// LoadRules reads the applied repair rules; a missing file is empty.
func LoadRules(path string) ([]engine.Rule, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("repair: 读取规则文件失败: %w", err)
	}
	rules := []engine.Rule{}
	if err := json.Unmarshal(raw, &rules); err != nil {
		return nil, fmt.Errorf("repair: 解析规则文件失败: %w", err)
	}
	return rules, nil
}

// UpsertRule adds or replaces one rule by ID.
func UpsertRule(path string, rule engine.Rule) error {
	rules, err := LoadRules(path)
	if err != nil {
		return err
	}
	replaced := false
	for index := range rules {
		if rules[index].ID == rule.ID {
			rules[index] = rule
			replaced = true
			break
		}
	}
	if !replaced {
		rules = append(rules, rule)
	}
	return saveRules(path, rules)
}

// RemoveRule deletes one rule by ID and reports whether it existed.
func RemoveRule(path, id string) (bool, error) {
	rules, err := LoadRules(path)
	if err != nil {
		return false, err
	}
	kept := make([]engine.Rule, 0, len(rules))
	removed := false
	for _, rule := range rules {
		if rule.ID == id {
			removed = true
			continue
		}
		kept = append(kept, rule)
	}
	if !removed {
		return false, nil
	}
	return true, saveRules(path, kept)
}

// saveRules writes the whole rule set atomically.
func saveRules(path string, rules []engine.Rule) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("repair: 创建目录失败: %w", err)
	}
	raw, err := json.MarshalIndent(rules, "", "  ")
	if err != nil {
		return fmt.Errorf("repair: 序列化规则失败: %w", err)
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, raw, 0o644); err != nil {
		return fmt.Errorf("repair: 写临时文件失败: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("repair: 替换规则文件失败: %w", err)
	}
	return nil
}

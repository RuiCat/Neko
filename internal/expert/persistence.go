// persistence.go saves and restores expert contexts as one JSON file so a
// restart does not throw away the learned working memory. The current shape is
// v2 (one pool per entity); the retired v1 global pool is kept only for the
// legacy SaveFile/LoadFile pair and is migrated by backup + empty start.
package expert

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
)

// PERSIST_VERSION is the current on-disk file shape: per-entity pools.
const PERSIST_VERSION = 2

// LEGACY_PERSIST_VERSION is the retired global-pool shape. SaveFile/LoadFile
// keep it for the old registry tests; production loads go through the Set.
const LEGACY_PERSIST_VERSION = 1

// persistedSet is the v2 on-disk shape: 角色:猫猫 -> 事件记忆 -> entries.
type persistedSet struct {
	Version  int                           `json:"version"`
	Entities map[string]map[string][]Entry `json:"entities"`
}

// persistedRegistry is the retired v1 on-disk shape (one global pool).
type persistedRegistry struct {
	Version int                `json:"version"`
	Experts map[string][]Entry `json:"experts"`
}

// SaveSetFile writes every entity pool atomically to path.
func SaveSetFile(path string, set *Set) error {
	if set == nil {
		return fmt.Errorf("expert: 专家集合未配置")
	}
	payload := persistedSet{Version: PERSIST_VERSION, Entities: set.Snapshot()}
	raw, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return fmt.Errorf("expert: 序列化上下文失败: %w", err)
	}
	return writeFileAtomic(path, raw)
}

// LoadSetFile restores per-entity contexts from path. A missing file is not an
// error (first run); a malformed or unknown-version file is, so a corrupt
// store is not silently ignored. A v1 file cannot be attributed to entities:
// it is copied to <path>.v1.bak and the set starts empty (the next Save
// overwrites the original file).
func LoadSetFile(path string, set *Set) error {
	if set == nil {
		return fmt.Errorf("expert: 专家集合未配置")
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("expert: 读取上下文失败: %w", err)
	}
	var header struct {
		Version int `json:"version"`
	}
	if err := json.Unmarshal(raw, &header); err != nil {
		return fmt.Errorf("expert: 解析上下文失败: %w", err)
	}
	switch header.Version {
	case PERSIST_VERSION:
		payload := persistedSet{}
		if err := json.Unmarshal(raw, &payload); err != nil {
			return fmt.Errorf("expert: 解析上下文失败: %w", err)
		}
		set.Load(payload.Entities)
		return nil
	case LEGACY_PERSIST_VERSION:
		// v1 是全局单池，无法归属实体：先备份，空状态启动，原文件留给后续 Save 覆盖。
		backup := path + ".v1.bak"
		if err := os.WriteFile(backup, raw, 0o644); err != nil {
			return fmt.Errorf("expert: 备份 v1 上下文失败: %w", err)
		}
		log.Printf("expert: 旧版专家上下文已备份到 %s，按空状态启动（v1 无法按实体归属）", backup)
		return nil
	default:
		return fmt.Errorf("expert: 上下文版本 %d 不受支持（期望 %d）", header.Version, PERSIST_VERSION)
	}
}

// SaveFile writes one global registry atomically in the retired v1 shape.
func SaveFile(path string, registry *Registry) error {
	payload := persistedRegistry{Version: LEGACY_PERSIST_VERSION, Experts: registry.Snapshot()}
	raw, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return fmt.Errorf("expert: 序列化上下文失败: %w", err)
	}
	return writeFileAtomic(path, raw)
}

// LoadFile restores one global registry from the retired v1 shape.
func LoadFile(path string, registry *Registry) error {
	raw, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("expert: 读取上下文失败: %w", err)
	}
	payload := persistedRegistry{}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return fmt.Errorf("expert: 解析上下文失败: %w", err)
	}
	if payload.Version != LEGACY_PERSIST_VERSION {
		return fmt.Errorf("expert: 上下文版本 %d 不受支持（期望 %d）", payload.Version, LEGACY_PERSIST_VERSION)
	}
	registry.Load(payload.Experts)
	return nil
}

// writeFileAtomic replaces path through a synced temp file, so a crash never
// leaves a half-written store behind.
func writeFileAtomic(path string, raw []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("expert: 创建目录失败: %w", err)
	}
	tmp := path + ".tmp"
	f, err := os.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o644)
	if err != nil {
		return fmt.Errorf("expert: 写临时文件失败: %w", err)
	}
	if _, err := f.Write(raw); err != nil {
		_ = f.Close()
		_ = os.Remove(tmp)
		return fmt.Errorf("expert: 写临时文件失败: %w", err)
	}
	if err := f.Sync(); err != nil {
		_ = f.Close()
		_ = os.Remove(tmp)
		return fmt.Errorf("expert: 同步临时文件失败: %w", err)
	}
	if err := f.Close(); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("expert: 关闭临时文件失败: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("expert: 替换上下文文件失败: %w", err)
	}
	return nil
}

// injectedUserPrefixes 是桥接注入的 user 文本前缀：这些消息不是主人说的话。
// 与插件侧 `injectedText.mjs` 的清单保持一致。
var injectedUserPrefixes = []string{"[astrbox-", "[context-guard]"}

// DropInjectedEchoes removes memory entries that only exist because a bridge
// push was recorded as user input: the entry's user half is an injected push
// (「用户: [astrbox-observation] …」), or the entry has no user half at all
// (「助手: …」, written for a push-triggered turn). Recalling them makes the NPC
// echo its own pushes and replies back into the next compensation package.
// It reports how many entries were dropped.
func DropInjectedEchoes(registry *Registry) int {
	if registry == nil {
		return 0
	}
	snapshot := registry.Snapshot()
	dropped := 0
	for id, entries := range snapshot {
		kept := entries[:0]
		for _, entry := range entries {
			if isInjectedEcho(entry.Content) {
				dropped++
				continue
			}
			kept = append(kept, entry)
		}
		snapshot[id] = kept
	}
	if dropped > 0 {
		registry.Load(snapshot)
	}
	return dropped
}

// isInjectedEcho reports whether one memory entry is an injected-push echo.
func isInjectedEcho(content string) bool {
	if strings.HasPrefix(content, "助手: ") {
		return true
	}
	user, ok := strings.CutPrefix(content, "用户: ")
	if !ok {
		return false
	}
	for _, prefix := range injectedUserPrefixes {
		if strings.HasPrefix(user, prefix) {
			return true
		}
	}
	return false
}

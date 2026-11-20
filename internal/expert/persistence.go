// persistence.go saves and restores expert contexts as one JSON file so a
// restart does not throw away the learned working memory.
package expert

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// PERSIST_VERSION is bumped when the file shape changes.
const PERSIST_VERSION = 1

// persistedRegistry is the on-disk shape.
type persistedRegistry struct {
	Version int                `json:"version"`
	Experts map[string][]Entry `json:"experts"`
}

// SaveFile writes every expert context atomically to path.
func SaveFile(path string, registry *Registry) error {
	payload := persistedRegistry{Version: PERSIST_VERSION, Experts: registry.Snapshot()}
	raw, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return fmt.Errorf("expert: 序列化上下文失败: %w", err)
	}
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

// LoadFile restores contexts from path. A missing file is not an error (first
// run); a malformed file is, so a corrupt store is not silently ignored.
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
	if payload.Version != PERSIST_VERSION {
		return fmt.Errorf("expert: 上下文版本 %d 不受支持（期望 %d）", payload.Version, PERSIST_VERSION)
	}
	registry.Load(payload.Experts)
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

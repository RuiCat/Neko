// write.go persists the day's triage result as JSONL. The write is atomic so a
// crash never leaves a half-written triage file behind.
package triage

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// WriteFile writes findings as one JSON object per line, atomically.
func WriteFile(path string, findings []Finding) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("triage: 创建目录失败: %w", err)
	}
	raw := make([]byte, 0, len(findings)*160)
	for _, finding := range findings {
		line, err := json.Marshal(finding)
		if err != nil {
			return fmt.Errorf("triage: 序列化 finding 失败: %w", err)
		}
		raw = append(raw, line...)
		raw = append(raw, '\n')
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, raw, 0o644); err != nil {
		return fmt.Errorf("triage: 写临时文件失败: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("triage: 替换 triage 文件失败: %w", err)
	}
	return nil
}

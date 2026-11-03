// Package annotations reads the human labels behind the B-class metrics:
// which memory fragments were actually used and which past events the answer
// cited. One JSONL line per annotated turn.
package annotations

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Record is one annotated turn.
type Record struct {
	TurnID        string     `json:"turn_id"`
	UsedFragments []int      `json:"used_fragments"`
	Cites         []Citation `json:"cites"`
	Correct       *bool      `json:"correct,omitempty"`
	Annotator     string     `json:"annotator"`
	CreatedAt     string     `json:"created_at"`
}

// Citation is one past event the answer cites.
type Citation struct {
	EventID string `json:"event_id"`
	Kind    string `json:"kind"`
}

// LoadDir reads every *.jsonl file in dir and indexes records by turn ID.
// Multiple annotators may label the same turn; the same (turn, annotator)
// pair twice, a malformed line or a failed validation is an error.
func LoadDir(dir string) (map[string][]Record, error) {
	index := map[string][]Record{}
	seen := map[string]bool{}
	paths, err := filepath.Glob(filepath.Join(dir, "*.jsonl"))
	if err != nil {
		return nil, fmt.Errorf("annotations: 扫描 %s 失败: %w", dir, err)
	}
	for _, path := range paths {
		file, err := os.Open(path)
		if err != nil {
			return nil, fmt.Errorf("annotations: 打开 %s 失败: %w", path, err)
		}
		scanner := bufio.NewScanner(file)
		scanner.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
		lineNumber := 0
		for scanner.Scan() {
			lineNumber++
			line := strings.TrimSpace(scanner.Text())
			if line == "" {
				continue
			}
			var record Record
			if err := json.Unmarshal([]byte(line), &record); err != nil {
				file.Close()
				return nil, fmt.Errorf("annotations: %s:%d 解析失败: %w", path, lineNumber, err)
			}
			if err := Validate(record); err != nil {
				file.Close()
				return nil, fmt.Errorf("annotations: %s:%d %w", path, lineNumber, err)
			}
			key := record.TurnID + "\x00" + record.Annotator
			if seen[key] {
				file.Close()
				return nil, fmt.Errorf("annotations: turn %q 的标注者 %q 重复标注", record.TurnID, record.Annotator)
			}
			seen[key] = true
			index[record.TurnID] = append(index[record.TurnID], record)
		}
		scanErr := scanner.Err()
		file.Close()
		if scanErr != nil {
			return nil, fmt.Errorf("annotations: 读取 %s 失败: %w", path, scanErr)
		}
	}
	return index, nil
}

// Validate checks the fields the metrics rely on.
func Validate(record Record) error {
	if strings.TrimSpace(record.TurnID) == "" {
		return fmt.Errorf("缺少 turn_id")
	}
	if strings.TrimSpace(record.Annotator) == "" {
		return fmt.Errorf("turn %q 缺少 annotator", record.TurnID)
	}
	if strings.TrimSpace(record.CreatedAt) == "" {
		return fmt.Errorf("turn %q 缺少 created_at", record.TurnID)
	}
	for _, index := range record.UsedFragments {
		if index < 0 {
			return fmt.Errorf("turn %q 的 used_fragments 含负数 %d", record.TurnID, index)
		}
	}
	for _, cite := range record.Cites {
		if strings.TrimSpace(cite.EventID) == "" {
			return fmt.Errorf("turn %q 的 cites 含空 event_id", record.TurnID)
		}
	}
	return nil
}

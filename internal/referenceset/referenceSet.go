// referenceSet.go loads and validates the frozen reference set: the external
// anchor that never takes part in training, only in the online gate.
package referenceset

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"
)

// Entry is one reference scenario with the reactions it must and must not
// produce.
type Entry struct {
	ID                 string   `json:"id"`
	Scenario           string   `json:"scenario"`
	Context            string   `json:"context"`
	ExpectedReactions  []string `json:"expected_reactions"`
	ForbiddenReactions []string `json:"forbidden_reactions"`
	Author             string   `json:"author"`
	CreatedAt          string   `json:"created_at"`
	Locked             bool     `json:"locked"`
	Note               string   `json:"note,omitempty"`
}

// MIN_EXPECTED_REACTIONS is the lower bound of usable reference entries; a
// single reaction cannot express the acceptable range of a persona.
const MIN_EXPECTED_REACTIONS = 2

// LoadFile reads a JSONL reference set.
func LoadFile(path string) ([]Entry, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("referenceset: 打开 %s 失败: %w", path, err)
	}
	defer file.Close()
	entries := []Entry{}
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	lineNumber := 0
	for scanner.Scan() {
		lineNumber++
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var entry Entry
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			return nil, fmt.Errorf("referenceset: %s:%d 解析失败: %w", path, lineNumber, err)
		}
		entries = append(entries, entry)
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("referenceset: 读取 %s 失败: %w", path, err)
	}
	return entries, nil
}

// Validate checks the invariants that make the set a usable anchor: unique
// IDs, locked entries, concrete scenario text and at least the minimum number
// of expected reactions.
func Validate(entries []Entry) error {
	if len(entries) == 0 {
		return fmt.Errorf("referenceset: 参照集为空")
	}
	seen := map[string]bool{}
	for index, entry := range entries {
		position := fmt.Sprintf("第 %d 条", index+1)
		if strings.TrimSpace(entry.ID) == "" {
			return fmt.Errorf("referenceset: %s 缺少 id", position)
		}
		if seen[entry.ID] {
			return fmt.Errorf("referenceset: id %q 重复", entry.ID)
		}
		seen[entry.ID] = true
		if !entry.Locked {
			return fmt.Errorf("referenceset: %s (%s) 未锁定", position, entry.ID)
		}
		if strings.TrimSpace(entry.Scenario) == "" || strings.TrimSpace(entry.Context) == "" {
			return fmt.Errorf("referenceset: %s (%s) 缺少 scenario 或 context", position, entry.ID)
		}
		if len(entry.ExpectedReactions) < MIN_EXPECTED_REACTIONS {
			return fmt.Errorf("referenceset: %s (%s) 期望反应少于 %d 条", position, entry.ID, MIN_EXPECTED_REACTIONS)
		}
		if len(entry.ForbiddenReactions) == 0 {
			return fmt.Errorf("referenceset: %s (%s) 缺少 forbidden_reactions", position, entry.ID)
		}
		if _, err := time.Parse("2006-01-02", entry.CreatedAt); err != nil {
			return fmt.Errorf("referenceset: %s (%s) created_at 不是 YYYY-MM-DD: %q", position, entry.ID, entry.CreatedAt)
		}
	}
	return nil
}

// HashFile returns the SHA-256 hex digest of path, the value stored next to the
// frozen set so any silent edit is detected.
func HashFile(path string) (string, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("referenceset: 读取 %s 失败: %w", path, err)
	}
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:]), nil
}

// Verify checks the file against a hash sidecar whose first field is the
// expected digest.
func Verify(path, hashPath string) error {
	expected, err := HashFile(path)
	if err != nil {
		return err
	}
	raw, err := os.ReadFile(hashPath)
	if err != nil {
		return fmt.Errorf("referenceset: 读取 %s 失败: %w", hashPath, err)
	}
	fields := strings.Fields(string(raw))
	if len(fields) == 0 {
		return fmt.Errorf("referenceset: %s 为空", hashPath)
	}
	if !strings.EqualFold(fields[0], expected) {
		return fmt.Errorf("referenceset: 哈希不匹配（期望 %s，实际 %s）", fields[0], expected)
	}
	return nil
}

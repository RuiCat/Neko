// Package candidates stores state-transition candidates that no enabled rule
// covers (C3). They wait for review instead of blocking the turn.
package candidates

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// RECORD_VERSION is bumped when the on-disk shape changes.
const RECORD_VERSION = 1

// Statuses for a queued candidate.
const (
	STATUS_PENDING  = "pending"
	STATUS_APPROVED = "approved"
	STATUS_REJECTED = "rejected"
)

// Record is one uncovered transition candidate.
type Record struct {
	RecordVersion int       `json:"record_version"`
	ID            string    `json:"id"`
	EntityID      string    `json:"entity_id"`
	Path          string    `json:"path"`
	Proposed      any       `json:"proposed,omitempty"`
	Source        string    `json:"source,omitempty"`
	Evidence      []string  `json:"evidence,omitempty"`
	Status        string    `json:"status"`
	Reason        string    `json:"reason,omitempty"`
	CreatedAt     time.Time `json:"created_at"`
}

// Store appends candidates to one JSONL file.
type Store struct {
	mu   sync.Mutex
	path string
}

// NewStore returns a store writing to path.
func NewStore(path string) *Store {
	return &Store{path: path}
}

// Append validates and appends one candidate, filling ID/version/status/time.
// The normalised record is returned so callers can report the assigned ID.
func (s *Store) Append(record Record) (Record, error) {
	if strings.TrimSpace(record.EntityID) == "" {
		return Record{}, fmt.Errorf("candidates: 缺少 entity_id")
	}
	if strings.TrimSpace(record.Path) == "" {
		return Record{}, fmt.Errorf("candidates: 缺少 path")
	}
	if record.ID == "" {
		record.ID = fmt.Sprintf("cand_%d", time.Now().UnixNano())
	}
	if record.CreatedAt.IsZero() {
		record.CreatedAt = time.Now()
	}
	record.RecordVersion = RECORD_VERSION
	if record.Status == "" {
		record.Status = STATUS_PENDING
	}
	raw, err := json.Marshal(record)
	if err != nil {
		return Record{}, fmt.Errorf("candidates: 序列化失败: %w", err)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return Record{}, fmt.Errorf("candidates: 创建目录失败: %w", err)
	}
	file, err := os.OpenFile(s.path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return Record{}, fmt.Errorf("candidates: 打开 %s 失败: %w", s.path, err)
	}
	defer file.Close()
	if _, err := file.Write(append(raw, '\n')); err != nil {
		return Record{}, fmt.Errorf("candidates: 写入失败: %w", err)
	}
	return record, nil
}

// List returns the latest record per ID with the given status (empty status
// returns all). Later lines for the same ID win, so resolutions are append-only.
func (s *Store) List(status string) ([]Record, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	file, err := os.Open(s.path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("candidates: 打开 %s 失败: %w", s.path, err)
	}
	defer file.Close()
	records := []Record{}
	index := map[string]int{}
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var record Record
		if err := json.Unmarshal([]byte(line), &record); err != nil {
			return nil, fmt.Errorf("candidates: 解析失败: %w", err)
		}
		if position, ok := index[record.ID]; ok && record.ID != "" {
			records[position] = record
			continue
		}
		index[record.ID] = len(records)
		records = append(records, record)
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("candidates: 读取失败: %w", err)
	}
	if status == "" {
		return records, nil
	}
	filtered := records[:0]
	for _, record := range records {
		if record.Status == status {
			filtered = append(filtered, record)
		}
	}
	return filtered, nil
}

// Resolve appends a new state for one candidate (approved/rejected). The
// original line stays as history; List folds to the newest state.
func (s *Store) Resolve(id, status, note string) (Record, error) {
	if status != STATUS_APPROVED && status != STATUS_REJECTED {
		return Record{}, fmt.Errorf("candidates: 非法处置状态 %q", status)
	}
	records, err := s.List("")
	if err != nil {
		return Record{}, err
	}
	var found *Record
	for position := range records {
		if records[position].ID == id {
			candidate := records[position]
			found = &candidate
			break
		}
	}
	if found == nil {
		return Record{}, fmt.Errorf("candidates: 候选 %q 不存在", id)
	}
	record := *found
	record.Status = status
	if strings.TrimSpace(note) != "" {
		record.Reason = note
	}
	return s.Append(record)
}

// DEFAULT_TTL is how long a pending candidate waits before the nightly pass
// expires it.
const DEFAULT_TTL = 7 * 24 * time.Hour

// Expire rejects every pending candidate created before cutoff and returns how
// many were expired. The records stay in the file as history.
func (s *Store) Expire(cutoff time.Time) (int, error) {
	pending, err := s.List(STATUS_PENDING)
	if err != nil {
		return 0, err
	}
	expired := 0
	for _, record := range pending {
		if !record.CreatedAt.Before(cutoff) {
			continue
		}
		if _, err := s.Resolve(record.ID, STATUS_REJECTED, "expired"); err != nil {
			return expired, err
		}
		expired++
	}
	return expired, nil
}

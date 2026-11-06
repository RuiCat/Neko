// persist.go loads and atomically saves <dataDir>/blackboard.json.
package eventboard

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
)

// FILE_NAME is the board file under dataDir; a missing file means no boards.
const FILE_NAME = "blackboard.json"

// boardFile is the persisted container. Boards are written sorted by entity so
// the file stays stable and diff-friendly across ticks.
type boardFile struct {
	Boards []*Board `json:"boards"`
}

// loadBoards reads the board file. A missing file is an empty board set; any
// other read or parse problem is returned so the caller can log and continue
// with an empty registry.
func loadBoards(path string) (map[string]*Board, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return map[string]*Board{}, nil
		}
		return nil, fmt.Errorf("读取事件黑板 %s 失败: %w", path, err)
	}
	var file boardFile
	if err := json.Unmarshal(raw, &file); err != nil {
		return nil, fmt.Errorf("解析事件黑板 %s 失败: %w", path, err)
	}
	boards := make(map[string]*Board, len(file.Boards))
	for _, board := range file.Boards {
		if board == nil || board.Entity == "" {
			continue
		}
		if board.Entries == nil {
			board.Entries = []Entry{}
		}
		boards[board.Entity] = board
	}
	return boards, nil
}

// saveBoards writes every board with one atomic replace: a temp file in the
// same directory is fsynced and renamed over the target, so a crash never
// leaves a half-written board file behind.
func saveBoards(path string, boards map[string]*Board) error {
	entities := make([]string, 0, len(boards))
	for entity := range boards {
		entities = append(entities, entity)
	}
	sort.Strings(entities)
	ordered := make([]*Board, 0, len(entities))
	for _, entity := range entities {
		ordered = append(ordered, boards[entity])
	}
	raw, err := json.MarshalIndent(boardFile{Boards: ordered}, "", "  ")
	if err != nil {
		return fmt.Errorf("序列化事件黑板失败: %w", err)
	}
	return writeFileAtomic(path, raw)
}

// writeFileAtomic writes data through a temp file plus rename. It mirrors the
// entity store's atomic write so a board file is never observed half-written.
func writeFileAtomic(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	temp := path + ".tmp"
	file, err := os.OpenFile(temp, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o644)
	if err != nil {
		return err
	}
	if _, err := file.Write(data); err != nil {
		_ = file.Close()
		_ = os.Remove(temp)
		return err
	}
	if err := file.Sync(); err != nil {
		_ = file.Close()
		_ = os.Remove(temp)
		return err
	}
	if err := file.Close(); err != nil {
		_ = os.Remove(temp)
		return err
	}
	if err := os.Rename(temp, path); err != nil {
		_ = os.Remove(temp)
		return err
	}
	return nil
}

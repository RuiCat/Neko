// persist.go loads and atomically saves <dataDir>/session-boards.json (K6). A
// missing file is an empty store; a corrupt one is reported so the caller can
// log it and continue empty (fail-soft).
package sessionboard

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

const (
	// FILE_NAME is the store file under the runtime data directory (K6).
	FILE_NAME = "session-boards.json"
	// SCHEMA_VERSION is the file schema written today (K6).
	SCHEMA_VERSION = 1
)

// storeFile is the persisted container: one global board plus one board per
// workspace key, written sorted by key (K6).
type storeFile struct {
	Schema    int               `json:"schema"`
	UpdatedAt string            `json:"updated_at"`
	Global    Board             `json:"global"`
	Workspace map[string]*Board `json:"workspace"`
}

// storeState is the in-memory shape shared with the store.
type storeState struct {
	Global     Board
	Workspaces map[string]*Board
}

// emptyState returns the empty store state of a fresh data directory (K6).
func emptyState() storeState {
	return storeState{Global: newBoard(SCOPE_GLOBAL, GLOBAL_SCOPE_ID), Workspaces: map[string]*Board{}}
}

// loadStore reads the store file. A missing file is an empty store; any other
// read, parse or schema problem is returned with an empty store so the caller
// keeps serving with empty boards.
func loadStore(path string) (storeState, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return emptyState(), nil
		}
		return emptyState(), fmt.Errorf("读取会话黑板 %s 失败: %w", path, err)
	}
	var file storeFile
	if err := json.Unmarshal(raw, &file); err != nil {
		return emptyState(), fmt.Errorf("解析会话黑板 %s 失败: %w", path, err)
	}
	if file.Schema != SCHEMA_VERSION && file.Schema != 0 {
		return emptyState(), fmt.Errorf("会话黑板 %s 的 schema=%d 不受支持", path, file.Schema)
	}
	state := emptyState()
	state.Global = normalizeBoard(file.Global, SCOPE_GLOBAL, GLOBAL_SCOPE_ID)
	for key, board := range file.Workspace {
		if key == "" || board == nil {
			continue
		}
		normalized := normalizeBoard(*board, SCOPE_WORKSPACE, key)
		state.Workspaces[key] = &normalized
	}
	return state, nil
}

// encodeState serializes the whole store for an atomic replace (K6). Workspace
// keys are sorted by encoding/json, so the file stays stable across writes.
func encodeState(global Board, workspaces map[string]*Board, at time.Time) ([]byte, error) {
	if workspaces == nil {
		workspaces = map[string]*Board{}
	}
	file := storeFile{Schema: SCHEMA_VERSION, UpdatedAt: at.Format(time.RFC3339),
		Global: global, Workspace: workspaces}
	raw, err := json.MarshalIndent(file, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("序列化会话黑板失败: %w", err)
	}
	return raw, nil
}

// normalizeBoard repairs a loaded board: its scope identity is fixed by the
// file position, slices never stay nil and the retained-version watermark is
// recomputed from the retained entries (K1).
func normalizeBoard(board Board, scope, scopeID string) Board {
	board.Scope = scope
	board.ScopeID = scopeID
	if board.Regions == nil {
		board.Regions = []Region{}
	}
	for index := range board.Regions {
		if board.Regions[index].Entries == nil {
			board.Regions[index].Entries = []Entry{}
		}
	}
	board.refreshMinRetained()
	return board
}

// writeFileAtomic replaces path with data through a temp file in the same
// directory, fsync and rename, so no reader ever sees a half-written store
// (K6; same pattern as the event board).
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

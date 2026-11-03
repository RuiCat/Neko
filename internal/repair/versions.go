// versions.go stores the applied repair versions so a passing gate can promote
// them and a failing one can roll them back.
package repair

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// Version is one applied repair batch.
type Version struct {
	Version    string    `json:"version"`
	Hash       string    `json:"hash"`
	Applied    []string  `json:"applied"`
	Count      int       `json:"count"`
	AppliedAt  time.Time `json:"applied_at"`
	RolledBack bool      `json:"rolled_back"`
}

// Store persists the version history as one JSON file.
type Store struct {
	path string
}

// NewStore returns a store writing to path.
func NewStore(path string) *Store { return &Store{path: path} }

// Load returns every recorded version, oldest first.
func (s *Store) Load() ([]Version, error) {
	raw, err := os.ReadFile(s.path)
	if err != nil {
		if os.IsNotExist(err) {
			return []Version{}, nil
		}
		return nil, fmt.Errorf("repair: 读取版本文件失败: %w", err)
	}
	versions := []Version{}
	if err := json.Unmarshal(raw, &versions); err != nil {
		return nil, fmt.Errorf("repair: 解析版本文件失败: %w", err)
	}
	return versions, nil
}

// save writes the whole history atomically.
func (s *Store) save(versions []Version) error {
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return fmt.Errorf("repair: 创建目录失败: %w", err)
	}
	raw, err := json.MarshalIndent(versions, "", "  ")
	if err != nil {
		return fmt.Errorf("repair: 序列化版本失败: %w", err)
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, raw, 0o644); err != nil {
		return fmt.Errorf("repair: 写临时文件失败: %w", err)
	}
	if err := os.Rename(tmp, s.path); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("repair: 替换版本文件失败: %w", err)
	}
	return nil
}

// Apply promotes one batch as a new version. An empty batch is a no-op.
func (s *Store) Apply(candidates []Candidate, now time.Time) (Version, error) {
	if len(candidates) == 0 {
		return Version{}, nil
	}
	versions, err := s.Load()
	if err != nil {
		return Version{}, err
	}
	hasher := sha256.New()
	applied := make([]string, 0, len(candidates))
	for _, candidate := range candidates {
		hasher.Write([]byte(candidate.Hash))
		applied = append(applied, candidate.ID)
	}
	version := Version{
		Version:   fmt.Sprintf("v%d", len(versions)+1),
		Hash:      hex.EncodeToString(hasher.Sum(nil)),
		Applied:   applied,
		Count:     len(candidates),
		AppliedAt: now,
	}
	if err := s.save(append(versions, version)); err != nil {
		return Version{}, err
	}
	return version, nil
}

// Rollback marks the newest live version as rolled back and returns it.
func (s *Store) Rollback() (Version, error) {
	versions, err := s.Load()
	if err != nil {
		return Version{}, err
	}
	for index := len(versions) - 1; index >= 0; index-- {
		if versions[index].RolledBack {
			continue
		}
		versions[index].RolledBack = true
		if err := s.save(versions); err != nil {
			return Version{}, err
		}
		return versions[index], nil
	}
	return Version{}, fmt.Errorf("repair: 没有可回滚的版本")
}

// entityOps.go hosts the gated entity writes and deletes shared by the MCP
// tools and the control plane.
package mcpserver

import (
	"errors"
	"fmt"
	"log"
	"sort"
	"strings"
	"time"

	"github.com/RuiCat/Neko/internal/eventboard"
	"github.com/RuiCat/Neko/internal/hardstate"
)

// ErrEntityMissing marks a write or delete against an unknown entity.
var ErrEntityMissing = errors.New("实体不存在")

// ErrPathInvalid marks a rejected dotted path write.
var ErrPathInvalid = errors.New("路径无效")

// setValues validates the single/batch forms of entity_state_set and decodes
// them into dotted path -> value. The historical value string keeps its
// JSON-or-text parsing; batch string values use the same rule.
func (a entitySetArgs) setValues() (map[string]any, error) {
	if len(a.Values) > 0 && (a.Path != "" || a.Value != "") {
		return nil, fmt.Errorf("path/value 与 values 不能同时使用")
	}
	if len(a.Values) == 0 {
		if strings.TrimSpace(a.Path) == "" {
			return nil, fmt.Errorf("缺少 path/value 或 values")
		}
		return map[string]any{a.Path: parseValue(a.Value)}, nil
	}
	values := make(map[string]any, len(a.Values))
	for path, raw := range a.Values {
		if strings.TrimSpace(path) == "" {
			return nil, fmt.Errorf("values 的路径不能为空")
		}
		value := raw
		if text, ok := raw.(string); ok {
			value = parseValue(text)
		}
		values[path] = value
	}
	return values, nil
}

// WriteEntityPaths applies dotted path values to one entity under the shared
// state gate, warns about hard-state writes and saves the store. It returns the
// written paths in ascending order; callers must hold no state gate.
func (s *Server) WriteEntityPaths(entityID string, values map[string]any, source string) ([]string, error) {
	paths := make([]string, 0, len(values))
	for path := range values {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	if len(paths) == 0 {
		return nil, fmt.Errorf("%w: 没有要写入的路径", ErrPathInvalid)
	}
	changes := make([]eventboard.Change, 0, len(paths))
	err := s.WithStateWrite(func() error {
		ent := s.store.Get(entityID)
		if ent == nil {
			return fmt.Errorf("%w: '%s'", ErrEntityMissing, entityID)
		}
		for _, path := range paths {
			if check := hardstate.CheckWrite(s.hardStateSchema, entityID, path, source); check.HardState && !check.Allowed {
				log.Printf("astrbox: 硬状态写入告警: %s", check.Reason)
			}
			from, _ := ent.Get(path)
			if err := ent.Set(path, values[path]); err != nil {
				return fmt.Errorf("%w: %s: %s", ErrPathInvalid, path, err)
			}
			changes = append(changes, eventboard.Change{
				Path: path, From: from, To: values[path], FromKnown: true,
			})
		}
		if err := s.store.Save(); err != nil {
			return fmt.Errorf("保存失败: %w", err)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	// K1 扩展：直接写入也进入事件黑板，订阅推送才看得到场景/实体内容变化。
	// 黑板记录失败只记日志，不让一次成功的状态写入变成失败。
	if s.eventBoards != nil {
		if recordErr := s.eventBoards.Record(entityID, changes, "direct:"+source, time.Now()); recordErr != nil {
			log.Printf("astrbox: 直接写入记录黑板失败: %v", recordErr)
		}
	}
	return paths, nil
}

// DeleteEntity removes one entity, its change board and saves the store. It
// reports ErrEntityMissing for an unknown entity.
func (s *Server) DeleteEntity(entityID string) error {
	return s.WithStateWrite(func() error {
		if !s.store.Delete(entityID) {
			return fmt.Errorf("%w: '%s'", ErrEntityMissing, entityID)
		}
		if s.eventBoards != nil {
			if err := s.eventBoards.Delete(entityID); err != nil {
				return fmt.Errorf("删除事件黑板失败: %w", err)
			}
		}
		if err := s.store.Save(); err != nil {
			return fmt.Errorf("保存失败: %w", err)
		}
		return nil
	})
}

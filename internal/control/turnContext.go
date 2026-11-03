// turnContext.go serves the current compensation package to the DSH bridge
// plugin. The snapshot here is a projection for prompt injection (ctx_ prefix)
// and is deliberately not journaled: only ticks and DSH turns produce records.
package control

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/RuiCat/Neko/internal/compensation"
	"github.com/RuiCat/Neko/internal/gate"
	"github.com/RuiCat/Neko/internal/hardstate"
	"github.com/RuiCat/Neko/internal/journal"
)

// handleTurnContext builds one fresh package from the current entity state.
func (s *Server) handleTurnContext(r *http.Request) response {
	entityID := strings.TrimSpace(r.URL.Query().Get("entity"))
	if entityID == "" {
		return badRequest("缺少 entity 参数")
	}
	if s.runtime.Store.Get(entityID) == nil {
		return notFound(fmt.Sprintf("实体 '%s' 不存在", entityID))
	}
	now := time.Now()
	snapshotID := fmt.Sprintf("ctx_%d", now.UnixNano())
	snapshot, pkg, active, err := s.buildTurnContext(r.Context(), snapshotID, entityID, now)
	if err != nil {
		return internalError(err.Error())
	}
	return response{status: http.StatusOK, body: map[string]any{
		"ok":           true,
		"turn_id":      snapshotID,
		"entity":       entityID,
		"snapshot":     snapshot,
		"gate":         map[string]any{"active": active, "version": gate.RULE_GATE_VERSION},
		"compensation": pkg,
	}}
}

// buildTurnContext captures one entity's hard state and assembles a package.
// The caller owns the snapshot ID: ctx_ for prompt projections, evt_ for
// journaled DSH turns.
func (s *Server) buildTurnContext(ctx context.Context, snapshotID, entityID string, now time.Time) (hardstate.Snapshot, journal.Compensation, []string, error) {
	if s.runtime.Assembler == nil || s.runtime.Board == nil {
		return hardstate.Snapshot{}, journal.Compensation{}, nil, fmt.Errorf("补偿包组装器未配置")
	}
	ent := s.runtime.Store.Get(entityID)
	if ent == nil {
		return hardstate.Snapshot{}, journal.Compensation{}, nil, fmt.Errorf("实体 '%s' 不存在", entityID)
	}
	snapshot, err := hardstate.Capture(
		map[string]map[string]any{entityID: ent.Snapshot()},
		s.runtime.HardState,
		hardstate.IDs{SnapshotID: snapshotID, TurnID: snapshotID},
		now,
	)
	if err != nil {
		return hardstate.Snapshot{}, journal.Compensation{}, nil, fmt.Errorf("生成快照失败: %w", err)
	}
	var pkg journal.Compensation
	var active []string
	writeErr := s.mcp.WithStateWrite(func() error {
		if err := s.runtime.Board.SetSnapshot(snapshot); err != nil {
			return fmt.Errorf("写黑板失败: %w", err)
		}
		var assembleErr error
		pkg, active, assembleErr = s.runtime.Assembler.AssembleWithActive(ctx, compensation.Input{
			Board:    s.runtime.Board,
			Features: gate.Features{EventKind: "turn"},
		})
		if assembleErr != nil {
			return fmt.Errorf("组装补偿包失败: %w", assembleErr)
		}
		return nil
	})
	if writeErr != nil {
		return hardstate.Snapshot{}, journal.Compensation{}, nil, writeErr
	}
	return snapshot, pkg, active, nil
}

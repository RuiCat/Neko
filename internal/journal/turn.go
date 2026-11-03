// turn.go builds one TurnRecord from a finished state-machine tick. It is the
// only place where a snapshot and a tick are joined; callers just append.
package journal

import (
	"fmt"
	"time"

	"github.com/RuiCat/Neko/internal/engine"
	"github.com/RuiCat/Neko/internal/entity"
	"github.com/RuiCat/Neko/internal/hardstate"
)

// TurnOptions carries everything a turn record needs. Sequence is required;
// Entities is usually store.ListAll().
type TurnOptions struct {
	Entities  []*entity.Entity
	Schema    hardstate.Schema
	Sequence  *hardstate.Sequence
	SessionID string
	Now       time.Time
}

// RecordTurn captures the hard state for one turn and returns an incomplete
// record (no answer yet). The snapshot ID and the turn ID are the same value in
// P1, so a record can always be located by either.
func RecordTurn(options TurnOptions, result engine.TickResult) (TurnRecord, error) {
	if options.Sequence == nil {
		return TurnRecord{}, fmt.Errorf("journal: 缺少序列发生器")
	}
	if options.Now.IsZero() {
		return TurnRecord{}, fmt.Errorf("journal: 缺少时间")
	}
	turnID := options.Sequence.Next(options.Now)
	entityData := make(map[string]map[string]any, len(options.Entities))
	for _, ent := range options.Entities {
		if ent == nil {
			continue
		}
		entityData[ent.ID] = ent.Snapshot()
	}
	snapshot, err := hardstate.Capture(entityData, options.Schema, hardstate.IDs{
		SnapshotID: turnID, TurnID: turnID, TickID: result.TickID,
	}, options.Now)
	if err != nil {
		return TurnRecord{}, err
	}
	record := NewTurnRecord(turnID, snapshot, options.SessionID, options.Now)
	record.Source = SOURCE_TICK
	return record, nil
}

// NewTurnRecord builds an incomplete record from an existing snapshot. Chat
// turns use it after they have taken their own snapshot; the tick path gets it
// through RecordTurn.
func NewTurnRecord(turnID string, snapshot hardstate.Snapshot, sessionID string, now time.Time) TurnRecord {
	return TurnRecord{
		RecordVersion: RECORD_VERSION,
		TurnID:        turnID,
		SessionID:     sessionID,
		Status:        STATUS_INCOMPLETE,
		StartedAt:     now,
		Snapshot:      snapshot,
		Feedback:      Feedback{Kind: "none"},
	}
}

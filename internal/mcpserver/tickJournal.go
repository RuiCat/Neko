// tickJournal.go joins one finished tick to the journal: it captures the hard
// state, builds a TurnRecord, attaches the dry-run compensation package and
// queues it without blocking the tick gate.
package mcpserver

import (
	"context"
	"errors"
	"log"
	"time"

	"github.com/RuiCat/Neko/internal/compensation"
	"github.com/RuiCat/Neko/internal/engine"
	"github.com/RuiCat/Neko/internal/gate"
	"github.com/RuiCat/Neko/internal/journal"
)

// appendTurnRecord writes one journal record when journaling is wired. A full
// queue is expected under load and only counted by the writer; other errors
// are logged because they mean a record was lost.
func (s *Server) appendTurnRecord(result engine.TickResult) {
	if s.journal == nil || s.journalSequence == nil {
		return
	}
	record, err := journal.RecordTurn(journal.TurnOptions{
		Entities:  s.store.ListAll(),
		Schema:    s.hardStateSchema,
		Sequence:  s.journalSequence,
		SessionID: s.currentSessionID(),
		Now:       time.Now(),
	}, result)
	if err != nil {
		log.Printf("astrbox: 生成交互记录失败: %v", err)
		return
	}
	s.attachCompensation(&record, result)
	if err := s.journal.Append(record); err != nil && !errors.Is(err, journal.ErrQueueFull) {
		log.Printf("astrbox: 交互记录入队失败: %v", err)
	}
}

// attachCompensation runs the gate and assembler for this turn and records the
// package. It never calls a remote model: the dry-run validates the data path
// so the metrics have real samples before the LLM adapter lands.
func (s *Server) attachCompensation(record *journal.TurnRecord, result engine.TickResult) {
	if s.board == nil || s.assembler == nil {
		return
	}
	if err := s.board.SetSnapshot(record.Snapshot); err != nil {
		log.Printf("astrbox: 黑板写入快照失败: %v", err)
		return
	}
	pkg, active, err := s.assembler.AssembleWithActive(context.Background(), compensation.Input{
		UserInput: record.UserInput,
		Board:     s.board,
		Features:  gate.Features{EventKind: eventKindOf(result), Summary: result.ReplyHint},
	})
	if err != nil {
		log.Printf("astrbox: 补偿包组装失败: %v", err)
		return
	}
	record.Compensation = pkg
	record.Gate = journal.Gate{Active: active, Version: gate.RULE_GATE_VERSION}
}

// eventKindOf maps a tick outcome onto the gate's event vocabulary.
func eventKindOf(result engine.TickResult) string {
	if len(result.Conflicts) > 0 {
		return "conflict"
	}
	return "turn"
}

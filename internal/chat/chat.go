// Package chat is the standalone conversation loop: user input -> state
// machine tick (caller) -> compensation package -> remote model -> complete
// TurnRecord. It never writes hard state; the tick does that before it runs.
package chat

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/RuiCat/Neko/internal/blackboard"
	"github.com/RuiCat/Neko/internal/compensation"
	"github.com/RuiCat/Neko/internal/entity"
	"github.com/RuiCat/Neko/internal/gate"
	"github.com/RuiCat/Neko/internal/hardstate"
	"github.com/RuiCat/Neko/internal/journal"
	"github.com/RuiCat/Neko/internal/llm"
)

// DEFAULT_MAX_RECENT is how many chat messages (user+assistant) are replayed
// into the prompt. Compensation carries the long-term memory instead.
const DEFAULT_MAX_RECENT = 6

// Options wires one chat loop.
type Options struct {
	Store     *entity.Store
	Schema    hardstate.Schema
	Sequence  *hardstate.Sequence
	Board     *blackboard.Board
	Assembler *compensation.Assembler
	Journal   *journal.Writer
	Provider  llm.Provider
	Persona   string
	SessionID string
	MaxRecent int
	DryRun    bool
}

// Chat owns the per-conversation recent history and the persona prompt.
type Chat struct {
	options Options
	recent  []llm.Message
}

// Result is one completed turn.
type Result struct {
	TurnID    string
	Answer    string
	Package   journal.Compensation
	Active    []string
	Record    journal.TurnRecord
	LatencyMS int64
}

// New validates and builds a chat loop. A missing provider is allowed only in
// dry-run mode; otherwise it is a configuration error.
func New(options Options) (*Chat, error) {
	if options.Store == nil || options.Sequence == nil || options.Board == nil || options.Assembler == nil || options.Journal == nil {
		return nil, fmt.Errorf("chat: store/sequence/board/assembler/journal 均为必填")
	}
	if options.Provider == nil && !options.DryRun {
		return nil, fmt.Errorf("chat: 缺少 llm.Provider（dry-run 模式除外）")
	}
	if options.MaxRecent <= 0 {
		options.MaxRecent = DEFAULT_MAX_RECENT
	}
	if strings.TrimSpace(options.Persona) == "" {
		options.Persona = DEFAULT_PERSONA
	}
	if strings.TrimSpace(options.SessionID) == "" {
		options.SessionID = "neko-chat"
	}
	return &Chat{options: options}, nil
}

// Turn runs one conversation turn. tickID comes from the tick the caller ran
// just before this call; it is only used for traceability in the snapshot.
func (c *Chat) Turn(ctx context.Context, input, tickID string) (Result, error) {
	started := time.Now()
	turnID := c.options.Sequence.Next(started)
	entityData := make(map[string]map[string]any, len(c.options.Store.ListAll()))
	for _, ent := range c.options.Store.ListAll() {
		entityData[ent.ID] = ent.Snapshot()
	}
	snapshot, err := hardstate.Capture(entityData, c.options.Schema, hardstate.IDs{
		SnapshotID: turnID, TurnID: turnID, TickID: tickID,
	}, started)
	if err != nil {
		return Result{}, err
	}
	if err := c.options.Board.SetSnapshot(snapshot); err != nil {
		return Result{}, err
	}
	if err := c.options.Board.SetEvent(blackboard.Event{
		ID: turnID, Kind: "turn", Summary: summarize(input),
	}); err != nil {
		return Result{}, err
	}
	pkg, active, err := c.options.Assembler.AssembleWithActive(ctx, compensation.Input{
		UserInput: input,
		Board:     c.options.Board,
		Features:  gate.Features{EventKind: "turn", Summary: summarize(input)},
	})
	if err != nil {
		return Result{}, err
	}
	record := journal.NewTurnRecord(turnID, snapshot, c.options.SessionID, started)
	record.Source = journal.SOURCE_CHAT
	record.UserInput = input
	record.Compensation = pkg
	record.Gate = journal.Gate{Active: active, Version: gate.RULE_GATE_VERSION}
	record.EndedAt = &started

	result := Result{TurnID: turnID, Package: pkg, Active: active}
	if c.options.DryRun {
		record.Status = journal.STATUS_INCOMPLETE
		record.LatencyMS = time.Since(started).Milliseconds()
		record.Answer = ""
		result.Record = record
		return result, c.appendRecord(record)
	}
	response, callErr := c.options.Provider.Complete(ctx, llm.Request{
		Model:       "",
		Messages:    c.buildMessages(pkg, input),
		Temperature: 0.8,
		MaxTokens:   512,
	})
	ended := time.Now()
	record.EndedAt = &ended
	record.LatencyMS = ended.Sub(started).Milliseconds()
	if callErr != nil {
		record.Status = journal.STATUS_ERROR
		record.Answer = ""
		result.Record = record
		if err := c.appendRecord(record); err != nil {
			log.Printf("chat: 错误路径 journal 追加失败: %v", err)
		}
		return result, callErr
	}
	record.Status = journal.STATUS_COMPLETE
	record.Answer = response.Content
	record.AnswerModel = response.Model
	record.Cost = journal.Cost{PromptTokens: response.Usage.PromptTokens, CompletionTokens: response.Usage.CompletionTokens}
	result.Answer = response.Content
	result.Record = record
	c.remember(llm.Message{Role: "user", Content: input})
	c.remember(llm.Message{Role: "assistant", Content: response.Content})
	return result, c.appendRecord(record)
}

// buildMessages assembles the frozen prompt order: persona, compensation
// package, recent dialogue, current input.
func (c *Chat) buildMessages(pkg journal.Compensation, input string) []llm.Message {
	raw, err := json.Marshal(pkg)
	if err != nil {
		raw = []byte("{}")
	}
	messages := []llm.Message{
		{Role: "system", Content: c.options.Persona},
		{Role: "system", Content: "人格补偿包（状态数值只能引用，不能改写）：\n" + string(raw)},
	}
	messages = append(messages, c.recent...)
	messages = append(messages, llm.Message{Role: "user", Content: input})
	return messages
}

// remember appends to the bounded recent history.
func (c *Chat) remember(message llm.Message) {
	c.recent = append(c.recent, message)
	if len(c.recent) > c.options.MaxRecent {
		c.recent = c.recent[len(c.recent)-c.options.MaxRecent:]
	}
}

// Recent returns a copy of the replayed history (tests and diagnostics).
func (c *Chat) Recent() []llm.Message {
	return append([]llm.Message(nil), c.recent...)
}

// appendRecord queues the turn record; a full queue is surfaced as an error so
// the caller knows the turn is not fully durable.
func (c *Chat) appendRecord(record journal.TurnRecord) error {
	return c.options.Journal.Append(record)
}

// summarize keeps the board event short; the full input lives in the record.
func summarize(input string) string {
	trimmed := strings.TrimSpace(input)
	rs := []rune(trimmed)
	if len(rs) > 120 {
		return string(rs[:120]) + "…"
	}
	return trimmed
}

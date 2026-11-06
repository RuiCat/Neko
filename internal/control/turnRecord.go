// turnRecord.go writes a complete TurnRecord when the DSH bridge reports the
// end of a turn. The answer comes from the bridge's assistant digest; the
// snapshot and package are built after any .neko callback has run.
package control

import (
	"context"
	"log"
	"strings"
	"time"

	"github.com/RuiCat/Neko/internal/expert"
	"github.com/RuiCat/Neko/internal/gate"
	"github.com/RuiCat/Neko/internal/journal"
)

// recordDSHTurn persists one complete DSH turn for the effective entity the
// request was applied to (the server default when the request carried none). It
// is a no-op for other event kinds, turns without an assistant digest, or
// runtimes without a journal.
func (s *Server) recordDSHTurn(request sessionEventRequest, entityID string) {
	if request.Kind != KIND_TURN_END || request.Summary == nil {
		return
	}
	answer := strings.TrimSpace(request.Summary.AssistantDigest)
	if answer == "" {
		return
	}
	if s.runtime.Journal == nil || s.runtime.JournalSequence == nil {
		return
	}
	now := time.Now()
	turnID := s.runtime.JournalSequence.Next(now)
	snapshot, pkg, active, err := s.buildTurnContext(context.Background(), turnID, entityID, now)
	if err != nil {
		log.Printf("astrbox: DSH 轮记录失败: %v", err)
		return
	}
	record := journal.NewTurnRecord(turnID, snapshot, request.SessionID, now)
	record.Source = journal.SOURCE_DSH_TURN
	record.Status = journal.STATUS_COMPLETE
	record.UserInput = strings.TrimSpace(request.Summary.UserDigest)
	record.Answer = answer
	record.Compensation = pkg
	record.Gate = journal.Gate{Active: active, Version: gate.RULE_GATE_VERSION}
	record.EndedAt = &now
	if err := s.runtime.Journal.Append(record); err != nil {
		log.Printf("astrbox: DSH 轮记录入队失败: %v", err)
	}
	s.rememberTurn(turnID, record.UserInput, answer, now)
	if err := s.runtime.SaveExperts(); err != nil {
		log.Printf("astrbox: 保存专家上下文失败: %v", err)
	}
}

// rememberTurn feeds the turn into the experts' private contexts so later
// compensation packages can recall it. The heuristic routing is deliberately
// simple; it can be replaced by a trained classifier behind the same API.
func (s *Server) rememberTurn(turnID, userInput, answer string, now time.Time) {
	if s.runtime.Experts == nil {
		return
	}
	content := "助手: " + truncateRunes(answer, 160)
	if userInput != "" {
		content = "用户: " + truncateRunes(userInput, 120) + " | " + content
	}
	entry := expert.Entry{
		SnapshotID: turnID,
		Content:    content,
		Weight:     0.7,
		At:         now.Unix(),
	}
	for _, id := range memoryTargets(userInput) {
		s.runtime.Experts.Remember(id, entry)
	}
}

// memoryTargets maps one turn onto the experts that should remember it.
func memoryTargets(userInput string) []string {
	targets := []string{gate.EXPERT_EVENT, gate.EXPERT_EXPRESSION}
	if strings.ContainsAny(userInput, "？?") {
		targets = append(targets, gate.EXPERT_KNOWLEDGE)
	}
	if containsAny(userInput, relationshipWords) {
		targets = append(targets, gate.EXPERT_RELATION)
	}
	if containsAny(userInput, selfWords) {
		targets = append(targets, gate.EXPERT_SELF)
	}
	return targets
}

var relationshipWords = []string{"生气", "对不起", "道歉", "喜欢", "讨厌", "信任", "答应", "约定"}

var selfWords = []string{"难过", "开心", "累", "害怕", "心情", "孤独"}

func containsAny(text string, words []string) bool {
	for _, word := range words {
		if strings.Contains(text, word) {
			return true
		}
	}
	return false
}

// truncateRunes keeps one content field bounded without cutting a rune.
func truncateRunes(text string, limit int) string {
	runes := []rune(text)
	if len(runes) <= limit {
		return text
	}
	return string(runes[:limit]) + "…"
}

// Package diagnostics keeps a bounded, deduplicated ring of engine warnings,
// errors and consistency findings so MCP clients can inspect engine health.
package diagnostics

import (
	"crypto/sha1"
	"encoding/hex"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

// Severity ranks one diagnostic record.
type Severity string

// Severity levels, ordered from most to least attention.
const (
	SEVERITY_ERROR   Severity = "error"
	SEVERITY_WARNING Severity = "warning"
	SEVERITY_INFO    Severity = "info"
)

// maxMessageRunes bounds one diagnostic message so a state query stays small.
const MAX_MESSAGE_RUNES = 200

// Record is one deduplicated diagnostic entry.
type Record struct {
	ID         string    `json:"id"`
	Code       string    `json:"code"`
	Severity   Severity  `json:"severity"`
	Source     string    `json:"source,omitempty"`
	Script     string    `json:"script,omitempty"`
	Entity     string    `json:"entity,omitempty"`
	Type       string    `json:"type,omitempty"`
	Line       int       `json:"line,omitempty"`
	Message    string    `json:"message"`
	Hint       string    `json:"hint,omitempty"`
	Count      int       `json:"count"`
	FirstSeen  time.Time `json:"first_seen"`
	LastSeen   time.Time `json:"last_seen"`
	Generation uint64    `json:"generation"`
}

// fingerprint identifies one logical diagnostic across repeated occurrences.
func (r Record) fingerprint() string {
	parts := []string{
		r.Code, r.Source, r.Script, r.Entity, r.Type,
		strconv.Itoa(r.Line), r.Message,
	}
	sum := sha1.Sum([]byte(strings.Join(parts, "\x00")))
	return hex.EncodeToString(sum[:])
}

// normalize fills defaults and bounds the message length.
func (r Record) normalize() Record {
	if r.Severity == "" {
		r.Severity = SEVERITY_ERROR
	}
	r.Message = truncate(r.Message, MAX_MESSAGE_RUNES)
	r.Hint = truncate(r.Hint, MAX_MESSAGE_RUNES)
	if r.Code == "" {
		r.Code = "engine.unknown"
	}
	return r
}

// truncate shortens s to at most limit runes.
func truncate(s string, limit int) string {
	if utf8.RuneCountInString(s) <= limit {
		return s
	}
	runes := []rune(s)
	return string(runes[:limit]) + "…"
}

// severityRank sorts errors before warnings before info.
func severityRank(s Severity) int {
	switch s {
	case SEVERITY_ERROR:
		return 0
	case SEVERITY_WARNING:
		return 1
	default:
		return 2
	}
}

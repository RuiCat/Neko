// Daily write-audit log under <data-dir>/logs, pruned to seven days; logging
// failures never fail the request and token values are never written.
package auth

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const (
	// AUDIT_PREFIX names one audit file per day.
	AUDIT_PREFIX = "monitor-audit-"
	// AUDIT_SUFFIX is the audit file extension.
	AUDIT_SUFFIX = ".log"
	// AUDIT_DATE_FORMAT is the compact date used in the file name.
	AUDIT_DATE_FORMAT = "20060102"
	// AUDIT_KEEP_DAYS is how many days of audit files are retained.
	AUDIT_KEEP_DAYS = 7
	// AUDIT_MAX_LINE bounds one audit line in bytes.
	AUDIT_MAX_LINE = 1024
	// AUDIT_MAX_FIELD bounds one free-text field in bytes.
	AUDIT_MAX_FIELD = 200
)

// Audit appends one line per write operation to data/logs/monitor-audit-*.log.
type Audit struct {
	mu       sync.Mutex
	dir      string
	keepDays int
}

// NewAudit builds the audit logger; a non-positive keepDays falls back to 7.
func NewAudit(dir string, keepDays int) *Audit {
	if keepDays <= 0 {
		keepDays = AUDIT_KEEP_DAYS
	}
	return &Audit{dir: dir, keepDays: keepDays}
}

// Record appends one audit line. The line never contains a token: only the
// action, entity, path, result and remote address are written.
func (a *Audit) Record(action, entity, path, result, remote string) {
	if a == nil {
		return
	}
	line := fmt.Sprintf("action=%s entity=%s path=%s result=%s remote=%s",
		sanitize(action, AUDIT_MAX_FIELD),
		sanitize(entity, AUDIT_MAX_FIELD),
		sanitize(path, AUDIT_MAX_FIELD),
		sanitize(result, AUDIT_MAX_FIELD),
		sanitize(remote, AUDIT_MAX_FIELD),
	)
	if len(line) > AUDIT_MAX_LINE {
		line = line[:AUDIT_MAX_LINE]
	}
	now := time.Now()
	a.mu.Lock()
	defer a.mu.Unlock()
	if err := os.MkdirAll(a.dir, 0o755); err != nil {
		log.Printf("monitor: 审计目录不可写: %v", err)
		return
	}
	pathName := filepath.Join(a.dir, AUDIT_PREFIX+now.Format(AUDIT_DATE_FORMAT)+AUDIT_SUFFIX)
	file, err := os.OpenFile(pathName, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		log.Printf("monitor: 审计写入失败: %v", err)
		return
	}
	_, err = file.WriteString(now.Format(time.RFC3339) + " " + line + "\n")
	_ = file.Close()
	if err != nil {
		log.Printf("monitor: 审计写入失败: %v", err)
		return
	}
	a.pruneLocked(now)
}

// sanitize flattens control characters and bounds one audit field.
func sanitize(value string, limit int) string {
	value = strings.Map(func(r rune) rune {
		if r == '\n' || r == '\r' || r == '\t' {
			return ' '
		}
		return r
	}, value)
	if len(value) > limit {
		return value[:limit]
	}
	return value
}

// pruneLocked drops audit files older than keepDays.
func (a *Audit) pruneLocked(now time.Time) {
	entries, err := os.ReadDir(a.dir)
	if err != nil {
		return
	}
	cutoff := now.AddDate(0, 0, -a.keepDays)
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasPrefix(name, AUDIT_PREFIX) || !strings.HasSuffix(name, AUDIT_SUFFIX) {
			continue
		}
		datePart := strings.TrimSuffix(strings.TrimPrefix(name, AUDIT_PREFIX), AUDIT_SUFFIX)
		date, err := time.Parse(AUDIT_DATE_FORMAT, datePart)
		if err != nil || !date.Before(cutoff) {
			continue
		}
		_ = os.Remove(filepath.Join(a.dir, name))
	}
}

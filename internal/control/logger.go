// Daily control-plane request log under <data-dir>/logs, pruned to 7 days.
package control

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const (
	// CONTROL_LOG_PREFIX is the daily control log file prefix.
	CONTROL_LOG_PREFIX = "dsh-control-"
	// CONTROL_LOG_SUFFIX is the daily control log file suffix.
	CONTROL_LOG_SUFFIX = ".log"
	// CONTROL_LOG_KEEP_DAYS is how many days of control logs are kept.
	CONTROL_LOG_KEEP_DAYS = 7
	// CONTROL_LOG_DATE_FORMAT names one control log file per day.
	CONTROL_LOG_DATE_FORMAT = "20060102"
)

// dailyLogger appends access lines to data/logs/dsh-control-YYYYMMDD.log and
// prunes files older than keepDays.
type dailyLogger struct {
	mu       sync.Mutex
	dir      string
	keepDays int
}

// newDailyLogger builds a logger; non-positive keepDays falls back to 7.
func newDailyLogger(dir string, keepDays int) *dailyLogger {
	if keepDays <= 0 {
		keepDays = CONTROL_LOG_KEEP_DAYS
	}
	return &dailyLogger{dir: dir, keepDays: keepDays}
}

// append writes one line to today's file. Logging never fails a request.
func (l *dailyLogger) append(line string) {
	if l == nil {
		return
	}
	now := time.Now()
	l.mu.Lock()
	defer l.mu.Unlock()
	if err := os.MkdirAll(l.dir, 0o755); err != nil {
		return
	}
	path := filepath.Join(l.dir, CONTROL_LOG_PREFIX+now.Format(CONTROL_LOG_DATE_FORMAT)+CONTROL_LOG_SUFFIX)
	file, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return
	}
	_, _ = file.WriteString(now.Format(time.RFC3339) + " " + line + "\n")
	_ = file.Close()
	l.pruneLocked(now)
}

// pruneLocked removes control logs whose date is older than keepDays.
func (l *dailyLogger) pruneLocked(now time.Time) {
	entries, err := os.ReadDir(l.dir)
	if err != nil {
		return
	}
	cutoff := now.AddDate(0, 0, -l.keepDays)
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasPrefix(name, CONTROL_LOG_PREFIX) || !strings.HasSuffix(name, CONTROL_LOG_SUFFIX) {
			continue
		}
		datePart := strings.TrimSuffix(strings.TrimPrefix(name, CONTROL_LOG_PREFIX), CONTROL_LOG_SUFFIX)
		date, err := time.Parse(CONTROL_LOG_DATE_FORMAT, datePart)
		if err != nil || !date.Before(cutoff) {
			continue
		}
		_ = os.Remove(filepath.Join(l.dir, name))
	}
}

// logRequest records one served request in the daily control log.
func (s *Server) logRequest(r *http.Request, resp response, elapsed time.Duration) {
	line := fmt.Sprintf("%s %s %d %s", r.Method, r.URL.Path, resp.status, elapsed.Round(time.Microsecond))
	if resp.note != "" {
		line += " " + resp.note
	}
	s.log.append(line)
}

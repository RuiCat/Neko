// GET /monitor/api/logs: bounded tail reads of the whitelisted state and
// control daily logs, never following symlinks and never returning half a line.
package api

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const (
	// LOG_KIND_STATE selects data/logs/state_YYYY-MM-DD.log.
	LOG_KIND_STATE = "state"
	// LOG_KIND_CONTROL selects data/logs/dsh-control-YYYYMMDD.log.
	LOG_KIND_CONTROL = "control"
	// DEFAULT_LOG_LIMIT is used when limit is absent.
	DEFAULT_LOG_LIMIT = 200
	// MAX_LOG_LIMIT caps the number of returned lines.
	MAX_LOG_LIMIT = 2000
	// MAX_LOG_TAIL_BYTES bounds how much of a log file is read.
	MAX_LOG_TAIL_BYTES = 1 << 20
	// MAX_LOG_RESPONSE_BYTES bounds the whole lines payload.
	MAX_LOG_RESPONSE_BYTES = 1 << 20

	// LOG_DATE_LAYOUT is the query/state-log date layout.
	LOG_DATE_LAYOUT = "2006-01-02"
	// LOG_CONTROL_DATE_LAYOUT is the control-log file date layout.
	LOG_CONTROL_DATE_LAYOUT = "20060102"
)

// logLine is one complete log line in the response.
type logLine struct {
	TS     string `json:"ts,omitempty"`
	Raw    string `json:"raw"`
	Parsed any    `json:"parsed,omitempty"`
}

// handleLogs answers a bounded tail of one whitelisted log file.
func (h *Handler) handleLogs(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()
	kind := query.Get("kind")
	if kind == "" {
		kind = LOG_KIND_STATE
	}
	if kind != LOG_KIND_STATE && kind != LOG_KIND_CONTROL {
		WriteFailure(w, http.StatusBadRequest, CODE_BAD_REQUEST, "kind 只支持 state 或 control")
		return
	}
	day, err := parseLogDate(query.Get("date"))
	if err != nil {
		WriteFailure(w, http.StatusBadRequest, CODE_BAD_REQUEST, err.Error())
		return
	}
	limit, err := parseLogLimit(query.Get("limit"))
	if err != nil {
		WriteFailure(w, http.StatusBadRequest, CODE_BAD_REQUEST, err.Error())
		return
	}
	lines, truncated := h.readLogTail(kind, day, limit)
	WriteOK(w, map[string]any{
		"kind": kind, "date": day.Format(LOG_DATE_LAYOUT),
		"lines": lines, "truncated": truncated,
	})
}

// parseLogDate validates date first and re-formats it; a bad value is 400.
func parseLogDate(raw string) (time.Time, error) {
	if raw == "" {
		return time.Now(), nil
	}
	day, err := time.Parse(LOG_DATE_LAYOUT, raw)
	if err != nil {
		return time.Time{}, fmt.Errorf("date 必须是 YYYY-MM-DD")
	}
	return day, nil
}

// parseLogLimit validates the optional limit parameter.
func parseLogLimit(raw string) (int, error) {
	if raw == "" {
		return DEFAULT_LOG_LIMIT, nil
	}
	value, err := strconv.Atoi(raw)
	if err != nil || value <= 0 {
		return 0, fmt.Errorf("limit 必须是正整数")
	}
	if value > MAX_LOG_LIMIT {
		value = MAX_LOG_LIMIT
	}
	return value, nil
}

// readLogTail opens the whitelisted file through os.Root (which refuses
// escaping symlinks) and returns at most limit complete lines.
func (h *Handler) readLogTail(kind string, day time.Time, limit int) ([]logLine, bool) {
	empty := make([]logLine, 0)
	logsDir := filepath.Join(h.deps.Runtime.DataDir, "logs")
	name := LOG_KIND_STATE + "_" + day.Format(LOG_DATE_LAYOUT) + ".log"
	if kind == LOG_KIND_CONTROL {
		name = "dsh-control-" + day.Format(LOG_CONTROL_DATE_LAYOUT) + ".log"
	}
	root, err := os.OpenRoot(logsDir)
	if err != nil {
		return empty, false
	}
	defer func() { _ = root.Close() }()
	info, err := root.Lstat(name)
	if err != nil {
		return empty, false
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return empty, false
	}
	file, err := root.Open(name)
	if err != nil {
		return empty, false
	}
	defer func() { _ = file.Close() }()
	rawLines, truncated := tailLines(file, limit)
	lines := make([]logLine, 0, len(rawLines))
	for _, raw := range rawLines {
		lines = append(lines, buildLogLine(kind, raw))
	}
	return clampLogLines(lines, truncated)
}

// tailLines reads a bounded tail and keeps only newline-terminated lines.
func tailLines(file *os.File, limit int) ([]string, bool) {
	info, err := file.Stat()
	if err != nil || info.Size() == 0 {
		return nil, false
	}
	size := info.Size()
	start := size - MAX_LOG_TAIL_BYTES
	if start < 0 {
		start = 0
	}
	if _, err := file.Seek(start, io.SeekStart); err != nil {
		return nil, false
	}
	buffer := make([]byte, size-start)
	if _, err := io.ReadFull(file, buffer); err != nil {
		return nil, false
	}
	chunks := strings.Split(string(buffer), "\n")
	if start > 0 {
		chunks = chunks[1:]
	}
	// The final element is either the empty tail after a newline or an
	// incomplete line; neither may be returned.
	if len(chunks) > 0 {
		chunks = chunks[:len(chunks)-1]
	}
	truncated := start > 0
	if len(chunks) > limit {
		chunks = chunks[len(chunks)-limit:]
		truncated = true
	}
	return chunks, truncated
}

// buildLogLine parses one line; state lines are JSON, control lines are text
// prefixed with an RFC3339 timestamp.
func buildLogLine(kind, raw string) logLine {
	line := logLine{Raw: raw}
	if kind == LOG_KIND_STATE {
		var parsed any
		if err := json.Unmarshal([]byte(raw), &parsed); err == nil {
			line.Parsed = parsed
		}
		return line
	}
	if space := strings.IndexByte(raw, ' '); space > 0 {
		if stamp, err := time.Parse(time.RFC3339, raw[:space]); err == nil {
			line.TS = stamp.Format(time.RFC3339)
		}
	}
	return line
}

// clampLogLines drops oldest lines until the payload fits the response bound.
func clampLogLines(lines []logLine, truncated bool) ([]logLine, bool) {
	total := 2
	start := len(lines)
	for start > 0 {
		raw, err := json.Marshal(lines[start-1])
		if err != nil {
			start--
			continue
		}
		add := len(raw) + 1
		if total+add > MAX_LOG_RESPONSE_BYTES-256 {
			truncated = true
			break
		}
		total += add
		start--
	}
	if start > 0 {
		truncated = true
	}
	return lines[start:], truncated
}

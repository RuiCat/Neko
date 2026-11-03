// writer.go appends turn records to daily JSONL files from a single background
// goroutine, so a slow disk never holds the tick gate.
package journal

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"
)

// ErrQueueFull means the background writer is behind and the record was
// dropped; the tick path must never block on journal IO.
var ErrQueueFull = errors.New("journal: 队列已满，记录已丢弃")

// ErrClosed means the writer has already been closed.
var ErrClosed = errors.New("journal: writer 已关闭")

// DEFAULT_QUEUE_SIZE bounds the pending records held in memory.
const DEFAULT_QUEUE_SIZE = 1024

// Writer serialises records onto disk. Append never blocks on IO; Close drains
// the queue and reports the first write error seen.
type Writer struct {
	dir     string
	queue   chan TurnRecord
	done    chan struct{}
	closeMu sync.RWMutex
	closed  bool
	dropped atomic.Int64
	failed  atomic.Int64
	errMu   sync.Mutex
	lastErr error
}

// NewWriter creates a writer for dir and starts its background goroutine.
func NewWriter(dir string) *Writer {
	writer := &Writer{
		dir:   dir,
		queue: make(chan TurnRecord, DEFAULT_QUEUE_SIZE),
		done:  make(chan struct{}),
	}
	go writer.loop()
	return writer
}

// Append queues one record. It returns ErrQueueFull when the queue is full and
// ErrClosed after Close; callers on the tick path may ignore the error.
func (w *Writer) Append(record TurnRecord) error {
	w.closeMu.RLock()
	defer w.closeMu.RUnlock()
	if w.closed {
		return ErrClosed
	}
	select {
	case w.queue <- record:
		return nil
	default:
		w.dropped.Add(1)
		return ErrQueueFull
	}
}

// Close drains the queue, stops the goroutine and returns the last write error.
func (w *Writer) Close() error {
	w.closeMu.Lock()
	if w.closed {
		w.closeMu.Unlock()
		return w.lastError()
	}
	w.closed = true
	close(w.queue)
	w.closeMu.Unlock()
	<-w.done
	return w.lastError()
}

// Dropped reports how many records were dropped because the queue was full.
func (w *Writer) Dropped() int64 { return w.dropped.Load() }

// Failed reports how many records failed to reach disk.
func (w *Writer) Failed() int64 { return w.failed.Load() }

func (w *Writer) lastError() error {
	w.errMu.Lock()
	defer w.errMu.Unlock()
	return w.lastErr
}

func (w *Writer) loop() {
	defer close(w.done)
	for record := range w.queue {
		if err := w.write(record); err != nil {
			w.failed.Add(1)
			w.errMu.Lock()
			w.lastErr = err
			w.errMu.Unlock()
		}
	}
}

// write appends one record to the file of its local start date.
func (w *Writer) write(record TurnRecord) error {
	raw, err := json.Marshal(record)
	if err != nil {
		return fmt.Errorf("journal: 序列化失败: %w", err)
	}
	if err := os.MkdirAll(w.dir, 0o755); err != nil {
		return fmt.Errorf("journal: 创建目录失败: %w", err)
	}
	path := dailyPath(w.dir, record.StartedAt)
	file, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("journal: 打开 %s 失败: %w", path, err)
	}
	defer file.Close()
	if _, err := file.Write(append(raw, '\n')); err != nil {
		return fmt.Errorf("journal: 写入 %s 失败: %w", path, err)
	}
	return nil
}

// dailyPath is shared by the reader and by tests so the file naming lives in
// exactly one place.
func dailyPath(dir string, day time.Time) string {
	return filepath.Join(dir, day.Format("2006-01-02")+".jsonl")
}

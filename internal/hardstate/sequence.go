// sequence.go issues monotonic snapshot IDs per local day. IDs are
// "evt_<yyyymmdd>_<seq>" with a zero-padded sequence, and the sequence resets
// when the day changes. A restarted process seeds itself from the journal.
package hardstate

import (
	"fmt"
	"sync"
	"time"
)

// Sequence issues snapshot IDs. It is safe for concurrent use.
type Sequence struct {
	mu   sync.Mutex
	date string
	last int
}

// NewSequence returns a sequence starting from zero.
func NewSequence() *Sequence {
	return &Sequence{}
}

// Next returns the next ID for the given local day. A new day resets the
// counter to 1.
func (s *Sequence) Next(now time.Time) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	day := now.Format("20060102")
	if s.date == "" {
		s.date = day
	} else if s.date != day {
		s.date = day
		s.last = 0
	}
	s.last++
	return fmt.Sprintf("evt_%s_%04d", day, s.last)
}

// SetLast seeds the counter from a previous run. It is only meaningful before
// the first Next call of the current day; a later day switch resets it anyway.
func (s *Sequence) SetLast(n int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if n < 0 {
		n = 0
	}
	s.last = n
}

// Last reports the last issued sequence number.
func (s *Sequence) Last() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.last
}

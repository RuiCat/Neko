// nightly.go runs the in-process daily decay: one timer per Runtime, local
// timezone, persists the result, and stops cleanly on Close.
package bootstrap

import (
	"fmt"
	"log"
	"strings"
	"time"
)

// DEFAULT_NIGHTLY_FACTOR halves every weight per nightly pass.
const DEFAULT_NIGHTLY_FACTOR = 0.5

// parseNightlyAt parses "HH:MM" in the host's local timezone.
func parseNightlyAt(value string) (hour, minute int, err error) {
	trimmed := strings.TrimSpace(value)
	parsed, parseErr := time.Parse("15:04", trimmed)
	if parseErr != nil {
		return 0, 0, fmt.Errorf("nightly-at 必须是 HH:MM（24 小时制），得到 %q", value)
	}
	return parsed.Hour(), parsed.Minute(), nil
}

// nextNightly returns the next local occurrence strictly after now.
func nextNightly(now time.Time, hour, minute int) time.Time {
	next := time.Date(now.Year(), now.Month(), now.Day(), hour, minute, 0, 0, now.Location())
	if !next.After(now) {
		next = next.AddDate(0, 0, 1)
	}
	return next
}

// startNightly enables the daily decay when Options.NightlyAt is set.
func (r *Runtime) startNightly(options Options) {
	if strings.TrimSpace(options.NightlyAt) == "" {
		return
	}
	hour, minute, err := parseNightlyAt(options.NightlyAt)
	if err != nil {
		log.Printf("astrbox: 夜间衰减未启用: %v", err)
		return
	}
	factor := options.NightlyFactor
	if factor <= 0 || factor >= 1 {
		factor = DEFAULT_NIGHTLY_FACTOR
	}
	r.nightlyStop = make(chan struct{})
	r.nightlyDone = make(chan struct{})
	go r.runNightly(hour, minute, factor)
	log.Printf("astrbox: 夜间衰减已启用：每天 %02d:%02d（factor=%.2f）", hour, minute, factor)
}

// runNightly waits for the next local occurrence, decays, persists, repeats.
func (r *Runtime) runNightly(hour, minute int, factor float64) {
	defer close(r.nightlyDone)
	for {
		next := nextNightly(time.Now(), hour, minute)
		timer := time.NewTimer(time.Until(next))
		select {
		case <-r.nightlyStop:
			timer.Stop()
			return
		case <-timer.C:
			removed := r.Experts.DecayAll(factor)
			total := 0
			for _, count := range removed {
				total += count
			}
			if err := r.SaveExperts(); err != nil {
				log.Printf("astrbox: 夜间衰减落盘失败: %v", err)
			}
			log.Printf("astrbox: 夜间衰减完成：移除 %d 条（factor=%.2f）", total, factor)
		}
	}
}

// stopNightly waits for the timer goroutine to exit.
func (r *Runtime) stopNightly() {
	if r.nightlyStop == nil {
		return
	}
	close(r.nightlyStop)
	<-r.nightlyDone
	r.nightlyStop = nil
	r.nightlyDone = nil
}

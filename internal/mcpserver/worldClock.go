// World clock: advances the in-world time that .neko time-driven rules read.
//
// The engine itself has no clock: `世界.环境.当前时间` is written only here,
// when a tick asks for an advance. Keeping the arithmetic in Go (instead of a
// script) makes day/month rollover exact and keeps the phase label aligned with
// the `时段判断` function used by the scripts.
package mcpserver

import "time"

const (
	// WORLD_ENTITY is the single entity that owns the in-world clock.
	WORLD_ENTITY = "世界"
	// WORLD_TIME_PATH is the RFC3339 timestamp rules read through 时段判断.
	WORLD_TIME_PATH = "环境.当前时间"
	// WORLD_MONTH_PATH mirrors the month so season rules see rollover.
	WORLD_MONTH_PATH = "环境.当前月份"
	// WORLD_PHASE_PATH is the coarse phase label (凌晨/清晨/上午/下午/傍晚/深夜).
	WORLD_PHASE_PATH = "状态.阶段"
	// MAX_CLOCK_ADVANCE_MINUTES bounds one tick's requested advance.
	MAX_CLOCK_ADVANCE_MINUTES = 24 * 60
)

// phaseOf maps an hour to the same label 时段判断 returns in .neko scripts.
func phaseOf(hour int) string {
	switch {
	case hour >= 6 && hour < 10:
		return "清晨"
	case hour >= 10 && hour < 14:
		return "上午"
	case hour >= 14 && hour < 18:
		return "下午"
	case hour >= 18 && hour < 22:
		return "傍晚"
	case hour >= 22:
		return "深夜"
	default:
		return "凌晨"
	}
}

// AdvanceWorldClock adds minutes to the in-world clock, refreshing the month
// and phase labels in the same write. It reports the new timestamp and whether
// anything was written. Missing entity/fields or an unparsable timestamp are a
// silent no-op: the tick must never fail because the world entity is absent.
//
// Callers must hold the tick gate (this is invoked from runTickLocked) so the
// read-modify-write below cannot race another state write.
func (s *Server) AdvanceWorldClock(minutes int) (string, bool) {
	if minutes <= 0 || minutes > MAX_CLOCK_ADVANCE_MINUTES || s.store == nil {
		return "", false
	}
	world := s.store.Get(WORLD_ENTITY)
	if world == nil {
		return "", false
	}
	raw, err := world.Get(WORLD_TIME_PATH)
	if err != nil {
		return "", false
	}
	text, ok := raw.(string)
	if !ok {
		return "", false
	}
	current, err := time.Parse(time.RFC3339, text)
	if err != nil {
		return "", false
	}
	next := current.Add(time.Duration(minutes) * time.Minute)
	formatted := next.Format("2006-01-02T15:04:05-07:00")
	if err := world.Set(WORLD_TIME_PATH, formatted); err != nil {
		return "", false
	}
	_ = world.Set(WORLD_MONTH_PATH, int(next.Month()))
	_ = world.Set(WORLD_PHASE_PATH, phaseOf(next.Hour()))
	return formatted, true
}

// WorldTime reads the current in-world timestamp for diagnostics/responses.
func (s *Server) WorldTime() string {
	if s.store == nil {
		return ""
	}
	world := s.store.Get(WORLD_ENTITY)
	if world == nil {
		return ""
	}
	raw, err := world.Get(WORLD_TIME_PATH)
	if err != nil {
		return ""
	}
	text, ok := raw.(string)
	if !ok {
		return ""
	}
	return text
}

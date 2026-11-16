// World clock: reads, advances and sets the in-world time that .neko
// time-driven rules read.
//
// The engine itself has no clock: `世界.环境.当前时间` is written only here,
// when a tick asks for an advance or the control plane sets it. Keeping the
// arithmetic in Go (instead of a script) makes day/month rollover exact and
// keeps the phase label aligned with the `时段判断` function used by the
// scripts.
package mcpserver

import (
	"errors"
	"fmt"
	"time"

	"github.com/RuiCat/Neko/internal/config"
	"github.com/RuiCat/Neko/internal/entity"
)

const (
	// MAX_CLOCK_ADVANCE_MINUTES bounds one tick's requested advance.
	MAX_CLOCK_ADVANCE_MINUTES = 24 * 60
	// CLOCK_TIME_FORMAT is the on-entity timestamp layout (RFC3339, seconds).
	CLOCK_TIME_FORMAT = "2006-01-02T15:04:05-07:00"
)

// storedPhase returns the world entity's current phase label (script-owned).
func storedPhase(world *entity.Entity) string {
	value, err := world.Get(worldPhasePath())
	if err != nil {
		return ""
	}
	text, _ := value.(string)
	return text
}

// 世界时钟的实体与路径是数据（config.world_*），源码不再绑定具体世界实体。
func worldEntityID() string { return config.Current().WorldEntity }

// WorldEntityID returns the configured world entity id (for the control plane).
func WorldEntityID() string  { return worldEntityID() }
func worldTimePath() string  { return config.Current().WorldTimePath }
func worldMonthPath() string { return config.Current().WorldMonthPath }
func worldPhasePath() string { return config.Current().WorldPhasePath }

// Clock read/write failures the control plane maps onto 404/400 and the MCP
// tool reports verbatim.
var (
	// ErrWorldEntityMissing marks a missing 世界 entity.
	ErrWorldEntityMissing = errors.New("世界实体不存在（config.world_entity）")
	// ErrWorldTimeInvalid marks an unusable 世界.环境.当前时间 value.
	ErrWorldTimeInvalid = errors.New("世界.环境.当前时间 不是合法 RFC3339 时间")
)

// ClockState is the in-world clock triple exposed by POST /clock and the
// world_clock tool.
type ClockState struct {
	Time  string `json:"worldTime"`
	Month int    `json:"month"`
	Phase string `json:"phase"`
}

// worldEntity returns the 世界 entity or ErrWorldEntityMissing.
func (s *Server) worldEntity() (*entity.Entity, error) {
	if s.store == nil {
		return nil, ErrWorldEntityMissing
	}
	world := s.store.Get(worldEntityID())
	if world == nil {
		return nil, ErrWorldEntityMissing
	}
	return world, nil
}

// readWorldTime parses the current in-world timestamp of one world entity.
func readWorldTime(world *entity.Entity) (string, time.Time, error) {
	raw, err := world.Get(worldTimePath())
	if err != nil {
		return "", time.Time{}, ErrWorldTimeInvalid
	}
	text, ok := raw.(string)
	if !ok {
		return "", time.Time{}, ErrWorldTimeInvalid
	}
	moment, err := time.Parse(time.RFC3339, text)
	if err != nil {
		return "", time.Time{}, ErrWorldTimeInvalid
	}
	return text, moment, nil
}

// writeWorldClock stores moment as the timestamp plus its month and phase
// labels, returning the resulting clock state. The timestamp write is the only
// step that can fail; the two mirrors are best-effort like before.
func writeWorldClock(world *entity.Entity, moment time.Time) (ClockState, error) {
	formatted := moment.Format(CLOCK_TIME_FORMAT)
	if err := world.Set(worldTimePath(), formatted); err != nil {
		return ClockState{}, fmt.Errorf("写入世界时间失败: %w", err)
	}
	_ = world.Set(worldMonthPath(), int(moment.Month()))
	// 阶段标签由脚本规则（scripts/kernel/world/worldPhase.neko）在 tick 时校准。
	return ClockState{Time: formatted, Month: int(moment.Month()), Phase: storedPhase(world)}, nil
}

// WorldClockState reads the current in-world clock without taking the state
// gate (entity reads are lock-protected already).
func (s *Server) WorldClockState() (ClockState, error) {
	world, err := s.worldEntity()
	if err != nil {
		return ClockState{}, err
	}
	text, moment, err := readWorldTime(world)
	if err != nil {
		return ClockState{}, err
	}
	return ClockState{Time: text, Month: int(moment.Month()), Phase: storedPhase(world)}, nil
}

// AdvanceWorldClockState adds minutes (1..MAX_CLOCK_ADVANCE_MINUTES) to the
// in-world clock, refreshing the month and phase labels in the same write.
// Callers hold the state gate when writing so the read-modify-write cannot race
// another state write.
func (s *Server) AdvanceWorldClockState(minutes int) (ClockState, error) {
	if minutes <= 0 || minutes > MAX_CLOCK_ADVANCE_MINUTES {
		return ClockState{}, fmt.Errorf("分钟数必须在 1..%d 之间", MAX_CLOCK_ADVANCE_MINUTES)
	}
	world, err := s.worldEntity()
	if err != nil {
		return ClockState{}, err
	}
	_, moment, err := readWorldTime(world)
	if err != nil {
		return ClockState{}, err
	}
	return writeWorldClock(world, moment.Add(time.Duration(minutes)*time.Minute))
}

// SetWorldClockState writes an explicit moment into the in-world clock and
// returns the resulting state. Callers hold the state gate.
func (s *Server) SetWorldClockState(moment time.Time) (ClockState, error) {
	world, err := s.worldEntity()
	if err != nil {
		return ClockState{}, err
	}
	return writeWorldClock(world, moment)
}

// AdvanceWorldClock adds minutes to the in-world clock. It reports the new
// timestamp and whether anything was written. Missing entity/fields or an
// unparsable timestamp are a silent no-op: the tick must never fail because the
// world entity is absent.
//
// Callers must hold the tick gate (this is invoked from runTickLocked).
func (s *Server) AdvanceWorldClock(minutes int) (string, bool) {
	state, err := s.AdvanceWorldClockState(minutes)
	if err != nil {
		return "", false
	}
	return state.Time, true
}

// WorldTime reads the current in-world timestamp for diagnostics/responses.
func (s *Server) WorldTime() string {
	if s.store == nil {
		return ""
	}
	world := s.store.Get(worldEntityID())
	if world == nil {
		return ""
	}
	raw, err := world.Get(worldTimePath())
	if err != nil {
		return ""
	}
	text, ok := raw.(string)
	if !ok {
		return ""
	}
	return text
}

package mcpserver

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/RuiCat/Neko/internal/engine"
	"github.com/RuiCat/Neko/internal/entity"
	"github.com/RuiCat/Neko/internal/registry"
)

// newClockFixture builds a server whose store owns one 世界 entity with a
// frozen in-world clock, mirroring the shipped data layout.
func newClockFixture(t *testing.T, worldTime string) (*Server, *entity.Entity, string) {
	t.Helper()
	dataDir := t.TempDir()
	store := entity.NewStore(dataDir)
	if err := store.Load(); err != nil {
		t.Fatalf("load store: %v", err)
	}
	world, err := store.Create(WORLD_ENTITY, WORLD_ENTITY, map[string]any{
		"环境": map[string]any{
			"当前时间": worldTime,
			"当前月份": 8,
		},
		"状态": map[string]any{"阶段": "下午"},
	}, nil)
	if err != nil {
		t.Fatalf("create world: %v", err)
	}
	machine := engine.New(store)
	reg, err := registry.New(store, machine, filepath.Join(t.TempDir(), "scripts.json"), t.TempDir())
	if err != nil {
		t.Fatalf("registry: %v", err)
	}
	return New(store, machine, reg), world, dataDir
}

func worldString(t *testing.T, world *entity.Entity, path string) string {
	t.Helper()
	value, err := world.Get(path)
	if err != nil {
		t.Fatalf("get %s: %v", path, err)
	}
	text, ok := value.(string)
	if !ok {
		t.Fatalf("%s = %#v (want string)", path, value)
	}
	return text
}

func worldInt(t *testing.T, world *entity.Entity, path string) int {
	t.Helper()
	value, err := world.Get(path)
	if err != nil {
		t.Fatalf("get %s: %v", path, err)
	}
	number, ok := value.(int)
	if !ok {
		t.Fatalf("%s = %#v (want int)", path, value)
	}
	return number
}

func TestPhaseOfMatchesScripts(t *testing.T) {
	cases := map[int]string{
		0: "凌晨", 5: "凌晨", 6: "清晨", 9: "清晨", 10: "上午", 13: "上午",
		14: "下午", 17: "下午", 18: "傍晚", 21: "傍晚", 22: "深夜", 23: "深夜",
	}
	for hour, want := range cases {
		if got := phaseOf(hour); got != want {
			t.Errorf("phaseOf(%d) = %q, want %q", hour, got, want)
		}
	}
}

func TestAdvanceWorldClockSameDay(t *testing.T) {
	server, world, _ := newClockFixture(t, "2026-08-05T17:00:00+08:00")

	got, wrote := server.AdvanceWorldClock(10)
	if !wrote || got != "2026-08-05T17:10:00+08:00" {
		t.Fatalf("advance = %q/%v, want 2026-08-05T17:10…/true", got, wrote)
	}
	if time := worldString(t, world, WORLD_TIME_PATH); time != got {
		t.Fatalf("stored time = %q, want %q", time, got)
	}
	if phase := worldString(t, world, WORLD_PHASE_PATH); phase != "下午" {
		t.Fatalf("phase = %q, want 下午", phase)
	}
}

func TestAdvanceWorldClockRollsDaysAndMonths(t *testing.T) {
	server, world, _ := newClockFixture(t, "2026-08-31T23:50:00+08:00")

	got, wrote := server.AdvanceWorldClock(20)
	if !wrote || got != "2026-09-01T00:10:00+08:00" {
		t.Fatalf("advance = %q/%v, want 2026-09-01T00:10…/true", got, wrote)
	}
	if month := worldInt(t, world, WORLD_MONTH_PATH); month != 9 {
		t.Fatalf("month = %d, want 9", month)
	}
	if phase := worldString(t, world, WORLD_PHASE_PATH); phase != "凌晨" {
		t.Fatalf("phase = %q, want 凌晨", phase)
	}
}

func TestAdvanceWorldClockRejectsInvalidInput(t *testing.T) {
	server, world, _ := newClockFixture(t, "2026-08-05T17:00:00+08:00")
	for _, minutes := range []int{0, -1, MAX_CLOCK_ADVANCE_MINUTES + 1} {
		if _, wrote := server.AdvanceWorldClock(minutes); wrote {
			t.Fatalf("AdvanceWorldClock(%d) wrote, want no-op", minutes)
		}
	}
	if time := worldString(t, world, WORLD_TIME_PATH); time != "2026-08-05T17:00:00+08:00" {
		t.Fatalf("time changed to %q", time)
	}
}

func TestAdvanceWorldClockMissingWorldIsNoop(t *testing.T) {
	dataDir := t.TempDir()
	store := entity.NewStore(dataDir)
	if err := store.Load(); err != nil {
		t.Fatalf("load store: %v", err)
	}
	machine := engine.New(store)
	reg, err := registry.New(store, machine, filepath.Join(t.TempDir(), "scripts.json"), t.TempDir())
	if err != nil {
		t.Fatalf("registry: %v", err)
	}
	server := New(store, machine, reg)
	if _, wrote := server.AdvanceWorldClock(10); wrote {
		t.Fatal("missing 世界 entity must be a silent no-op")
	}
	if got := server.WorldTime(); got != "" {
		t.Fatalf("WorldTime = %q, want empty", got)
	}
}

func TestRunTickAdvancesClockAndPersists(t *testing.T) {
	server, _, dataDir := newClockFixture(t, "2026-08-05T17:00:00+08:00")

	if _, err := server.RunTick("", TickOptions{AdvanceMinutes: 10}); err != nil {
		t.Fatalf("run tick: %v", err)
	}
	if got := server.WorldTime(); got != "2026-08-05T17:10:00+08:00" {
		t.Fatalf("WorldTime = %q, want 2026-08-05T17:10:00+08:00", got)
	}
	raw, err := os.ReadFile(filepath.Join(dataDir, "entities.json"))
	if err != nil {
		t.Fatalf("read entities.json: %v", err)
	}
	var payload struct {
		Entities map[string]struct {
			Data map[string]any `json:"data"`
		} `json:"entities"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		t.Fatalf("parse entities.json: %v", err)
	}
	worldData := payload.Entities[WORLD_ENTITY].Data
	env, _ := worldData["环境"].(map[string]any)
	if got, _ := env["当前时间"].(string); got != "2026-08-05T17:10:00+08:00" {
		t.Fatalf("persisted time = %q", got)
	}
}

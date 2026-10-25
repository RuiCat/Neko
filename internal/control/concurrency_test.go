package control

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/ruicat/astrbox/internal/engine"
	"github.com/ruicat/astrbox/internal/mcpserver"
)

// TestConcurrentTickStateSetAndStateRead runs the three entry points that share
// the runtime at once; the -race build reports any unsynchronized access.
func TestConcurrentTickStateSetAndStateRead(t *testing.T) {
	env := newTestEnv(t, Options{})
	env.runtime.Machine.AddRule(engine.Rule{
		ID: "r_concurrency", Match: "NPC[*]", Priority: 1, Enabled: true,
		Branches: []engine.Branch{{Then: []engine.Op{{
			Op: "set", Path: "{e}.状态.并发计数", Value: "运行中",
		}}}},
	})
	entity := env.runtime.Store.Get("猫猫")
	const (
		writers = 3
		rounds  = 30
	)
	var waitGroup sync.WaitGroup

	for worker := 0; worker < writers; worker++ {
		waitGroup.Add(1)
		go func() {
			defer waitGroup.Done()
			for round := 0; round < rounds; round++ {
				if _, err := env.mcp.RunTick("", mcpserver.TickOptions{SkipSaveWhenUnchanged: true}); err != nil {
					t.Errorf("RunTick: %v", err)
					return
				}
			}
		}()
	}

	for worker := 0; worker < writers; worker++ {
		waitGroup.Add(1)
		go func(offset int) {
			defer waitGroup.Done()
			for round := 0; round < rounds; round++ {
				value := fmt.Sprintf("设置-%d-%d", offset, round)
				err := env.mcp.WithStateWrite(func() error {
					if err := entity.Set("状态.并发设置", value); err != nil {
						return err
					}
					return env.runtime.Store.Save()
				})
				if err != nil {
					t.Errorf("state set: %v", err)
					return
				}
			}
		}(worker)
	}

	for worker := 0; worker < writers; worker++ {
		waitGroup.Add(1)
		go func() {
			defer waitGroup.Done()
			for round := 0; round < rounds; round++ {
				recorder := env.call(t, http.MethodGet, "/state?entity=猫猫&path=状态.情绪", nil)
				if recorder.Code != http.StatusOK {
					t.Errorf("/state status = %d (%s)", recorder.Code, recorder.Body.String())
					return
				}
			}
		}()
	}

	done := make(chan struct{})
	go func() { waitGroup.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(30 * time.Second):
		t.Fatal("concurrency workload did not finish")
	}
}

// TestLoggerPrunesOldControlLogs keeps 7 days and removes older daily files.
func TestLoggerPrunesOldControlLogs(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "logs")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	oldName := CONTROL_LOG_PREFIX + time.Now().AddDate(0, 0, -10).Format(CONTROL_LOG_DATE_FORMAT) + CONTROL_LOG_SUFFIX
	oldPath := filepath.Join(dir, oldName)
	if err := os.WriteFile(oldPath, []byte("old\n"), 0o644); err != nil {
		t.Fatalf("write old log: %v", err)
	}
	logger := newDailyLogger(dir, CONTROL_LOG_KEEP_DAYS)
	logger.append("GET /health 200 1ms")
	if _, err := os.Stat(oldPath); !os.IsNotExist(err) {
		t.Fatalf("old control log not pruned: %v", err)
	}
	todayPath := filepath.Join(dir, CONTROL_LOG_PREFIX+time.Now().Format(CONTROL_LOG_DATE_FORMAT)+CONTROL_LOG_SUFFIX)
	raw, err := os.ReadFile(todayPath)
	if err != nil {
		t.Fatalf("today log missing: %v", err)
	}
	if !containsAll(string(raw), "GET /health 200") {
		t.Fatalf("today log = %s", raw)
	}
}

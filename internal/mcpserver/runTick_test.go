package mcpserver

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/ruicat/astrbox/internal/engine"
	"github.com/ruicat/astrbox/internal/entity"
	"github.com/ruicat/astrbox/internal/registry"
)

type tickFixture struct {
	server  *Server
	machine *engine.StateMachine
	entity  *entity.Entity
	dataDir string
}

func newTickFixture(t *testing.T) *tickFixture {
	t.Helper()
	dataDir := t.TempDir()
	store := entity.NewStore(dataDir)
	if err := store.Load(); err != nil {
		t.Fatalf("load store: %v", err)
	}
	ent, err := store.Create("猫猫", "NPC", map[string]any{"身体": map[string]any{"体力": 50}}, nil)
	if err != nil {
		t.Fatalf("create entity: %v", err)
	}
	machine := engine.New(store)
	reg, err := registry.New(store, machine, filepath.Join(t.TempDir(), "scripts.json"), t.TempDir())
	if err != nil {
		t.Fatalf("registry: %v", err)
	}
	server := New(store, machine, reg)
	server.SetDataDir(dataDir)
	return &tickFixture{server: server, machine: machine, entity: ent, dataDir: dataDir}
}

func TestRunTickSaveOptions(t *testing.T) {
	fx := newTickFixture(t)
	entitiesPath := filepath.Join(fx.dataDir, "entities.json")

	result, err := fx.server.RunTick("", TickOptions{SkipSaveWhenUnchanged: true})
	if err != nil {
		t.Fatalf("run tick: %v", err)
	}
	if len(result.Transitions) != 0 || len(result.Conflicts) != 0 {
		t.Fatalf("expected unchanged tick, got %+v", result)
	}
	if _, err := os.Stat(entitiesPath); !os.IsNotExist(err) {
		t.Fatalf("unchanged tick with skip option wrote entities.json: %v", err)
	}

	fx.machine.AddRule(engine.Rule{
		ID: "m1.save", Match: "[*]", Enabled: true,
		Branches: []engine.Branch{{Then: []engine.Op{{Op: "set", Path: "身体.体力", Value: 40}}}},
	})
	if _, err := fx.server.RunTick("", TickOptions{SkipSaveWhenUnchanged: true}); err != nil {
		t.Fatalf("run tick: %v", err)
	}
	if _, err := os.Stat(entitiesPath); err != nil {
		t.Fatalf("changed tick with skip option must save: %v", err)
	}
	if err := os.Remove(entitiesPath); err != nil {
		t.Fatalf("remove entities file: %v", err)
	}

	if _, err := fx.server.RunTick("", TickOptions{}); err != nil {
		t.Fatalf("run tick: %v", err)
	}
	if _, err := os.Stat(entitiesPath); err != nil {
		t.Fatalf("default tick must always save: %v", err)
	}
}

func TestStateMachineTickToolAlwaysSaves(t *testing.T) {
	fx := newTickFixture(t)
	session, cancel := connect(t, fx.server)
	defer cancel()
	ctx, stop := context.WithTimeout(context.Background(), 10*time.Second)
	defer stop()

	result, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name: "state_machine_tick", Arguments: map[string]any{"scope": "*"},
	})
	if err != nil {
		t.Fatalf("call state_machine_tick: %v", err)
	}
	if len(result.Content) == 0 {
		t.Fatal("empty tick result")
	}
	if text := result.Content[0].(*mcp.TextContent).Text; strings.HasPrefix(text, "❌") {
		t.Fatalf("tick failed: %s", text)
	}
	if _, err := os.Stat(filepath.Join(fx.dataDir, "entities.json")); err != nil {
		t.Fatalf("state_machine_tick must always save: %v", err)
	}
}

func TestRunTickConcurrentWithSetAndSnapshot(t *testing.T) {
	fx := newTickFixture(t)
	fx.machine.AddRule(engine.Rule{
		ID: "m1.concurrent", Match: "[*]", Enabled: true,
		Branches: []engine.Branch{{Then: []engine.Op{{Op: "set", Path: "身体.体力", Value: 40}}}},
	})
	session, cancel := connect(t, fx.server)
	defer cancel()
	ctx, stop := context.WithTimeout(context.Background(), 30*time.Second)
	defer stop()

	const rounds = 50
	var wait sync.WaitGroup
	wait.Add(3)
	go func() {
		defer wait.Done()
		for i := 0; i < rounds; i++ {
			if _, err := fx.server.RunTick("*", TickOptions{SkipSaveWhenUnchanged: true}); err != nil {
				t.Errorf("run tick: %v", err)
				return
			}
		}
	}()
	go func() {
		defer wait.Done()
		for i := 0; i < rounds; i++ {
			result, err := session.CallTool(ctx, &mcp.CallToolParams{
				Name:      "entity_state_set",
				Arguments: map[string]any{"entity": "猫猫", "path": "身体.体力", "value": strconv.Itoa(i)},
			})
			if err != nil {
				t.Errorf("entity_state_set: %v", err)
				return
			}
			if len(result.Content) == 0 || strings.HasPrefix(result.Content[0].(*mcp.TextContent).Text, "❌") {
				t.Errorf("entity_state_set failed: %+v", result.Content)
				return
			}
		}
	}()
	go func() {
		defer wait.Done()
		for i := 0; i < rounds; i++ {
			if snapshot := fx.entity.Snapshot(); snapshot == nil {
				t.Error("snapshot is nil")
				return
			}
		}
	}()
	wait.Wait()
}

func TestWithStateWriteReturnsCallbackError(t *testing.T) {
	fx := newTickFixture(t)
	want := errors.New("write failed")
	if err := fx.server.WithStateWrite(func() error { return want }); !errors.Is(err, want) {
		t.Fatalf("WithStateWrite error = %v, want %v", err, want)
	}
	if err := fx.server.WithStateWrite(func() error { return nil }); err != nil {
		t.Fatalf("gate not released after callback error: %v", err)
	}
}

func TestWithStateWriteSerializesWithRunTick(t *testing.T) {
	fx := newTickFixture(t)
	fx.machine.AddRule(engine.Rule{
		ID: "m1.gate", Match: "[*]", Enabled: true,
		Branches: []engine.Branch{{Then: []engine.Op{{Op: "set", Path: "身体.体力", Value: 40}}}},
	})
	// The change listener only fires from inside a tick, so overlap proves the
	// tick's critical section crossed WithStateWrite's callback.
	var writeActive atomic.Bool
	overlap := make(chan struct{}, 1)
	fx.entity.AddChangeListener(func(_ *entity.Entity, _ string, _, _ any) {
		if writeActive.Load() {
			select {
			case overlap <- struct{}{}:
			default:
			}
		}
	})

	entered := make(chan struct{})
	release := make(chan struct{})
	writeDone := make(chan error, 1)
	go func() {
		writeDone <- fx.server.WithStateWrite(func() error {
			writeActive.Store(true)
			close(entered)
			<-release
			writeActive.Store(false)
			return nil
		})
	}()
	<-entered

	tickDone := make(chan error, 1)
	go func() {
		_, err := fx.server.RunTick("", TickOptions{SkipSaveWhenUnchanged: true})
		tickDone <- err
	}()

	time.Sleep(20 * time.Millisecond)
	select {
	case err := <-tickDone:
		t.Fatalf("RunTick finished while WithStateWrite held the gate (err=%v)", err)
	default:
	}
	close(release)
	if err := <-writeDone; err != nil {
		t.Fatalf("WithStateWrite: %v", err)
	}
	if err := <-tickDone; err != nil {
		t.Fatalf("RunTick: %v", err)
	}
	select {
	case <-overlap:
		t.Fatal("RunTick critical section overlapped WithStateWrite")
	default:
	}
}

func TestTryRunTickBusyWhileGateHeld(t *testing.T) {
	fx := newTickFixture(t)
	entered := make(chan struct{})
	release := make(chan struct{})
	writeDone := make(chan error, 1)
	go func() {
		writeDone <- fx.server.WithStateWrite(func() error {
			close(entered)
			<-release
			return nil
		})
	}()
	<-entered

	type tryOutcome struct {
		result engine.TickResult
		busy   bool
		err    error
	}
	outcome := make(chan tryOutcome, 1)
	go func() {
		result, busy, err := fx.server.TryRunTick("", TickOptions{SkipSaveWhenUnchanged: true})
		outcome <- tryOutcome{result: result, busy: busy, err: err}
	}()
	select {
	case got := <-outcome:
		if got.err != nil {
			t.Fatalf("busy TryRunTick returned error: %v", got.err)
		}
		if got.busy {
			t.Fatal("TryRunTick acquired the gate while WithStateWrite held it")
		}
		if got.result.TickID != "" {
			t.Fatalf("busy TryRunTick returned non-zero result: %+v", got.result)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("TryRunTick blocked while the gate was held")
	}

	close(release)
	if err := <-writeDone; err != nil {
		t.Fatalf("WithStateWrite: %v", err)
	}
	result, busy, err := fx.server.TryRunTick("", TickOptions{SkipSaveWhenUnchanged: true})
	if err != nil {
		t.Fatalf("TryRunTick after release: %v", err)
	}
	if !busy {
		t.Fatal("TryRunTick did not acquire the released gate")
	}
	if result.TickID == "" {
		t.Fatalf("TryRunTick returned zero result after acquiring the gate: %+v", result)
	}
}

package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"
)

// TestE2EConcurrentTickStateSetAndStateRead drives tick, state_set and /state at
// once on one real process; the -race e2e build fails on the first data race.
func TestE2EConcurrentTickStateSetAndStateRead(t *testing.T) {
	dataDir := t.TempDir()
	scriptsDir := t.TempDir()
	prepareMinimalData(t, dataDir)
	session := startStdioSessionWithDirs(t, dataDir, scriptsDir)
	session.initialize(t)
	client := newUnixClient(session.socketPath)

	deadline := time.Now().Add(15 * time.Second)
	for {
		if status, _, err := requestJSON(client, http.MethodGet, "/health", nil); err == nil && status == http.StatusOK {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("控制面未就绪（stderr=%s）", session.stderr.String())
		}
		time.Sleep(20 * time.Millisecond)
	}

	const rounds = 6
	failures := make(chan error, 8)
	var waitGroup sync.WaitGroup

	waitGroup.Add(1)
	go func() {
		defer waitGroup.Done()
		for round := 0; round < rounds; round++ {
			if _, err := session.callToolErr("state_machine_tick", map[string]any{"scope": ""}); err != nil {
				failures <- fmt.Errorf("MCP tick: %w", err)
				return
			}
		}
	}()

	waitGroup.Add(1)
	go func() {
		defer waitGroup.Done()
		for round := 0; round < rounds; round++ {
			arguments := map[string]any{
				"entity": "猫猫", "path": "状态.并发值", "value": strconv.Itoa(round),
			}
			if _, err := session.callToolErr("entity_state_set", arguments); err != nil {
				failures <- fmt.Errorf("MCP state_set: %w", err)
				return
			}
		}
	}()

	waitGroup.Add(1)
	go func() {
		defer waitGroup.Done()
		for round := 0; round < rounds; round++ {
			status, _, err := requestJSON(client, http.MethodPost, "/tick", []byte(`{}`))
			if err != nil {
				failures <- fmt.Errorf("POST /tick: %w", err)
				return
			}
			if status != http.StatusOK && status != http.StatusConflict {
				failures <- fmt.Errorf("POST /tick status = %d", status)
				return
			}
		}
	}()

	waitGroup.Add(1)
	go func() {
		defer waitGroup.Done()
		for round := 0; round < rounds*3; round++ {
			status, _, err := requestJSON(client, http.MethodGet, "/state?entity=猫猫&path=状态.情绪", nil)
			if err != nil {
				failures <- fmt.Errorf("GET /state: %w", err)
				return
			}
			if status != http.StatusOK {
				failures <- fmt.Errorf("GET /state status = %d", status)
				return
			}
		}
	}()

	done := make(chan struct{})
	go func() { waitGroup.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(60 * time.Second):
		t.Fatal("并发负载未在 60s 内完成")
	}
	close(failures)
	for err := range failures {
		t.Fatalf("并发请求失败: %v（stderr=%s）", err, session.stderr.String())
	}

	if err := session.command.Process.Signal(syscall.SIGTERM); err != nil {
		t.Fatalf("signal: %v", err)
	}
	if code := session.shutdown(t); code != EXIT_OK {
		t.Fatalf("exit code = %d（stderr=%s）", code, session.stderr.String())
	}
	session.assertStdoutFrames(t)
	if strings.Contains(session.stderr.String(), "DATA RACE") {
		t.Fatalf("stderr 存在数据竞争报告: %s", session.stderr.String())
	}
	assertEntitiesSaved(t, dataDir)
}

// assertEntitiesSaved verifies that the concurrent writes reached disk.
func assertEntitiesSaved(t *testing.T, dataDir string) {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(dataDir, "user", "entities.json"))
	if err != nil {
		t.Fatalf("读取 entities.json: %v", err)
	}
	if !strings.Contains(string(raw), "并发值") {
		t.Fatalf("并发写入未落盘: %s", raw)
	}
	var payload map[string]any
	if err := json.Unmarshal(raw, &payload); err != nil {
		t.Fatalf("entities.json 不是 JSON: %v", err)
	}
}

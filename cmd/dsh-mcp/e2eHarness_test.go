package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

// serverBinary is the real dsh-mcp binary built once for the e2e tests.
var serverBinary string

// TestMain builds the server binary (race-instrumented under go test -race).
func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "dsh-mcp-e2e")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	serverBinary = filepath.Join(dir, "dsh-mcp")
	args := []string{"build"}
	if raceInstrumented {
		args = append(args, "-race")
	}
	args = append(args, "-o", serverBinary, ".")
	build := exec.Command("go", args...)
	build.Stderr = os.Stderr
	if err := build.Run(); err != nil {
		fmt.Fprintln(os.Stderr, "build e2e server:", err)
		_ = os.RemoveAll(dir)
		os.Exit(1)
	}
	fmt.Fprintf(os.Stderr, "e2e server binary: %s (race=%v)\n", serverBinary, raceInstrumented)
	code := m.Run()
	_ = os.RemoveAll(dir)
	os.Exit(code)
}

// syncBuffer is a goroutine-safe bytes.Buffer for child stderr.
type syncBuffer struct {
	mu     sync.Mutex
	buffer bytes.Buffer
}

// Write appends child output.
func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buffer.Write(p)
}

// String returns everything captured so far.
func (b *syncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buffer.String()
}

// dshProcess is one live dsh-mcp instance with its stdin held open.
type dshProcess struct {
	command    *exec.Cmd
	stdin      *os.File
	dataDir    string
	scriptsDir string
	socketPath string
	stderr     *syncBuffer
	waitDone   chan struct{}
	waitErr    error
	waitOnce   sync.Once
}

// startProcess launches dsh-mcp on fresh temp directories.
func startProcess(t *testing.T, extraArgs ...string) *dshProcess {
	t.Helper()
	return startProcessWithDirs(t, t.TempDir(), t.TempDir(), extraArgs...)
}

// startProcessWithDirs launches dsh-mcp over the given directories.
func startProcessWithDirs(t *testing.T, dataDir, scriptsDir string, extraArgs ...string) *dshProcess {
	t.Helper()
	args := []string{"-data-dir", dataDir, "-scripts-dir", scriptsDir}
	args = append(args, extraArgs...)
	return startCommand(t, dataDir, scriptsDir, args, nil)
}

// startProcessEnv relies on the ASTRBOX_* environment defaults instead of flags.
func startProcessEnv(t *testing.T, dataDir, scriptsDir string) *dshProcess {
	t.Helper()
	return startCommand(t, dataDir, scriptsDir, nil,
		[]string{"ASTRBOX_DATA_DIR=" + dataDir, "ASTRBOX_SCRIPTS_DIR=" + scriptsDir})
}

// startCommand launches dsh-mcp with the given args and extra environment.
func startCommand(t *testing.T, dataDir, scriptsDir string, args, extraEnv []string) *dshProcess {
	t.Helper()
	readEnd, writeEnd, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	command := exec.Command(serverBinary, args...)
	command.Env = append(append(os.Environ(), raceEnv()...), extraEnv...)
	command.Stdin = readEnd
	stderr := &syncBuffer{}
	command.Stderr = stderr
	command.Stdout = io.Discard
	if err := command.Start(); err != nil {
		_ = readEnd.Close()
		_ = writeEnd.Close()
		t.Fatalf("start dsh-mcp: %v", err)
	}
	_ = readEnd.Close()
	process := &dshProcess{
		command: command, stdin: writeEnd, dataDir: dataDir, scriptsDir: scriptsDir,
		socketPath: filepath.Join(dataDir, "dsh-control.sock"), stderr: stderr,
		waitDone: make(chan struct{}),
	}
	go func() {
		process.waitErr = command.Wait()
		close(process.waitDone)
	}()
	t.Cleanup(func() {
		_ = writeEnd.Close()
		process.killIfRunning()
	})
	return process
}

// killIfRunning terminates a still-running child.
func (p *dshProcess) killIfRunning() {
	select {
	case <-p.waitDone:
		return
	default:
	}
	_ = p.command.Process.Kill()
	<-p.waitDone
}

// wait blocks for process exit and returns its code.
func (p *dshProcess) wait(t *testing.T, timeout time.Duration) int {
	t.Helper()
	select {
	case <-p.waitDone:
		return p.exitCode(t)
	case <-time.After(timeout):
		t.Fatalf("dsh-mcp 未在 %s 内退出（stderr=%s）", timeout, p.stderr.String())
		return -1
	}
}

// exitCode extracts the exit code after Wait returned.
func (p *dshProcess) exitCode(t *testing.T) int {
	t.Helper()
	if p.waitErr == nil {
		return 0
	}
	exitError, ok := p.waitErr.(*exec.ExitError)
	if !ok {
		t.Fatalf("wait: %v", p.waitErr)
	}
	return exitError.ExitCode()
}

// healthClient returns a UDS HTTP client for this process.
func (p *dshProcess) healthClient() *http.Client {
	return newUnixClient(p.socketPath)
}

// waitForHealth polls GET /health until the listener answers.
func (p *dshProcess) waitForHealth(t *testing.T) *http.Client {
	t.Helper()
	client := p.healthClient()
	deadline := time.Now().Add(15 * time.Second)
	var lastErr error
	for time.Now().Before(deadline) {
		select {
		case <-p.waitDone:
			t.Fatalf("dsh-mcp 提前退出（code=%d, stderr=%s）", p.exitCodeQuiet(), p.stderr.String())
		default:
		}
		status, payload, err := requestJSON(client, http.MethodGet, "/health", nil)
		if err == nil && status == http.StatusOK && payload["ok"] == true {
			return client
		}
		if err != nil {
			lastErr = err
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("控制面未就绪: %v（stderr=%s）", lastErr, p.stderr.String())
	return nil
}

// exitCodeQuiet reports the exit code without failing the test.
func (p *dshProcess) exitCodeQuiet() int {
	select {
	case <-p.waitDone:
		if p.waitErr == nil {
			return 0
		}
		if exitError, ok := p.waitErr.(*exec.ExitError); ok {
			return exitError.ExitCode()
		}
	default:
	}
	return -1
}

// newUnixClient builds an HTTP client that dials a Unix socket.
func newUnixClient(socketPath string) *http.Client {
	dialer := &net.Dialer{Timeout: time.Second}
	return &http.Client{
		Timeout: 10 * time.Second,
		Transport: &http.Transport{
			DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
				return dialer.DialContext(ctx, "unix", socketPath)
			},
		},
	}
}

// requestJSON performs one control-plane request.
func requestJSON(client *http.Client, method, path string, body []byte) (int, map[string]any, error) {
	var reader io.Reader
	if body != nil {
		reader = bytes.NewReader(body)
	}
	request, err := http.NewRequest(method, "http://unix"+path, reader)
	if err != nil {
		return 0, nil, err
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := client.Do(request)
	if err != nil {
		return 0, nil, err
	}
	defer response.Body.Close()
	raw, err := io.ReadAll(response.Body)
	if err != nil {
		return response.StatusCode, nil, err
	}
	payload := map[string]any{}
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &payload); err != nil {
			return response.StatusCode, nil, fmt.Errorf("响应不是 JSON: %v (%s)", err, raw)
		}
	}
	return response.StatusCode, payload, nil
}

// mustJSON performs one request and fails the test on transport errors.
func mustJSON(t *testing.T, client *http.Client, method, path string, body []byte) (int, map[string]any) {
	t.Helper()
	status, payload, err := requestJSON(client, method, path, body)
	if err != nil {
		t.Fatalf("%s %s: %v", method, path, err)
	}
	return status, payload
}

// prepareMinimalData writes one NPC template and the 猫猫 entity into dataDir.
func prepareMinimalData(t *testing.T, dataDir string) {
	t.Helper()
	templateDir := filepath.Join(dataDir, "templates")
	if err := os.MkdirAll(templateDir, 0o755); err != nil {
		t.Fatalf("mkdir templates: %v", err)
	}
	template := `{"状态":{"情绪":"平静","身体":"健康"},"身体":{"体力":80}}`
	if err := os.WriteFile(filepath.Join(templateDir, "NPC.json"), []byte(template), 0o644); err != nil {
		t.Fatalf("write template: %v", err)
	}
	entities := `{"version":1,"entities":{"猫猫":{"id":"猫猫","type":"NPC","data":{"状态":{"情绪":"平静","身体":"健康"},"身体":{"体力":80},"备注":"e2e"},"keys":{}}}}`
	if err := os.WriteFile(filepath.Join(dataDir, "entities.json"), []byte(entities), 0o644); err != nil {
		t.Fatalf("write entities: %v", err)
	}
}

// waitForFile waits until path exists (or fails the test).
func waitForFile(t *testing.T, path string, timeout time.Duration) os.FileInfo {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		info, err := os.Stat(path)
		if err == nil {
			return info
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("文件未出现: %s", path)
	return nil
}

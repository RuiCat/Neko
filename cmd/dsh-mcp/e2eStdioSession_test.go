package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

// stdioSession is a raw newline-delimited MCP client over a real binary so the
// test can inspect every stdout frame.
type stdioSession struct {
	command    *exec.Cmd
	stdin      io.WriteCloser
	stderr     *syncBuffer
	dataDir    string
	scriptsDir string
	socketPath string

	writeMu  sync.Mutex
	mu       sync.Mutex
	pending  map[int]chan map[string]any
	nextID   int
	lines    []string
	readErr  error
	waitErr  error
	done     chan struct{}
	waitDone chan struct{}
}

// startStdioSession launches dsh-mcp with fresh temp directories.
func startStdioSession(t *testing.T, extraArgs ...string) *stdioSession {
	t.Helper()
	return startStdioSessionWithDirs(t, t.TempDir(), t.TempDir(), extraArgs...)
}

// startStdioSessionWithDirs launches dsh-mcp over the given directories.
func startStdioSessionWithDirs(t *testing.T, dataDir, scriptsDir string, extraArgs ...string) *stdioSession {
	t.Helper()
	args := []string{"-data-dir", dataDir, "-scripts-dir", scriptsDir}
	args = append(args, extraArgs...)
	command := exec.Command(serverBinary, args...)
	command.Env = append(os.Environ(), raceEnv()...)
	stdin, err := command.StdinPipe()
	if err != nil {
		t.Fatalf("stdin pipe: %v", err)
	}
	stdout, err := command.StdoutPipe()
	if err != nil {
		t.Fatalf("stdout pipe: %v", err)
	}
	stderr := &syncBuffer{}
	command.Stderr = stderr
	session := &stdioSession{
		command: command, stdin: stdin, stderr: stderr,
		dataDir: dataDir, scriptsDir: scriptsDir,
		socketPath: filepath.Join(dataDir, "dsh-control.sock"),
		pending:    map[int]chan map[string]any{},
		done:       make(chan struct{}), waitDone: make(chan struct{}),
	}
	if err := command.Start(); err != nil {
		t.Fatalf("start dsh-mcp: %v", err)
	}
	go session.readLoop(stdout)
	go func() {
		session.waitErr = command.Wait()
		close(session.waitDone)
	}()
	t.Cleanup(func() {
		_ = stdin.Close()
		session.killIfRunning()
	})
	return session
}

// raceEnv makes an instrumented server abort on the first data race.
func raceEnv() []string {
	if raceInstrumented {
		return []string{"GORACE=halt_on_error=1"}
	}
	return nil
}

// readLoop parses every stdout line as one MCP frame and routes responses.
func (s *stdioSession) readLoop(stdout io.Reader) {
	defer close(s.done)
	scanner := bufio.NewScanner(stdout)
	scanner.Buffer(make([]byte, 64*1024), 4*1024*1024)
	for scanner.Scan() {
		line := scanner.Text()
		s.mu.Lock()
		s.lines = append(s.lines, line)
		s.mu.Unlock()
		message := map[string]any{}
		if err := json.Unmarshal([]byte(line), &message); err != nil {
			s.setReadError(fmt.Errorf("stdout 不是 MCP 帧: %v（%s）", err, line))
			continue
		}
		rawID, ok := message["id"].(float64)
		if !ok {
			continue
		}
		id := int(rawID)
		s.mu.Lock()
		channel := s.pending[id]
		delete(s.pending, id)
		s.mu.Unlock()
		if channel != nil {
			channel <- message
		}
	}
	if err := scanner.Err(); err != nil {
		s.setReadError(err)
	}
}

// setReadError records the first stdout protocol failure.
func (s *stdioSession) setReadError(err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.readErr == nil {
		s.readErr = err
	}
}

// readError returns the first stdout protocol failure.
func (s *stdioSession) readError() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.readErr
}

// writeFrame writes one newline-delimited JSON-RPC frame.
func (s *stdioSession) writeFrame(frame map[string]any) error {
	raw, err := json.Marshal(frame)
	if err != nil {
		return err
	}
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	_, err = s.stdin.Write(append(raw, '\n'))
	return err
}

// requestErr sends one request and waits for its id-matched response.
func (s *stdioSession) requestErr(method string, params map[string]any) (map[string]any, error) {
	s.mu.Lock()
	s.nextID++
	id := s.nextID
	channel := make(chan map[string]any, 1)
	s.pending[id] = channel
	s.mu.Unlock()

	frame := map[string]any{"jsonrpc": "2.0", "id": id, "method": method}
	if params != nil {
		frame["params"] = params
	}
	if err := s.writeFrame(frame); err != nil {
		return nil, fmt.Errorf("写 MCP 帧 %s: %w", method, err)
	}
	select {
	case message := <-channel:
		return message, nil
	case <-s.done:
		return nil, fmt.Errorf("%s 响应前 stdout 关闭: %v", method, s.readError())
	case <-time.After(20 * time.Second):
		return nil, fmt.Errorf("等待 %s 响应超时", method)
	}
}

// request sends one request and fails the test on protocol errors.
func (s *stdioSession) request(t *testing.T, method string, params map[string]any) map[string]any {
	t.Helper()
	message, err := s.requestErr(method, params)
	if err != nil {
		t.Fatalf("%s: %v（stderr=%s）", method, err, s.stderr.String())
	}
	return message
}

// notify sends one notification without an id.
func (s *stdioSession) notify(t *testing.T, method string, params map[string]any) {
	t.Helper()
	frame := map[string]any{"jsonrpc": "2.0", "method": method}
	if params != nil {
		frame["params"] = params
	}
	if err := s.writeFrame(frame); err != nil {
		t.Fatalf("写通知 %s: %v", method, err)
	}
}

// initialize performs the MCP handshake.
func (s *stdioSession) initialize(t *testing.T) map[string]any {
	t.Helper()
	response := s.request(t, "initialize", map[string]any{
		"protocolVersion": "2025-06-18",
		"capabilities":    map[string]any{},
		"clientInfo":      map[string]any{"name": "dsh-mcp-e2e", "version": "1.0.0"},
	})
	result, ok := response["result"].(map[string]any)
	if !ok {
		t.Fatalf("initialize 失败: %v", response)
	}
	s.notify(t, "notifications/initialized", nil)
	return result
}

// callToolErr calls one MCP tool and returns its result object.
func (s *stdioSession) callToolErr(name string, arguments map[string]any) (map[string]any, error) {
	response, err := s.requestErr("tools/call", map[string]any{"name": name, "arguments": arguments})
	if err != nil {
		return nil, err
	}
	if rpcError, ok := response["error"]; ok {
		return nil, fmt.Errorf("tools/call %s 返回错误: %v", name, rpcError)
	}
	result, ok := response["result"].(map[string]any)
	if !ok {
		return nil, fmt.Errorf("tools/call %s 缺少 result: %v", name, response)
	}
	if isError, _ := result["isError"].(bool); isError {
		return nil, fmt.Errorf("tools/call %s isError: %v", name, result)
	}
	return result, nil
}

// shutdown closes stdin and waits for a normal exit.
func (s *stdioSession) shutdown(t *testing.T) int {
	t.Helper()
	_ = s.stdin.Close()
	select {
	case <-s.waitDone:
	case <-time.After(20 * time.Second):
		_ = s.command.Process.Kill()
		<-s.waitDone
		t.Fatalf("关闭 stdin 后进程未退出（stderr=%s）", s.stderr.String())
	}
	select {
	case <-s.done:
	case <-time.After(5 * time.Second):
		t.Fatalf("stdout 读取协程未结束")
	}
	if s.waitErr == nil {
		return 0
	}
	if exitError, ok := s.waitErr.(*exec.ExitError); ok {
		return exitError.ExitCode()
	}
	t.Fatalf("wait: %v", s.waitErr)
	return -1
}

// killIfRunning terminates a still-running child.
func (s *stdioSession) killIfRunning() {
	select {
	case <-s.waitDone:
		return
	default:
	}
	_ = s.command.Process.Kill()
	<-s.waitDone
}

// assertStdoutFrames verifies every captured stdout line is one JSON frame.
func (s *stdioSession) assertStdoutFrames(t *testing.T) {
	t.Helper()
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.lines) == 0 {
		t.Fatalf("stdout 没有任何 MCP 帧")
	}
	for _, line := range s.lines {
		if !json.Valid([]byte(line)) {
			t.Fatalf("stdout 出现非 MCP 帧: %q", line)
		}
	}
}

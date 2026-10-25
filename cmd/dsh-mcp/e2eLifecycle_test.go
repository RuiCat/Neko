package main

import (
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

// TestE2EHelpExitsZero checks bin/dsh-mcp -h exits 0 and documents every flag.
func TestE2EHelpExitsZero(t *testing.T) {
	output, err := exec.Command(serverBinary, "-h").CombinedOutput()
	if err != nil {
		t.Fatalf("-h exit: %v (%s)", err, output)
	}
	for _, flagName := range []string{"-data-dir", "-scripts-dir", "-config", "-control-socket"} {
		if !strings.Contains(string(output), flagName) {
			t.Fatalf("-h 缺少参数 %s：%s", flagName, output)
		}
	}
}

// TestE2ESigtermCleansSocket checks exit 0, 0600 socket mode and socket removal.
func TestE2ESigtermCleansSocket(t *testing.T) {
	process := startProcess(t)
	client := process.waitForHealth(t)
	info, err := os.Stat(process.socketPath)
	if err != nil {
		t.Fatalf("socket missing: %v", err)
	}
	if mode := info.Mode().Perm(); mode != 0o600 {
		t.Fatalf("socket mode = %o, want 600", mode)
	}
	if status, payload := mustJSON(t, client, http.MethodGet, "/health", nil); status != http.StatusOK {
		t.Fatalf("health status = %d (%v)", status, payload)
	}
	if err := process.command.Process.Signal(syscall.SIGTERM); err != nil {
		t.Fatalf("signal: %v", err)
	}
	if code := process.wait(t, 15*time.Second); code != EXIT_OK {
		t.Fatalf("exit code = %d, want %d (stderr=%s)", code, EXIT_OK, process.stderr.String())
	}
	if _, err := os.Stat(process.socketPath); !os.IsNotExist(err) {
		t.Fatalf("socket still exists after SIGTERM: %v", err)
	}
	if !strings.Contains(process.stderr.String(), "收到信号") {
		t.Fatalf("stderr = %s", process.stderr.String())
	}
}

// TestE2EDoubleInstanceExitsThree checks the second instance exits 3 with the
// owner pid and removal hint on stderr.
func TestE2EDoubleInstanceExitsThree(t *testing.T) {
	first := startProcess(t)
	first.waitForHealth(t)
	lockPath := filepath.Join(first.dataDir, ENGINE_LOCK_FILE)
	raw, err := os.ReadFile(lockPath)
	if err != nil {
		t.Fatalf("lock file: %v", err)
	}
	ownerPID := strconv.Itoa(first.command.Process.Pid)
	if strings.TrimSpace(string(raw)) != ownerPID {
		t.Fatalf("lock file = %q, want %q", raw, ownerPID)
	}

	second := exec.Command(serverBinary, "-data-dir", first.dataDir, "-scripts-dir", first.scriptsDir)
	stderr := &syncBuffer{}
	second.Stderr = stderr
	err = second.Run()
	exitError, ok := err.(*exec.ExitError)
	if !ok {
		t.Fatalf("second instance err = %v（stderr=%s）", err, stderr.String())
	}
	if code := exitError.ExitCode(); code != EXIT_ALREADY_RUNNING {
		t.Fatalf("second exit code = %d, want %d（stderr=%s）", code, EXIT_ALREADY_RUNNING, stderr.String())
	}
	for _, expected := range []string{"pid " + ownerPID, "rm "} {
		if !strings.Contains(stderr.String(), expected) {
			t.Fatalf("stderr 缺少 %q：%s", expected, stderr.String())
		}
	}
	// The first instance must stay healthy and still own the socket.
	client := newUnixClient(first.socketPath)
	if status, _ := mustJSON(t, client, http.MethodGet, "/health", nil); status != http.StatusOK {
		t.Fatalf("first instance unhealthy after second attempt: %d", status)
	}
}

// TestE2EKillNineSelfHeals checks a SIGKILLed instance leaves a stale socket and
// the next start unlinks it and serves again.
func TestE2EKillNineSelfHeals(t *testing.T) {
	first := startProcess(t)
	first.waitForHealth(t)
	if err := first.command.Process.Kill(); err != nil {
		t.Fatalf("kill: %v", err)
	}
	first.wait(t, 10*time.Second)
	if _, err := os.Stat(first.socketPath); err != nil {
		t.Fatalf("stale socket missing after SIGKILL: %v", err)
	}

	second := startProcessWithDirs(t, first.dataDir, first.scriptsDir)
	second.waitForHealth(t)
	info, err := os.Stat(second.socketPath)
	if err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("自愈后的 socket 权限异常: %v %v", info, err)
	}
	if err := second.command.Process.Signal(syscall.SIGTERM); err != nil {
		t.Fatalf("signal: %v", err)
	}
	if code := second.wait(t, 15*time.Second); code != EXIT_OK {
		t.Fatalf("restarted exit code = %d (stderr=%s)", code, second.stderr.String())
	}
	if _, err := os.Stat(second.socketPath); !os.IsNotExist(err) {
		t.Fatalf("socket still exists after restart shutdown: %v", err)
	}
}

// TestE2EEnvDirDefaults checks ASTRBOX_DATA_DIR / ASTRBOX_SCRIPTS_DIR defaults.
func TestE2EEnvDirDefaults(t *testing.T) {
	dataDir := t.TempDir()
	scriptsDir := t.TempDir()
	process := startProcessEnv(t, dataDir, scriptsDir)
	client := process.waitForHealth(t)
	status, payload := mustJSON(t, client, http.MethodGet, "/health", nil)
	if status != http.StatusOK {
		t.Fatalf("health status = %d", status)
	}
	if payload["dataDir"] != dataDir || payload["scriptsDir"] != scriptsDir {
		t.Fatalf("dirs = %v / %v, want %s / %s", payload["dataDir"], payload["scriptsDir"], dataDir, scriptsDir)
	}
	if err := process.command.Process.Signal(syscall.SIGTERM); err != nil {
		t.Fatalf("signal: %v", err)
	}
	if code := process.wait(t, 15*time.Second); code != EXIT_OK {
		t.Fatalf("exit code = %d (stderr=%s)", code, process.stderr.String())
	}
}

// TestE2ECustomControlSocket checks -control-socket overrides the default path.
func TestE2ECustomControlSocket(t *testing.T) {
	customPath := filepath.Join(t.TempDir(), "custom-control.sock")
	process := startProcess(t, "-control-socket", customPath)
	process.socketPath = customPath
	process.waitForHealth(t)
	if _, err := os.Stat(filepath.Join(process.dataDir, "dsh-control.sock")); !os.IsNotExist(err) {
		t.Fatalf("默认 socket 不应存在: %v", err)
	}
	if err := process.command.Process.Signal(syscall.SIGTERM); err != nil {
		t.Fatalf("signal: %v", err)
	}
	if code := process.wait(t, 15*time.Second); code != EXIT_OK {
		t.Fatalf("exit code = %d（stderr=%s）", code, process.stderr.String())
	}
	if _, err := os.Stat(customPath); !os.IsNotExist(err) {
		t.Fatalf("自定义 socket 未清理: %v", err)
	}
}

// TestE2EBadConfigExitsTwo checks a config load failure exits 2.
func TestE2EBadConfigExitsTwo(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(configPath, []byte(`{"state_group":""}`), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}
	command := exec.Command(serverBinary, "-data-dir", t.TempDir(), "-scripts-dir", t.TempDir(),
		"-config", configPath)
	stderr := &syncBuffer{}
	command.Stderr = stderr
	err := command.Run()
	exitError, ok := err.(*exec.ExitError)
	if !ok {
		t.Fatalf("err = %v（stderr=%s）", err, stderr.String())
	}
	if code := exitError.ExitCode(); code != EXIT_LOAD_FAILED {
		t.Fatalf("exit code = %d, want %d（stderr=%s）", code, EXIT_LOAD_FAILED, stderr.String())
	}
	if !strings.Contains(stderr.String(), "运行时装配失败") {
		t.Fatalf("stderr = %s", stderr.String())
	}
}

// TestE2EUnbindableSocketExitsFour checks a socket path that cannot be bound.
func TestE2EUnbindableSocketExitsFour(t *testing.T) {
	missingParent := filepath.Join(t.TempDir(), "missing", "dsh-control.sock")
	command := exec.Command(serverBinary, "-data-dir", t.TempDir(), "-scripts-dir", t.TempDir(),
		"-control-socket", missingParent)
	stderr := &syncBuffer{}
	command.Stderr = stderr
	err := command.Run()
	exitError, ok := err.(*exec.ExitError)
	if !ok {
		t.Fatalf("err = %v（stderr=%s）", err, stderr.String())
	}
	if code := exitError.ExitCode(); code != EXIT_SOCKET_FAILED {
		t.Fatalf("exit code = %d, want %d（stderr=%s）", code, EXIT_SOCKET_FAILED, stderr.String())
	}
	if !strings.Contains(stderr.String(), "无法建立控制面 socket") {
		t.Fatalf("stderr = %s", stderr.String())
	}
}

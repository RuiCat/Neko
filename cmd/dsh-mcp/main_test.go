package main

import (
	"errors"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestDefaultDirsFollowEnv checks the ASTRBOX_* flag defaults.
func TestDefaultDirsFollowEnv(t *testing.T) {
	t.Setenv("ASTRBOX_DATA_DIR", "/tmp/astrbox-data")
	t.Setenv("ASTRBOX_SCRIPTS_DIR", "/tmp/astrbox-scripts")
	if got := defaultDataDir(); got != "/tmp/astrbox-data" {
		t.Fatalf("defaultDataDir = %q", got)
	}
	if got := defaultScriptsDir(); got != "/tmp/astrbox-scripts" {
		t.Fatalf("defaultScriptsDir = %q", got)
	}
	t.Setenv("ASTRBOX_DATA_DIR", "")
	t.Setenv("ASTRBOX_SCRIPTS_DIR", "")
	if got := defaultDataDir(); got != "data" {
		t.Fatalf("defaultDataDir fallback = %q", got)
	}
	if got := defaultScriptsDir(); got != "scripts" {
		t.Fatalf("defaultScriptsDir fallback = %q", got)
	}
}

// TestAcquireEngineLockIsExclusive checks flock exclusivity and the owner pid.
func TestAcquireEngineLockIsExclusive(t *testing.T) {
	dataDir := t.TempDir()
	first, err := acquireEngineLock(dataDir)
	if err != nil {
		t.Fatalf("first lock: %v", err)
	}
	lockPath := filepath.Join(dataDir, ENGINE_LOCK_FILE)
	if pid := readLockPID(lockPath); pid != os.Getpid() {
		t.Fatalf("lock pid = %d, want %d", pid, os.Getpid())
	}

	second, err := acquireEngineLock(dataDir)
	var held *lockHeldError
	if !errors.As(err, &held) {
		t.Fatalf("second lock err = %v (lock=%v)", err, second)
	}
	if held.PID != os.Getpid() {
		t.Fatalf("held pid = %d", held.PID)
	}
	if !strings.Contains(held.Error(), "pid") {
		t.Fatalf("error text = %q", held.Error())
	}

	if err := first.Release(); err != nil {
		t.Fatalf("release: %v", err)
	}
	third, err := acquireEngineLock(dataDir)
	if err != nil {
		t.Fatalf("reacquire after release: %v", err)
	}
	if err := third.Release(); err != nil {
		t.Fatalf("third release: %v", err)
	}
}

// TestOpenControlListenerReusesStaleSocket checks the connect probe and unlink.
func TestOpenControlListenerReusesStaleSocket(t *testing.T) {
	socketPath := filepath.Join(t.TempDir(), "dsh-control.sock")
	first, err := openControlListener(socketPath)
	if err != nil {
		t.Fatalf("first listener: %v", err)
	}
	unixListener, ok := first.(*net.UnixListener)
	if !ok {
		t.Fatalf("listener type = %T", first)
	}

	if _, err := openControlListener(socketPath); !errors.Is(err, errSocketInUse) {
		t.Fatalf("live socket err = %v", err)
	}

	unixListener.SetUnlinkOnClose(false)
	if err := first.Close(); err != nil {
		t.Fatalf("close first: %v", err)
	}
	if _, err := os.Stat(socketPath); err != nil {
		t.Fatalf("stale socket should remain: %v", err)
	}

	third, err := openControlListener(socketPath)
	if err != nil {
		t.Fatalf("stale socket not self-healed: %v", err)
	}
	if err := third.Close(); err != nil {
		t.Fatalf("close third: %v", err)
	}
	_ = os.Remove(socketPath)
}

// TestOpenControlListenerRejectsLongPath checks the 108-byte AF_UNIX guard.
func TestOpenControlListenerRejectsLongPath(t *testing.T) {
	longPath := filepath.Join(t.TempDir(), strings.Repeat("s", 120))
	if _, err := openControlListener(longPath); err == nil {
		t.Fatalf("long socket path accepted: %s", longPath)
	} else if errors.Is(err, errSocketInUse) {
		t.Fatalf("long socket path reported as busy: %v", err)
	}
}

// TestRunHelpAndBadFlag checks the flag-driven exit codes.
func TestRunHelpAndBadFlag(t *testing.T) {
	if code := run([]string{"-h"}); code != EXIT_OK {
		t.Fatalf("-h exit code = %d", code)
	}
	if code := run([]string{"-no-such-flag"}); code != EXIT_LOAD_FAILED {
		t.Fatalf("bad flag exit code = %d", code)
	}
	if code := run([]string{"-data-dir", ""}); code != EXIT_LOAD_FAILED {
		t.Fatalf("empty data-dir exit code = %d", code)
	}
}

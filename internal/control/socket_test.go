package control

import (
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestListenAppliesSocketPermission checks the explicit 0600 socket mode.
func TestListenAppliesSocketPermission(t *testing.T) {
	socketPath := filepath.Join(t.TempDir(), "dsh-control.sock")
	listener, err := Listen(socketPath)
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer func() {
		_ = listener.Close()
		_ = os.Remove(socketPath)
	}()
	info, err := os.Stat(socketPath)
	if err != nil {
		t.Fatalf("stat socket: %v", err)
	}
	if mode := info.Mode().Perm(); mode != SOCKET_MODE {
		t.Fatalf("socket mode = %o, want %o", mode, SOCKET_MODE)
	}
	connection, err := net.DialTimeout("unix", socketPath, time.Second)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	_ = connection.Close()
}

// TestListenRejectsLongPath keeps the AF_UNIX path limit explicit.
func TestListenRejectsLongPath(t *testing.T) {
	longPath := filepath.Join(t.TempDir(), strings.Repeat("x", MAX_SOCKET_PATH_LENGTH))
	if len(longPath) <= MAX_SOCKET_PATH_LENGTH {
		t.Fatalf("test path too short: %d", len(longPath))
	}
	if _, err := Listen(longPath); err == nil {
		t.Fatalf("long socket path accepted: %s", longPath)
	}
}

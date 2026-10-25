// Control-socket preparation: probe a stale socket, unlink it, bind once.
package main

import (
	"errors"
	"fmt"
	"io/fs"
	"net"
	"os"
	"syscall"
	"time"

	"github.com/ruicat/astrbox/internal/control"
)

const (
	// SOCKET_PROBE_TIMEOUT bounds the stale-socket connect probe.
	SOCKET_PROBE_TIMEOUT = 500 * time.Millisecond
	// SOCKET_BIND_ATTEMPTS is one bind plus the single retry after unlink.
	SOCKET_BIND_ATTEMPTS = 2
)

// errSocketInUse means an existing control socket accepted a connection.
var errSocketInUse = errors.New("控制面 socket 已被占用")

// openControlListener applies the K3 startup rule: a connectable socket means
// another instance is alive; ECONNREFUSED (or a missing path) means stale, so
// unlink it and retry the bind once.
func openControlListener(socketPath string) (net.Listener, error) {
	if conn, err := net.DialTimeout("unix", socketPath, SOCKET_PROBE_TIMEOUT); err == nil {
		_ = conn.Close()
		return nil, errSocketInUse
	} else if !errors.Is(err, syscall.ECONNREFUSED) && !errors.Is(err, fs.ErrNotExist) {
		return nil, fmt.Errorf("探测已有 socket 失败: %w", err)
	}

	var lastErr error
	for attempt := 0; attempt < SOCKET_BIND_ATTEMPTS; attempt++ {
		if err := os.Remove(socketPath); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return nil, fmt.Errorf("清理残留 socket %s: %w", socketPath, err)
		}
		listener, err := control.Listen(socketPath)
		if err == nil {
			return listener, nil
		}
		lastErr = err
	}
	return nil, lastErr
}

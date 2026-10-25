// Unix-socket listener creation with the 0600 control-plane permission.
package control

import (
	"fmt"
	"net"
	"os"
)

const (
	// MAX_SOCKET_PATH_LENGTH keeps the path under the 108-byte AF_UNIX limit.
	MAX_SOCKET_PATH_LENGTH = 100
	// SOCKET_MODE restricts the control socket to the owning user.
	SOCKET_MODE = 0o600
)

// Listen binds the control-plane Unix socket and explicitly applies 0600.
func Listen(socketPath string) (net.Listener, error) {
	if socketPath == "" {
		return nil, fmt.Errorf("控制面 socket 路径为空")
	}
	if len(socketPath) > MAX_SOCKET_PATH_LENGTH {
		return nil, fmt.Errorf("控制面 socket 路径过长（%d 字节，上限 %d）：%s",
			len(socketPath), MAX_SOCKET_PATH_LENGTH, socketPath)
	}
	listener, err := net.Listen("unix", socketPath)
	if err != nil {
		return nil, err
	}
	if err := os.Chmod(socketPath, SOCKET_MODE); err != nil {
		_ = listener.Close()
		_ = os.Remove(socketPath)
		return nil, fmt.Errorf("设置 socket 权限: %w", err)
	}
	return listener, nil
}

// Single-instance flock over <data-dir>/.engine.lock, recording the owner pid.
package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
)

// ENGINE_LOCK_FILE is the per-data-directory single-instance lock file.
const ENGINE_LOCK_FILE = ".engine.lock"

// lockHeldError reports that another process owns the data directory.
type lockHeldError struct {
	Path string
	PID  int
}

// Error names the lock file and the recorded owner pid.
func (e *lockHeldError) Error() string {
	return fmt.Sprintf("单实例锁 %s 已被占用（pid %d）", e.Path, e.PID)
}

// engineLock owns the open file description holding the flock.
type engineLock struct {
	file *os.File
}

// acquireEngineLock takes a non-blocking exclusive flock and records this
// process pid in the lock file.
func acquireEngineLock(dataDir string) (*engineLock, error) {
	path := filepath.Join(dataDir, ENGINE_LOCK_FILE)
	file, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		return nil, fmt.Errorf("打开单实例锁 %s: %w", path, err)
	}
	if err := syscall.Flock(int(file.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		pid := readLockPID(path)
		_ = file.Close()
		if errors.Is(err, syscall.EWOULDBLOCK) || errors.Is(err, syscall.EAGAIN) {
			return nil, &lockHeldError{Path: path, PID: pid}
		}
		return nil, fmt.Errorf("加锁 %s: %w", path, err)
	}
	if err := writeLockPID(file); err != nil {
		_ = syscall.Flock(int(file.Fd()), syscall.LOCK_UN)
		_ = file.Close()
		return nil, fmt.Errorf("记录锁持有者 pid: %w", err)
	}
	return &engineLock{file: file}, nil
}

// Release unlocks and closes the lock file; the file itself stays for the pid.
func (l *engineLock) Release() error {
	if l == nil || l.file == nil {
		return nil
	}
	unlockErr := syscall.Flock(int(l.file.Fd()), syscall.LOCK_UN)
	closeErr := l.file.Close()
	l.file = nil
	if unlockErr != nil {
		return unlockErr
	}
	return closeErr
}

// readLockPID reads the pid recorded by the current owner (0 when unknown).
func readLockPID(path string) int {
	raw, err := os.ReadFile(path)
	if err != nil {
		return 0
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(raw)))
	if err != nil {
		return 0
	}
	return pid
}

// writeLockPID truncates the lock file and writes this process pid.
func writeLockPID(file *os.File) error {
	if err := file.Truncate(0); err != nil {
		return err
	}
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return err
	}
	_, err := file.WriteString(strconv.Itoa(os.Getpid()) + "\n")
	return err
}

//go:build windows

package serve

import (
	"fmt"
	"os"
	"time"

	"golang.org/x/sys/windows"
)

// acquireFileLockTimeout tries to acquire an exclusive lock with exponential
// backoff up to the given timeout.
func acquireFileLockTimeout(f *os.File, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	backoff := 5 * time.Millisecond
	maxBackoff := 50 * time.Millisecond

	for {
		ol := new(windows.Overlapped)
		err := windows.LockFileEx(
			windows.Handle(f.Fd()),
			windows.LOCKFILE_EXCLUSIVE_LOCK|windows.LOCKFILE_FAIL_IMMEDIATELY,
			0, 1, 0, ol,
		)
		if err == nil {
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("timeout after %v waiting for port file lock", timeout)
		}
		time.Sleep(backoff)
		if backoff < maxBackoff {
			backoff *= 2
			if backoff > maxBackoff {
				backoff = maxBackoff
			}
		}
	}
}

// releaseFileLock releases the exclusive lock.
func releaseFileLock(f *os.File) {
	if f != nil {
		ol := new(windows.Overlapped)
		windows.UnlockFileEx(windows.Handle(f.Fd()), 0, 1, 0, ol)
	}
}

// isProcessAlive checks if a process with the given PID is still running.
func isProcessAlive(pid int) bool {
	handle, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, uint32(pid))
	if err != nil {
		return false
	}
	defer windows.CloseHandle(handle)

	var exitCode uint32
	err = windows.GetExitCodeProcess(handle, &exitCode)
	if err != nil {
		return false
	}
	// STILL_ACTIVE (259) means process is running
	return exitCode == 259
}

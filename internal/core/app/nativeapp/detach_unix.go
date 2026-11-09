//go:build unix

package nativeapp

import (
	"errors"
	"syscall"
	"time"
)

// detachAttr puts the binary in its own process group so it survives the gtool
// process exit.
func detachAttr() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{Setpgid: true}
}

func groupAlive(pid int) bool {
	return syscall.Kill(-pid, 0) == nil
}

// stopGroup terminates the process group led by pid (TERM, then KILL) and
// waits for it to be gone; a group that no longer exists is not an error.
func stopGroup(pid int) error {
	if err := syscall.Kill(-pid, syscall.SIGTERM); err != nil {
		if errors.Is(err, syscall.ESRCH) {
			return nil
		}
		return err
	}
	for deadline := time.Now().Add(stopTimeout); time.Now().Before(deadline); time.Sleep(stopPoll) {
		if !groupAlive(pid) {
			return nil
		}
	}
	if err := syscall.Kill(-pid, syscall.SIGKILL); err != nil && !errors.Is(err, syscall.ESRCH) {
		return err
	}
	return nil
}

//go:build unix

package process

import (
	"errors"
	"syscall"
)

// processGroupAttr makes the child lead its own process group, so a signal to
// the group reaches every process it spawns (e.g. through a shell wrapper).
func processGroupAttr() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{Setpgid: true}
}

func signalGroup(pid int, sig syscall.Signal) error {
	if err := syscall.Kill(-pid, sig); err != nil && !errors.Is(err, syscall.ESRCH) {
		return err
	}
	return nil
}

func terminateGroup(pid int) error { return signalGroup(pid, syscall.SIGTERM) }

func killGroup(pid int) error { return signalGroup(pid, syscall.SIGKILL) }

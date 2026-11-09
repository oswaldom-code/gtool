//go:build windows

package process

import (
	"errors"
	"os"
	"syscall"
)

// Windows has no POSIX process groups: only the process itself is signalled.
func processGroupAttr() *syscall.SysProcAttr { return nil }

func killProcess(pid int) error {
	p, err := os.FindProcess(pid)
	if err != nil {
		return nil
	}
	if err := p.Kill(); err != nil && !errors.Is(err, os.ErrProcessDone) {
		return err
	}
	return nil
}

func terminateGroup(pid int) error { return killProcess(pid) }

func killGroup(pid int) error { return killProcess(pid) }

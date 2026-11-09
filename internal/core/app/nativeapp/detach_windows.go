//go:build windows

package nativeapp

import (
	"errors"
	"os"
	"syscall"
)

// The native launcher relies on pkill/pgrep and process groups;
// on Windows it builds but is not supported.
func detachAttr() *syscall.SysProcAttr { return nil }

func stopGroup(pid int) error {
	p, err := os.FindProcess(pid)
	if err != nil {
		return nil
	}
	if err := p.Kill(); err != nil && !errors.Is(err, os.ErrProcessDone) {
		return err
	}
	return nil
}

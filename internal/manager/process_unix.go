//go:build !windows

package manager

import (
	"errors"
	"os"
	"os/exec"
	"syscall"
)

func startDetached(cmd *exec.Cmd) error {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	return cmd.Start()
}

func processAlive(pid int) bool {
	process, err := os.FindProcess(pid)
	return err == nil && process.Signal(syscall.Signal(0)) == nil
}

func stopProcess(pid int) error {
	process, err := os.FindProcess(pid)
	if err != nil {
		return err
	}
	err = process.Signal(syscall.SIGTERM)
	if errors.Is(err, os.ErrProcessDone) {
		return nil
	}
	return err
}

func killProcess(pid int) error {
	process, err := os.FindProcess(pid)
	if err != nil {
		return err
	}
	return process.Kill()
}

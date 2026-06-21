//go:build windows

package manager

import (
	"os"
	"os/exec"
)

func startDetached(cmd *exec.Cmd) error { return cmd.Start() }

func processAlive(pid int) bool {
	process, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	// Windows does not support signal 0. The panel uses persisted runtime state in production on Linux.
	return process != nil
}

func stopProcess(pid int) error { return killProcess(pid) }

func killProcess(pid int) error {
	process, err := os.FindProcess(pid)
	if err != nil {
		return err
	}
	return process.Kill()
}

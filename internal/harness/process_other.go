//go:build !darwin && !linux && !freebsd && !openbsd && !netbsd && !dragonfly

package harness

import (
	"os"
	"os/exec"
)

func configureProcess(cmd *exec.Cmd) {}
func ProcessAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	process, err := os.FindProcess(pid)
	return err == nil && process.Signal(os.Signal(nil)) == nil
}

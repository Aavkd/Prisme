//go:build !linux

package backup

import (
	"os"
	"os/exec"
)

func configureProcess(c *exec.Cmd) {}

// The worker requires Linux; Windows only validates configuration and unit tests.
func checkPrivate(s os.FileInfo) error { return nil }

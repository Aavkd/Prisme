package backup

import (
	"os"
	"os/exec"
	"syscall"
)

func configureProcess(c *exec.Cmd) {
	c.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	c.Cancel = func() error {
		if c.Process == nil {
			return os.ErrProcessDone
		}
		return syscall.Kill(-c.Process.Pid, syscall.SIGKILL)
	}
}
func checkPrivate(s os.FileInfo) error {
	if s.Mode().Perm()&0077 != 0 {
		return ErrInvalid
	}
	return nil
}

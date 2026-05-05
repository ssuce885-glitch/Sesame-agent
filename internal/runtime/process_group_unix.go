//go:build unix

package runtime

import (
	"errors"
	"os"
	"os/exec"
	"syscall"
	"time"
)

const processGroupWaitDelay = 2 * time.Second

func configureCommandCancellation(cmd *exec.Cmd) {
	if cmd == nil {
		return
	}
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	cmd.SysProcAttr.Setpgid = true
	cmd.WaitDelay = processGroupWaitDelay
	cmd.Cancel = func() error {
		if cmd.Process == nil {
			return nil
		}

		var killErr error
		pgid, err := syscall.Getpgid(cmd.Process.Pid)
		if err == nil {
			killErr = syscall.Kill(-pgid, syscall.SIGKILL)
			if killErr == nil || errors.Is(killErr, syscall.ESRCH) {
				return nil
			}
		}

		procErr := cmd.Process.Kill()
		if procErr == nil || errors.Is(procErr, os.ErrProcessDone) {
			if err == nil || errors.Is(err, syscall.ESRCH) {
				return nil
			}
			return err
		}
		if err == nil || errors.Is(err, syscall.ESRCH) {
			return procErr
		}
		if killErr == nil || errors.Is(killErr, syscall.ESRCH) {
			return errors.Join(err, procErr)
		}
		return errors.Join(err, killErr, procErr)
	}
}

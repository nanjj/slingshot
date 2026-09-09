//go:build unix

package main

import (
	"errors"
	"os"
	"os/exec"
	"syscall"
)

// setProcessGroup 让子进程成为新进程组的组长, 使超时时可以整组杀掉。
// LaTeX 工具链是进程树 (latexmk → xelatex → xdvipdfmx), 只杀直接子进程会
// 留下孤儿继续占用 CPU 并持有 stdout 管道。
func setProcessGroup(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}

// killProcessGroup 向 cmd 所在的整个进程组发 SIGKILL。
// 进程已退出时返回 os.ErrProcessDone (os/exec 视其为正常取消)。
func killProcessGroup(cmd *exec.Cmd) error {
	if cmd.Process == nil {
		return os.ErrProcessDone
	}
	if err := syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL); err != nil {
		if errors.Is(err, syscall.ESRCH) {
			return os.ErrProcessDone
		}
		return err
	}
	return nil
}

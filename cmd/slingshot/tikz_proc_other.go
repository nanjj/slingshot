//go:build !unix

package main

import (
	"os"
	"os/exec"
)

// setProcessGroup 在非 Unix 平台 (Windows) 上没有进程组语义, 不做处理。
func setProcessGroup(cmd *exec.Cmd) {}

// killProcessGroup 退化为杀掉直接子进程。
func killProcessGroup(cmd *exec.Cmd) error {
	if cmd.Process == nil {
		return os.ErrProcessDone
	}
	return cmd.Process.Kill()
}

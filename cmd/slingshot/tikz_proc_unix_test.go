//go:build unix

package main

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

// TestRunCmdTimeout 钉住 runCmd 的超时行为: ctx 到期后必须很快返回
// context.DeadlineExceeded, 而不是等被挂住的命令自己退出。
func TestRunCmdTimeout(t *testing.T) {
	if _, err := exec.LookPath("sleep"); err != nil {
		t.Skipf("sleep unavailable: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	start := time.Now()
	err := runCmd(ctx, t.TempDir(), "sleep", "30")
	elapsed := time.Since(start)

	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("runCmd() error = %v, want context.DeadlineExceeded", err)
	}
	if elapsed > 10*time.Second {
		t.Fatalf("runCmd() returned after %s, want it to stop shortly after the 200ms deadline", elapsed)
	}
	if !strings.Contains(err.Error(), "TIKZ_TIMEOUT") {
		t.Errorf("runCmd() error = %v, want it to mention the TIKZ_TIMEOUT knob", err)
	}
}

// TestRunCmdCancelKillsProcessGroup 钉住进程组语义: 取消后整棵进程树
// (sh → sleep) 都必须消失。只杀直接子进程的实现会留下 sleep 孤儿, 正是
// latexmk → xelatex 在超时场景下的故障模式。
func TestRunCmdCancelKillsProcessGroup(t *testing.T) {
	if _, err := exec.LookPath("sh"); err != nil {
		t.Skipf("sh unavailable: %v", err)
	}
	dir := t.TempDir()
	pidFile := filepath.Join(dir, "child.pid")

	// 用 cancel 而不是超时来触发清理: 等 sh 写好孙进程 pid 再取消, 避免与
	// 进程启动竞态 (两条路径走的是同一段 cmd.Cancel 代码)。
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() {
		done <- runCmd(ctx, dir, "sh", "-c", "sleep 30 & echo $! > child.pid; wait")
	}()

	pid := waitForPIDFile(t, pidFile)
	cancel()
	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Fatalf("runCmd() error = %v, want context.Canceled", err)
	}

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if killErr := syscall.Kill(pid, 0); errors.Is(killErr, syscall.ESRCH) {
			return // 进程组已整体清理
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("grandchild pid %d survived cancellation: process group was not killed", pid)
}

// waitForPIDFile 等待 pidFile 出现并返回其中的 pid。
func waitForPIDFile(t *testing.T, pidFile string) int {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		data, err := os.ReadFile(pidFile)
		if err == nil {
			pid, convErr := strconv.Atoi(strings.TrimSpace(string(data)))
			if convErr != nil {
				t.Fatalf("parsing grandchild pid %q: %v", data, convErr)
			}
			return pid
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("%s did not appear", pidFile)
	return 0
}

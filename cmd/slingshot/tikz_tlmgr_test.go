package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// resetUserModeCache 清空包级缓存, 让测试从干净状态开始。
func resetUserModeCache(t *testing.T) {
	t.Helper()

	old := userModePackageCache
	userModePackageCache = map[string]bool{}

	t.Cleanup(func() { userModePackageCache = old })
}

// TestEnsureUserModePackageKpsewhichGate 钉住快速路径: kpsewhich 命中时不启动
// tlmgr。tlmgr 每次启动要数秒, 是 `slingshot tikz` 从 1.3s 变 6-17s 的原因。
func TestEnsureUserModePackageKpsewhichGate(t *testing.T) {
	resetUserModeCache(t)

	oldKps := kpsewhichHasFn
	t.Cleanup(func() { kpsewhichHasFn = oldKps })

	// PATH 里没有 tlmgr: 只要代码试图调用它就会失败。
	t.Setenv("PATH", t.TempDir())

	calls := 0
	kpsewhichHasFn = func(file string) bool {
		calls++
		if file != "physketch.sty" {
			t.Errorf("kpsewhichHasFn(%q), want physketch.sty", file)
		}

		return true
	}

	if err := ensureUserModePackage("physketch.general"); err != nil {
		t.Fatalf("ensureUserModePackage() = %v, want nil when kpsewhich finds the file", err)
	}

	if calls != 1 {
		t.Fatalf("kpsewhich consulted %d times, want 1", calls)
	}

	if !userModePackageCache["physketch"] {
		t.Fatal("package not cached after kpsewhich hit")
	}
}

// TestEnsureUserModePackageMissingFileNeedsTlmgr 确认慢路径仍然存在: kpsewhich
// 说找不到、PATH 里也没有 tlmgr 时, 必须报出可读的错误而不是静默跳过。
func TestEnsureUserModePackageMissingFileNeedsTlmgr(t *testing.T) {
	resetUserModeCache(t)

	oldKps := kpsewhichHasFn
	t.Cleanup(func() { kpsewhichHasFn = oldKps })
	kpsewhichHasFn = func(string) bool { return false }

	t.Setenv("PATH", t.TempDir()) // no tlmgr

	err := ensureUserModePackage("somepkg")
	if err == nil || !strings.Contains(err.Error(), "tlmgr not found") {
		t.Fatalf("ensureUserModePackage() = %v, want a tlmgr-not-found error", err)
	}
}

// TestEnsureUserModePackagesBatches 钉住批量快速路径: N 个包只发一次 kpsewhich,
// 全部命中时不逐个探测、不碰 tlmgr。
func TestEnsureUserModePackagesBatches(t *testing.T) {
	resetUserModeCache(t)

	oldKps := kpsewhichHasFn
	t.Cleanup(func() { kpsewhichHasFn = oldKps })
	kpsewhichHasFn = func(string) bool {
		t.Error("per-package kpsewhichHasFn called on the batch fast path")

		return true
	}

	oldCount := kpsewhichCountFn
	t.Cleanup(func() { kpsewhichCountFn = oldCount })

	batchCalls := 0
	kpsewhichCountFn = func(files []string) int {
		batchCalls++
		want := []string{"physketch.sty", "circuitikz.sty", "siunitx.sty"}
		if len(files) != len(want) {
			t.Fatalf("kpsewhichCountFn(%v), want %v", files, want)
		}

		for i := range want {
			if files[i] != want[i] {
				t.Fatalf("kpsewhichCountFn(%v), want %v", files, want)
			}
		}

		return len(files)
	}

	t.Setenv("PATH", t.TempDir()) // no tlmgr; must not be reached

	pkgs := []string{"physketch.general", "physketch.circuit", "circuitikz", "siunitx"}
	if err := ensureUserModePackages(pkgs); err != nil {
		t.Fatalf("ensureUserModePackages() = %v, want nil", err)
	}

	if batchCalls != 1 {
		t.Fatalf("batched kpsewhich called %d times, want 1", batchCalls)
	}

	for _, pkg := range []string{"physketch", "circuitikz", "siunitx"} {
		if !userModePackageCache[pkg] {
			t.Errorf("package %q not cached", pkg)
		}
	}
}

// TestEnsureUserModePackagesFallsBackWhenMissing 确认批量判断不全命中时会退回
// 逐个处理 (这里只验证它走到 ensureUserModePackage 并因缺 tlmgr 而报错)。
func TestEnsureUserModePackagesFallsBackWhenMissing(t *testing.T) {
	resetUserModeCache(t)

	oldKps := kpsewhichHasFn
	t.Cleanup(func() { kpsewhichHasFn = oldKps })
	kpsewhichHasFn = func(string) bool { return false }

	oldCount := kpsewhichCountFn
	t.Cleanup(func() { kpsewhichCountFn = oldCount })
	kpsewhichCountFn = func(files []string) int { return len(files) - 1 }

	t.Setenv("PATH", t.TempDir()) // no tlmgr

	err := ensureUserModePackages([]string{"somepkg"})
	if err == nil || !strings.Contains(err.Error(), "tlmgr not found") {
		t.Fatalf("ensureUserModePackages() = %v, want a tlmgr-not-found error", err)
	}
}

// TestEnsureUserModePackageInstallsWhenMissing covers the slow path: kpsewhich
// says the style is absent, tlmgr says it is not installed, so tlmgr installs
// it. That branch is what actually gets a missing package onto the machine,
// and a mistake there turns into "the package never appears" rather than a
// clear error.
//
// tlmgr is replaced by a script on PATH, the same technique
// TestRenderTikzEngineExit0WithoutPDF uses for latexmk.
func TestEnsureUserModePackageInstallsWhenMissing(t *testing.T) {
	resetUserModeCache(t)

	oldKps := kpsewhichHasFn
	t.Cleanup(func() { kpsewhichHasFn = oldKps })
	kpsewhichHasFn = func(string) bool { return false }

	dir := t.TempDir()
	log := filepath.Join(dir, "tlmgr.log")
	// tlmgr is invoked as "tlmgr --usermode <subcommand> <pkg>". One shift
	// leaves the subcommand in $1; shifting twice would put the package name
	// there instead, neither case would match, and the script would report
	// success for everything.
	script := "#!/bin/sh\n" +
		"echo \"$@\" >> " + log + "\n" +
		"shift\n" +
		"case \"$1\" in\n" +
		"  info) exit 1 ;;\n" + // not installed yet
		"  install) exit 0 ;;\n" +
		"esac\n" +
		"exit 0\n"

	tlmgr := filepath.Join(dir, "tlmgr")
	if err := os.WriteFile(tlmgr, []byte(script), 0755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))

	if err := ensureUserModePackage("somepkg"); err != nil {
		t.Fatalf("ensureUserModePackage() = %v, want nil", err)
	}

	calls, err := os.ReadFile(log)
	if err != nil {
		t.Fatal(err)
	}
	got := string(calls)
	for _, want := range []string{"--usermode info somepkg", "--usermode install somepkg"} {
		if !strings.Contains(got, want) {
			t.Errorf("tlmgr was not called with %q; log:\n%s", want, got)
		}
	}
	if !userModePackageCache["somepkg"] {
		t.Error("package not cached after a successful install")
	}
}

// TestEnsureUserModePackageInstallFailure covers the error path: tlmgr install
// fails, and the message must name the package and include tlmgr's output so
// the user can see why.
func TestEnsureUserModePackageInstallFailure(t *testing.T) {
	resetUserModeCache(t)

	oldKps := kpsewhichHasFn
	t.Cleanup(func() { kpsewhichHasFn = oldKps })
	kpsewhichHasFn = func(string) bool { return false }

	dir := t.TempDir()
	script := "#!/bin/sh\n" +
		"case \"$2\" in\n" +
		"  info) exit 1 ;;\n" +
		"  install) echo 'cannot find package' >&2; exit 1 ;;\n" +
		"esac\n" +
		"exit 0\n"
	if err := os.WriteFile(filepath.Join(dir, "tlmgr"), []byte(script), 0755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))

	err := ensureUserModePackage("somepkg")
	if err == nil {
		t.Fatal("ensureUserModePackage() = nil, want an install failure")
	}
	for _, want := range []string{"somepkg", "cannot find package"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q does not mention %q", err, want)
		}
	}
	if userModePackageCache["somepkg"] {
		t.Error("failed package was cached as installed")
	}
}

// TestEnsureUserModePackageTlmgrInfoSaysInstalled covers the branch where
// kpsewhich cannot see the file but tlmgr reports the package as installed.
// That happens when a package lives in a tree kpsewhich does not search, and
// the code asks tlmgr before installing rather than reinstalling blindly.
func TestEnsureUserModePackageTlmgrInfoSaysInstalled(t *testing.T) {
	resetUserModeCache(t)

	oldKps := kpsewhichHasFn
	t.Cleanup(func() { kpsewhichHasFn = oldKps })
	kpsewhichHasFn = func(string) bool { return false }

	dir := t.TempDir()
	log := filepath.Join(dir, "tlmgr.log")

	// info succeeds, so install must never be reached.
	script := "#!/bin/sh\n" +
		"echo \"$@\" >> " + log + "\n" +
		"shift\n" +
		"case \"$1\" in\n" +
		"  info) exit 0 ;;\n" +
		"  install) exit 1 ;;\n" +
		"esac\n" +
		"exit 0\n"

	if err := os.WriteFile(filepath.Join(dir, "tlmgr"), []byte(script), 0755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))

	if err := ensureUserModePackage("somepkg"); err != nil {
		t.Fatalf("ensureUserModePackage() = %v, want nil when tlmgr reports it installed", err)
	}

	calls, err := os.ReadFile(log)
	if err != nil {
		t.Fatal(err)
	}

	got := string(calls)
	if !strings.Contains(got, "--usermode info somepkg") {
		t.Errorf("tlmgr info was not called; log:\n%s", got)
	}

	if strings.Contains(got, "install") {
		t.Errorf("tlmgr install was called for an installed package; log:\n%s", got)
	}

	if !userModePackageCache["somepkg"] {
		t.Error("package not cached after tlmgr reported it installed")
	}
}

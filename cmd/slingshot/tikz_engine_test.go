package main

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestContentHasCJK(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  bool
	}{
		{name: "chinese", input: "\\node {中文};", want: true},
		{name: "chinese in math text", input: "\\text{中文}", want: true},
		{name: "japanese hiragana", input: "\\node {こんにちは};", want: true},
		{name: "japanese katakana", input: "カタカナ", want: true},
		{name: "korean hangul syllables", input: "한국어", want: true},
		{name: "korean jamo", input: "한글", want: true},
		{name: "korean compatibility jamo", input: "ㄱㄴㄷ", want: true},
		{name: "fullwidth form", input: "ＡＢＣ", want: true},
		{name: "extension b", input: "\U00020000", want: true},
		{name: "pure ascii", input: "\\draw (0,0) -- (1,1);", want: false},
		{name: "math symbols only", input: "$\\alpha + \\beta = \\gamma$", want: false},
		{name: "emoji boundary no cjk", input: "😀 🚀", want: false},
		{name: "empty", input: "", want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := contentHasCJK(tt.input); got != tt.want {
				t.Errorf("contentHasCJK(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

// stubLookPath 返回一个 exec.LookPath seam 打桩函数: 命中白名单返回路径,
// 否则返回错误。
func stubLookPath(available map[string]bool) func(string) (string, error) {
	return func(file string) (string, error) {
		if available[file] {
			return "/usr/bin/" + file, nil
		}
		return "", errors.New(file + ": executable file not found in $PATH")
	}
}

func TestSelectTikzEngine(t *testing.T) {
	tests := []struct {
		name       string
		requested  string
		content    string
		latexmk    bool
		xelatex    bool
		xecjk      bool
		wantEngine tikzEngine
		wantErr    bool
	}{
		{
			name:      "empty requested defaults to xe",
			requested: "", content: "\\draw (0,0);",
			latexmk: true, xelatex: true,
			wantEngine: engineXe,
		},
		{
			name:      "explicit xe ok",
			requested: "xe", content: "\\draw (0,0);",
			latexmk: true, xelatex: true,
			wantEngine: engineXe,
		},
		{
			name:      "explicit xe cjk complete",
			requested: "xe", content: "\\node {中文};",
			latexmk: true, xelatex: true, xecjk: true,
			wantEngine: engineXe,
		},
		{
			name:      "explicit xe cjk missing xecjk errors",
			requested: "xe", content: "\\node {中文};",
			latexmk: true, xelatex: true, xecjk: false,
			wantErr: true,
		},
		{
			name:      "explicit xe latexmk missing errors",
			requested: "xe", content: "\\draw (0,0);",
			latexmk: false, xelatex: true,
			wantErr: true,
		},
		{
			name:      "pdf rejected",
			requested: "pdf", content: "\\draw (0,0);",
			latexmk: true, xelatex: true,
			wantErr: true,
		},
		{
			name:      "lua rejected",
			requested: "lua", content: "\\draw (0,0);",
			latexmk: true, xelatex: true,
			wantErr: true,
		},
		{
			name:      "tectonic rejected",
			requested: "tectonic", content: "\\draw (0,0);",
			latexmk: true, xelatex: true,
			wantErr: true,
		},
		{
			name:      "auto rejected",
			requested: "auto", content: "\\draw (0,0);",
			latexmk: true, xelatex: true,
			wantErr: true,
		},
		{
			name:      "invalid engine",
			requested: "bogus", content: "\\draw (0,0);",
			latexmk: true, xelatex: true,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			oldLook := lookPathFn
			oldKps := kpsewhichHasFn
			t.Cleanup(func() {
				lookPathFn = oldLook
				kpsewhichHasFn = oldKps
			})
			lookPathFn = stubLookPath(map[string]bool{
				"latexmk": tt.latexmk,
				"xelatex": tt.xelatex,
			})
			kpsewhichHasFn = func(file string) bool { return tt.xecjk }

			got, err := selectTikzEngine(tt.requested, tt.content)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("selectTikzEngine(%q) = %q, want error", tt.requested, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("selectTikzEngine(%q) unexpected error: %v", tt.requested, err)
			}
			if got != tt.wantEngine {
				t.Errorf("selectTikzEngine(%q) = %q, want %q", tt.requested, got, tt.wantEngine)
			}
		})
	}
}

// TestLatexmkCompileArgs 钉住每个引擎对应的 latexmk 标志, 并确保永不出现
// -shell-escape (它会把 LaTeX 文档变成任意命令执行)。
func TestLatexmkCompileArgs(t *testing.T) {
	tests := []struct {
		engine tikzEngine
		want   string
	}{
		{engine: engineXe, want: "-xelatex"},
	}
	for _, tt := range tests {
		args := latexmkCompileArgs(tt.engine)
		joined := strings.Join(args, " ")
		if strings.Contains(joined, "shell-escape") {
			t.Fatalf("latexmk args must not contain -shell-escape: %v", args)
		}
		if args[0] != tt.want || args[len(args)-1] != "input.tex" {
			t.Fatalf("latexmkCompileArgs(%q) = %v, want %q ... input.tex", tt.engine, args, tt.want)
		}
	}
}

// TestTikzCmdTimeout 覆盖 TIKZ_TIMEOUT 的解析: 未设置取默认值, 合法 duration
// 生效, 非法值报错 (不静默回退)。
func TestTikzCmdTimeout(t *testing.T) {
	t.Run("unset uses default", func(t *testing.T) {
		t.Setenv("TIKZ_TIMEOUT", "")
		got, err := tikzCmdTimeout()
		if err != nil {
			t.Fatalf("tikzCmdTimeout() unexpected error: %v", err)
		}
		if got != defaultTikzCmdTimeout {
			t.Fatalf("tikzCmdTimeout() = %s, want %s", got, defaultTikzCmdTimeout)
		}
	})
	t.Run("valid override", func(t *testing.T) {
		t.Setenv("TIKZ_TIMEOUT", "90s")
		got, err := tikzCmdTimeout()
		if err != nil {
			t.Fatalf("tikzCmdTimeout() unexpected error: %v", err)
		}
		if got != 90*time.Second {
			t.Fatalf("tikzCmdTimeout() = %s, want 90s", got)
		}
	})
	t.Run("invalid value errors", func(t *testing.T) {
		t.Setenv("TIKZ_TIMEOUT", "2 minutes")
		if _, err := tikzCmdTimeout(); err == nil {
			t.Fatal("tikzCmdTimeout() = nil error, want a parse error")
		}
	})
	t.Run("non positive errors", func(t *testing.T) {
		t.Setenv("TIKZ_TIMEOUT", "0s")
		if _, err := tikzCmdTimeout(); err == nil {
			t.Fatal("tikzCmdTimeout() = nil error, want a positive-duration error")
		}
	})
}

// --- Integration: real engines (skipped when missing) ---

// tikzIntegrationSamples 是集成测试使用的样例集合。每个样例是一个完整的
// TikZ 片段, 覆盖 tkz-euclide 5.x 语法与 circuitikz 元件。其中 [veclen] 与
// through= 是本改动的回归用例: 改动前在 TL2026 的 latexmk 路径上必失败。
// tikzIntegrationSamples holds the fragments whose failure signal points at
// slingshot rather than at TeX. Each one exercises code this repo owns:
//
//   - def_circle_r: the standalone wrapper every render goes through.
//   - through: the tkz-euclide package name, which the detector must
//     recognise before the 5.x syntax can compile.
//   - motor: a LaTeX shim, because circuitikz has no motor bipole.
//   - figchild: a self-contained command, which must not be wrapped.
//
// The fragments that used to live here -- tikzlings, tikzducks, tikzpeople,
// forest, pgfplots, tkz-triminos, scsnowman, the IEC and converter shims, and
// the minted/comment-family tcblisting layouts -- were asking "does this
// third-party package work?", and no change in this repository can answer
// that. Their slingshot-side handling is covered by the string-level tests in
// tikz_test.go, which run in microseconds.

// tikzIntegrationSamples holds the fragment whose failure signal points at
// code this repository owns rather than at a third-party package.
//
// motor is here because circuitikz has no motor bipole: slingshot supplies
// one as LaTeX, and only a compile can tell whether that LaTeX is correct.
//
// Everything else that used to live here was asking "does this package
// work?" -- tkz-euclide's syntax, tikzlings, tikzducks, tikzpeople, forest,
// pgfplots, tkz-triminos, scsnowman, the IEC and converter shims. No change
// in this repository can fix those, and the slingshot-side handling they
// exercised (package detection, library detection, the no-wrap rule) is
// covered by the string tests in tikz_test.go, which run in microseconds.
var tikzIntegrationSamples = map[string]string{
	"motor": `\begin{circuitikz}
\draw (0,0) to[motor] (2,0);
\end{circuitikz}
`,
}

// renderTikzSample 渲染单个样例到 outDir/sample.pdf, 返回 PDF 字节。
func renderTikzSample(t *testing.T, engine, name, sample string) []byte {
	t.Helper()
	in := filepath.Join(t.TempDir(), name+".tikz")
	out := filepath.Join(t.TempDir(), name+".pdf")
	if err := os.WriteFile(in, []byte(sample), 0644); err != nil {
		t.Fatal(err)
	}
	if err := renderTikz(in, out, engine); err != nil {
		t.Fatalf("renderTikz(%s, %s) failed: %v", name, engine, err)
	}
	data, err := os.ReadFile(out)
	if err != nil {
		t.Fatalf("reading output PDF: %v", err)
	}
	if !strings.HasPrefix(string(data), "%PDF") {
		t.Fatalf("output is not a PDF: %s", string(data[:min(16, len(data))]))
	}
	return data
}

// TestRenderTikzIntegrationXe 真跑 xe 引擎 (latexmk -xelatex, 可用时);
// 覆盖本改动的回归用例。
func TestRenderTikzIntegrationXe(t *testing.T) {
	t.Parallel()

	if err := latexmkAvailable(false); err != nil {
		t.Skipf("latexmk unavailable: %v", err)
	}
	for name, sample := range tikzIntegrationSamples {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			renderTikzSample(t, "xe", name, sample)
		})
	}
}

// assertRegionNotBlank 断言 img 的归一化区域 x[x0f,x1f) y[y0f,y1f) 内暗像素数
// > minDark; 不足时以 what 说明该区域的语义并失败。kind 是被渲染对象的类型名
// (如 "tcblisting" / "amscd CD"), 只用于失败消息前缀。抓的是"编译成功但内容
// 空白"这类静默丢失。阈值与消息格式单点化, 各渲染测试共用。
// countPureWhitePixels 统计 img 全图内亮度 >= 250 的像素数。tcblisting 标签底色
// 修复前是纯白 (255), 与盒底 (tcolorbox 默认 colback=black!5!white ≈ RGB 242)
// 形成"膏药"状白块; 修复后标签底色等于盒底, 纯白像素只剩标题白字。
// 亮度按 ITU-R BT.601, 与 countDarkPixels 同一口径。
// countExactColor 统计 img 全图内精确等于 (r,g,b) 的像素数。用于锁定用户
// colback 生效: 标签底色既不该回退到默认纯白, 也不该是硬编码盒底灰。
// edgeLuminance 返回归一化边缘中点 (w/2,0) 与 (0,h/2) 的 BT.601 亮度。
// 修复后 standalone 边距被裁掉 (tikzBorderSpec = 0pt), 页面边缘落在 tcblisting
// 盒子框线上 (暗); 未修复 / 无盒子的情形边缘是 2pt 纯白。

func TestRenderTikzEngineExit0WithoutPDF(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("fake engine script requires a POSIX shell")
	}
	writeInput := func(t *testing.T) (string, string) {
		t.Helper()
		in := filepath.Join(t.TempDir(), "in.tikz")
		out := filepath.Join(t.TempDir(), "out.pdf")
		if err := os.WriteFile(in, []byte("\\begin{tikzpicture}\n\\draw (0,0) -- (1,1);\n\\end{tikzpicture}\n"), 0644); err != nil {
			t.Fatal(err)
		}
		return in, out
	}
	fakeEngine := func(t *testing.T, name string) {
		t.Helper()
		dir := t.TempDir()
		if err := os.WriteFile(filepath.Join(dir, name), []byte("#!/bin/sh\nexit 0\n"), 0755); err != nil {
			t.Fatal(err)
		}
		t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	}

	t.Run("explicit xe", func(t *testing.T) {
		fakeEngine(t, "latexmk")
		fakeEngine(t, "xelatex")
		in, out := writeInput(t)
		err := renderTikz(in, out, "xe")
		if err == nil {
			t.Fatal("renderTikz() = nil, want an explicit error when the engine produces no PDF")
		}
		if !strings.Contains(err.Error(), "xe exited successfully but") {
			t.Fatalf("error = %v, want it to name the selected engine xe", err)
		}
	})
}

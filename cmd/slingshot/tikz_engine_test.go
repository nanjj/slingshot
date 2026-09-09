package main

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
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
		tectonic   bool
		wantEngine tikzEngine
		wantErr    bool
	}{
		// auto: latexmk 优先。
		{
			name:      "auto prefer latexmk no cjk",
			requested: "auto", content: "\\draw (0,0);",
			latexmk: true, xelatex: true, tectonic: true,
			wantEngine: engineLatexmk,
		},
		{
			name:      "auto latexmk missing cjk falls to tectonic",
			requested: "auto", content: "\\node {中文};",
			latexmk: true, xelatex: true, xecjk: false, tectonic: true,
			wantEngine: engineTectonic,
		},
		{
			name:      "auto latexmk with cjk complete",
			requested: "auto", content: "\\node {中文};",
			latexmk: true, xelatex: true, xecjk: true, tectonic: true,
			wantEngine: engineLatexmk,
		},
		{
			name:      "auto latexmk absent falls to tectonic",
			requested: "auto", content: "\\draw (0,0);",
			latexmk: false, xelatex: true, tectonic: true,
			wantEngine: engineTectonic,
		},
		{
			name:      "auto xelatex absent falls to tectonic",
			requested: "auto", content: "\\draw (0,0);",
			latexmk: true, xelatex: false, tectonic: true,
			wantEngine: engineTectonic,
		},
		{
			name:      "auto none available errors",
			requested: "auto", content: "\\draw (0,0);",
			latexmk: false, xelatex: false, tectonic: false,
			wantErr: true,
		},
		// 显式 latexmk: 探测失败直接报错, 不静默回退。
		{
			name:      "explicit latexmk ok",
			requested: "latexmk", content: "\\draw (0,0);",
			latexmk: true, xelatex: true,
			wantEngine: engineLatexmk,
		},
		{
			name:      "explicit latexmk cjk missing xecjk errors",
			requested: "latexmk", content: "\\node {中文};",
			latexmk: true, xelatex: true, xecjk: false, tectonic: true,
			wantErr: true,
		},
		{
			name:      "explicit latexmk missing errors",
			requested: "latexmk", content: "\\draw (0,0);",
			latexmk: false, xelatex: true, tectonic: true,
			wantErr: true,
		},
		// 显式 tectonic: 探测失败直接报错, 不静默回退。
		{
			name:      "explicit tectonic ok",
			requested: "tectonic", content: "\\draw (0,0);",
			tectonic:   true,
			wantEngine: engineTectonic,
		},
		{
			name:      "explicit tectonic missing errors",
			requested: "tectonic", content: "\\draw (0,0);",
			latexmk: true, xelatex: true, tectonic: false,
			wantErr: true,
		},
		// 非法值。
		{
			name:      "invalid engine",
			requested: "bogus", content: "\\draw (0,0);",
			latexmk: true, xelatex: true, tectonic: true,
			wantErr: true,
		},
		{
			name:      "empty requested treated as auto",
			requested: "", content: "\\draw (0,0);",
			latexmk: true, xelatex: true, tectonic: true,
			wantEngine: engineLatexmk,
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
				"latexmk":  tt.latexmk,
				"xelatex":  tt.xelatex,
				"tectonic": tt.tectonic,
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

func TestLatexmkCompileArgsNoShellEscape(t *testing.T) {
	args := latexmkCompileArgs()
	joined := strings.Join(args, " ")
	if strings.Contains(joined, "shell-escape") {
		t.Fatalf("latexmk args must not contain -shell-escape: %v", args)
	}
	if args[0] != "-xelatex" || args[len(args)-1] != "input.tex" {
		t.Fatalf("unexpected latexmk args: %v", args)
	}
}

// --- Integration: real engines (skipped when missing) ---

// tikzIntegrationSamples 是集成测试使用的样例集合。每个样例是一个完整的
// TikZ 片段, 覆盖 tkz-euclide 5.x 语法与 circuitikz 元件。其中 [veclen] 与
// through= 是本改动的回归用例: 改动前在 TL2026 的 latexmk 路径上必失败。
var tikzIntegrationSamples = map[string]string{
	"def_circle_r": `\begin{tikzpicture}
\tkzDefPoint(0,0){A}
\tkzDefCircle[R](A,1) \tkzGetPoint{a}
\tkzDrawCircle(A,a)
\tkzDrawPoint(A) \tkzDrawPoint(a)
\end{tikzpicture}
`,
	"veclen": `\begin{tikzpicture}
\tkzDefPoint(0,0){O}
\begin{scope}[veclen]
\tkzDefPoint(1,0){A}
\end{scope}
\tkzDrawPoint(O) \tkzDrawPoint(A)
\end{tikzpicture}
`,
	"through": `\begin{tikzpicture}
\tkzDefPoint(0,0){A} \tkzDefPoint(2,0){B}
\tkzDefCircle[apollonius,K=2](A,B) \tkzGetPoints{K1}{k}
\tkzDefPointOnCircle[through= center K1 angle 60 point k] \tkzGetPoint{I}
\tkzDrawPoint(I)
\end{tikzpicture}
`,
	"next_to": `\begin{tikzpicture}
\tkzDefPoint(0,0){A} \tkzDefPoint(2,0){B}
\tkzDefCircle[R](A,1) \tkzGetPoint{c}
\tkzInterLC[next to=A](A,B)(A,c) \tkzGetFirstPoint{X}
\tkzDrawPoint(X)
\end{tikzpicture}
`,
	"tangent_at": `\begin{tikzpicture}
\tkzDefPoint(0,0){O} \tkzDefPoint(1,0){T}
\tkzDefLine[tangent at=T](O) \tkzGetPoint{h}
\tkzDrawPoint(h)
\end{tikzpicture}
`,
	"buzzer": `\begin{circuitikz}
\draw (0,0) to[buzzer] (0,2);
\end{circuitikz}
`,
	"motor": `\begin{circuitikz}
\draw (0,0) to[motor] (2,0);
\end{circuitikz}
`,
	"circuit_ee_iec": `\usetikzlibrary{circuits.ee.IEC}
\begin{tikzpicture}[circuit ee IEC]
\draw (0,0) to[resistor={name=R}] (0,2);
\end{tikzpicture}
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

// TestRenderTikzIntegrationLatexmk 真跑 latexmk (可用时); 覆盖本改动的回归用例。
func TestRenderTikzIntegrationLatexmk(t *testing.T) {
	if err := latexmkAvailable(false); err != nil {
		t.Skipf("latexmk unavailable: %v", err)
	}
	for name, sample := range tikzIntegrationSamples {
		t.Run(name, func(t *testing.T) {
			renderTikzSample(t, "latexmk", name, sample)
		})
	}
}

// TestRenderTikzIntegrationTectonic 真跑 tectonic (可用时)。legacy 翻译与
// 2021-bundle shim 全部挂在 tectonic profile 上, 因此覆盖全部样例, 并额外
// 覆盖含 CJK 的片段 (tectonic bundle 自带 xeCJK)。
func TestRenderTikzIntegrationTectonic(t *testing.T) {
	if err := tectonicAvailable(); err != nil {
		t.Skipf("tectonic unavailable: %v", err)
	}
	for name, sample := range tikzIntegrationSamples {
		t.Run(name, func(t *testing.T) {
			renderTikzSample(t, "tectonic", name, sample)
		})
	}
	cjk := `\begin{tikzpicture}
\node at (0,0) {\text{中文}};
\end{tikzpicture}
`
	t.Run("cjk", func(t *testing.T) {
		assertPDFContainsText(t, renderTikzSample(t, "tectonic", "cjk", cjk), "中文")
	})
}

// TestRenderTikzIntegrationCJK 真跑 latexmk + xelatex + xeCJK 三者齐备时渲染 CJK。
func TestRenderTikzIntegrationCJK(t *testing.T) {
	if err := latexmkAvailable(true); err != nil {
		t.Skipf("latexmk + xelatex + xeCJK unavailable: %v", err)
	}
	sample := `\begin{tikzpicture}
\node at (0,0) {\text{中文}};
\end{tikzpicture}
`
	assertPDFContainsText(t, renderTikzSample(t, "latexmk", "cjk", sample), "中文")
}

// assertPDFContainsText 用 pdftotext 提取 PDF 文本并断言包含 want。
// pdftotext 不可用时只记日志: 该断言用于捕获 "PDF 有效但字形丢失 (tofu)"
// 这类回归 (CJK 前导被丢弃时就是如此), 缺失工具不应让测试失败。
func assertPDFContainsText(t *testing.T, pdf []byte, want string) {
	t.Helper()
	if _, err := exec.LookPath("pdftotext"); err != nil {
		t.Logf("pdftotext unavailable, skipping text assertion: %v", err)
		return
	}
	path := filepath.Join(t.TempDir(), "rendered.pdf")
	if err := os.WriteFile(path, pdf, 0644); err != nil {
		t.Fatal(err)
	}
	out, err := exec.Command("pdftotext", path, "-").Output()
	if err != nil {
		t.Fatalf("pdftotext failed: %v", err)
	}
	if !strings.Contains(string(out), want) {
		t.Fatalf("rendered PDF text does not contain %q; got %q", want, string(out))
	}
}

// TestRenderTikzEngineExit0WithoutPDF 钉住 "引擎退出 0 但没有产物" 的显式报错,
// 并断言错误里出现的是**实际选中的引擎**而不是 --engine 的原始取值:
// 用只 exit 0 的假引擎走完整 renderTikz 路径 (真实引擎很难构造该场景)。
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

	t.Run("explicit tectonic", func(t *testing.T) {
		fakeEngine(t, "tectonic")
		in, out := writeInput(t)
		err := renderTikz(in, out, "tectonic")
		if err == nil {
			t.Fatal("renderTikz() = nil, want an explicit error when the engine produces no PDF")
		}
		if !strings.Contains(err.Error(), "tectonic exited successfully but") {
			t.Fatalf("error = %v, want it to name the selected engine tectonic", err)
		}
	})

	t.Run("auto selects latexmk", func(t *testing.T) {
		if _, err := exec.LookPath("xelatex"); err != nil {
			t.Skipf("xelatex unavailable: %v", err)
		}
		fakeEngine(t, "latexmk")
		in, out := writeInput(t)
		err := renderTikz(in, out, "auto")
		if err == nil {
			t.Fatal("renderTikz() = nil, want an explicit error when the engine produces no PDF")
		}
		if !strings.Contains(err.Error(), "latexmk exited successfully but") {
			t.Fatalf("error = %v, want the selected engine (latexmk), not the --engine flag value (auto)", err)
		}
	})
}

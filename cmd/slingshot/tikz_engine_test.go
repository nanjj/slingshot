package main

import (
	"errors"
	"os"
	"path/filepath"
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

// TestRenderTikzIntegrationTectonic 真跑 tectonic (可用时)。
func TestRenderTikzIntegrationTectonic(t *testing.T) {
	if err := tectonicAvailable(); err != nil {
		t.Skipf("tectonic unavailable: %v", err)
	}
	renderTikzSample(t, "tectonic", "motor", tikzIntegrationSamples["motor"])
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
	renderTikzSample(t, "latexmk", "cjk", sample)
}

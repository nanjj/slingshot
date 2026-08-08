package mathrender

import (
	"bytes"
	"os"
	"strings"
	"testing"
)

// stubRender replaces the active renderer with one that returns a fake
// injection fragment, letting ProcessMath tests run without any engine.
func stubRender(t *testing.T, p *processor) {
	t.Helper()
	old := p.render
	p.render = func(tex string, display bool) (string, error) {
		style := "inline"
		if display {
			style = "display"
		}
		return `<svg data-test="` + style + `">` + tex + `</svg>`, nil
	}
	t.Cleanup(func() { p.render = old })
}

// newTestProcessor builds a processor with a stubbed renderer.
func newTestProcessor(t *testing.T) *processor {
	t.Helper()
	var stderr bytes.Buffer
	p := &processor{outDir: t.TempDir(), mode: ModeAuto, stderr: &stderr}
	stubRender(t, p)
	return p
}

// run processes HTML through the processor directly (bypassing engine
// detection, which is covered by integration tests).
func run(t *testing.T, html string, mode Mode) (string, string) {
	t.Helper()
	p := newTestProcessor(t)
	p.mode = mode
	out := string(p.run([]byte(html)))
	return out, p.stderr.(*bytes.Buffer).String()
}

func TestInlineDollar(t *testing.T) {
	out, _ := run(t, `<p style="x">about $ \pi $ in text</p>`, ModeAuto)
	if !strings.Contains(out, `<svg data-test="inline">`) {
		t.Fatalf("expected inline formula svg, got: %s", out)
	}
	if strings.Contains(out, "$ \\pi $") {
		t.Fatalf("original formula not replaced: %s", out)
	}
	if !strings.Contains(out, "in text</p>") {
		t.Fatalf("surrounding text lost: %s", out)
	}
	if !strings.Contains(out, `\pi`) {
		t.Fatalf("formula body not passed to renderer: %s", out)
	}
}

func TestInlineAcrossSoftLineBreak(t *testing.T) {
	// ox-md wraps long paragraphs; a formula may span two lines.
	out, _ := run(t, `<p style="x">diameter is $ 1.85 \times 31 = 57.4
$ km</p>`, ModeAuto)
	if !strings.Contains(out, `1.85 \times 31 = 57.4`) {
		t.Fatalf("formula body not cleaned across newline: %s", out)
	}
	if strings.Contains(out, "\n") && strings.Contains(out, "$") {
		t.Fatalf("formula delimiters leaked: %s", out)
	}
}

func TestInlineParen(t *testing.T) {
	out, _ := run(t, `<p>value \( \pi \) here</p>`, ModeAuto)
	if !strings.Contains(out, `<svg`) || strings.Contains(out, `\( \pi \)`) {
		t.Fatalf("paren formula not replaced: %s", out)
	}
}

func TestBlockDollar(t *testing.T) {
	out, _ := run(t, "<p>$$\nK = \\frac{1}{R}\n$$</p>", ModeAuto)
	if !strings.Contains(out, `data-test="display"`) {
		t.Fatalf("expected display style, got: %s", out)
	}
	if !strings.Contains(out, `K = \frac{1}{R}`) {
		t.Fatalf("block body wrong: %s", out)
	}
}

func TestBlockEquation(t *testing.T) {
	out, _ := run(t, "<p>\\begin{equation}\ne ^ {-1} = \\frac{1}{2!}\n\\end{equation}</p>", ModeAuto)
	if !strings.Contains(out, `data-test="display"`) {
		t.Fatalf("expected display style, got: %s", out)
	}
	// equation is stripped of its environment (no numbers) → plain content.
	if !strings.Contains(out, `<svg data-test="display">e ^ {-1} = \frac{1}{2!}</svg>`) {
		t.Fatalf("equation body wrong: %s", out)
	}
}

func TestBlockAlignKeepsEnvironment(t *testing.T) {
	out, _ := run(t, "<p>\\begin{align}\na &= b \\\\\nc &= d\n\\end{align}</p>", ModeAuto)
	if !strings.Contains(out, `<svg data-test="display">\begin{align} a &= b \\ c &= d \end{align}</svg>`) {
		t.Fatalf("align body should keep environment: %s", out)
	}
}

func TestBlockBracket(t *testing.T) {
	out, _ := run(t, "<p>\\[\nK = \\frac{1}{R}\n\\]</p>", ModeAuto)
	if !strings.Contains(out, `data-test="display"`) {
		t.Fatalf("bracket block not display: %s", out)
	}
}

func TestCodeProtected(t *testing.T) {
	// Inline code and code blocks must keep their $ untouched.
	out, _ := run(t, `<p>shell <code>$HOME</code> and <code>a $x$ b</code> ok</p>`+
		`<section class="code-snippet__fix code-snippet__go"><pre class="code-snippet__go"><code><span class="code-snippet_outer">x := 1 // $not math$</span></code></pre></section>`, ModeAuto)
	if strings.Contains(out, `<svg`) {
		t.Fatalf("code regions must be protected: %s", out)
	}
	if !strings.Contains(out, "$HOME") || !strings.Contains(out, "// $not math$") {
		t.Fatalf("code content lost: %s", out)
	}
}

func TestImgProtected(t *testing.T) {
	// An existing img whose title contains $ must not be touched.
	out, _ := run(t, `<p><img src="a.png" title="cost $5 each" style="x"></p>`, ModeAuto)
	if !strings.Contains(out, `<img src="a.png" title="cost $5 each" style="x">`) {
		t.Fatalf("existing img altered: %s", out)
	}
}

func TestEscapedBodyUnescaped(t *testing.T) {
	// goldmark escapes < as &lt; in text; the renderer must get the raw <.
	out, _ := run(t, `<p>$ 3.1415926 &lt; \pi &lt; 3.1415927 $</p>`, ModeAuto)
	if !strings.Contains(out, `3.1415926 < \pi < 3.1415927`) {
		t.Fatalf("entity not unescaped: %s", out)
	}
}

func TestRenderFailureKeepsText(t *testing.T) {
	var stderr bytes.Buffer
	p := &processor{outDir: t.TempDir(), mode: ModeAuto, stderr: &stderr}
	p.render = func(tex string, display bool) (string, error) {
		return "", os.ErrNotExist // simulate engine failure
	}
	out := string(p.run([]byte(`<p>keep $ \pi $ as-is</p>`)))
	if !strings.Contains(out, "$ \\pi $") {
		t.Fatalf("failed render must keep original text: %s", out)
	}
	if !strings.Contains(stderr.String(), "Warning: formula render failed") {
		t.Fatalf("expected warning on stderr, got: %s", stderr.String())
	}
}

func TestNoRendererKeepsText(t *testing.T) {
	var stderr bytes.Buffer
	p := &processor{outDir: t.TempDir(), mode: ModeAuto, stderr: &stderr} // render nil
	out := string(p.run([]byte(`<p>keep $ \pi $ as-is</p>`)))
	if !strings.Contains(out, "$ \\pi $") {
		t.Fatalf("nil renderer must keep original text: %s", out)
	}
	if strings.Contains(out, "<svg") {
		t.Fatalf("nil renderer must not inject svg: %s", out)
	}
}

func TestEmptyFormulaKept(t *testing.T) {
	out, _ := run(t, `<p>a $ $ b</p>`, ModeAuto)
	if strings.Contains(out, "<svg") {
		t.Fatalf("empty formula must stay: %s", out)
	}
}

func TestIdempotent(t *testing.T) {
	first, _ := run(t, `<p>about $ \pi $ text</p>`, ModeAuto)
	second, _ := run(t, first, ModeAuto)
	if second != first {
		t.Fatalf("second pass changed output:\n--- first ---\n%s\n--- second ---\n%s", first, second)
	}
}

func TestModeText(t *testing.T) {
	out, _ := run(t, `<p>about $ \pi $ text</p>`, ModeText)
	if !strings.Contains(out, "$ \\pi $") {
		t.Fatalf("ModeText must not touch formulas: %s", out)
	}
}

func TestMultipleFormulas(t *testing.T) {
	out, _ := run(t, `<p>$a$ and $b$ plus $ \frac{1}{2} $</p>`, ModeAuto)
	if strings.Count(out, "<svg") != 3 {
		t.Fatalf("expected 3 formulas, got: %s", out)
	}
}

func TestHeadingFormula(t *testing.T) {
	out, _ := run(t, `<h2 style="x">$ \pi $ 之点滴</h2>`, ModeAuto)
	if !strings.Contains(out, `<h2 style="x"><svg data-test="inline">`) {
		t.Fatalf("heading formula not replaced: %s", out)
	}
}

func TestParseMode(t *testing.T) {
	for _, tc := range []struct {
		in   string
		want Mode
		ok   bool
	}{
		{"auto", ModeAuto, true},
		{"AUTO", ModeAuto, true},
		{"svg", ModeSVG, true},
		{"png", ModePNG, true},
		{"text", ModeText, true},
		{"bogus", ModeAuto, false},
	} {
		got, err := ParseMode(tc.in)
		if (err == nil) != tc.ok {
			t.Errorf("ParseMode(%q) err=%v, want ok=%v", tc.in, err, tc.ok)
		}
		if err == nil && got != tc.want {
			t.Errorf("ParseMode(%q) = %v, want %v", tc.in, got, tc.want)
		}
	}
}

// --- Integration: real engines (skipped when missing) ---

func TestRenderSVGReal(t *testing.T) {
	if err := svgEngineAvailable(); err != nil {
		t.Skipf("svg engine unavailable: %v", err)
	}
	inject, err := renderSVG(`\pi \approx \frac{22}{7}`, false)
	if err != nil {
		t.Fatalf("renderSVG failed: %v", err)
	}
	if !strings.HasPrefix(inject, `<span style="white-space:nowrap;display:inline-block">`) {
		t.Fatalf("inline wrapper missing: %s", inject[:min(80, len(inject))])
	}
	if !strings.Contains(inject, "<svg") || !strings.Contains(inject, `viewBox=`) {
		t.Fatalf("svg element missing: %s", inject[:min(120, len(inject))])
	}
	// Baseline alignment in ex units — the WeChat-proven mdnice format.
	if !strings.Contains(inject, "vertical-align: -") {
		t.Fatalf("baseline vertical-align missing: %s", inject[:min(160, len(inject))])
	}

	disp, err := renderSVG(`K = \frac{1}{R}`, true)
	if err != nil {
		t.Fatalf("display renderSVG failed: %v", err)
	}
	if !strings.HasPrefix(disp, `<section style="text-align:center;overflow-x:auto;display:block">`) {
		t.Fatalf("display wrapper missing: %s", disp[:min(120, len(disp))])
	}
}

func TestRenderPNGReal(t *testing.T) {
	if err := pngEnginesAvailable(); err != nil {
		t.Skipf("png engine unavailable: %v", err)
	}
	dir := t.TempDir()
	tag, err := renderPNG(`\pi \approx \frac{22}{7}`, false, dir)
	if err != nil {
		t.Fatalf("renderPNG failed: %v", err)
	}
	if !strings.HasPrefix(tag, `<img src="formula-`) {
		t.Fatalf("unexpected tag: %s", tag)
	}
	// Cache hit: second call returns same path without re-rendering.
	again, err := renderPNG(`\pi \approx \frac{22}{7}`, false, dir)
	if err != nil || again != tag {
		t.Fatalf("cache hit mismatch: %q %v", again, err)
	}
	// Display variant uses a different key.
	disp, err := renderPNG(`\pi \approx \frac{22}{7}`, true, dir)
	if err != nil {
		t.Fatalf("display render failed: %v", err)
	}
	if disp == tag {
		t.Fatal("display and inline must have distinct cache keys")
	}
}

// Go 1.21+ has a builtin min; this file relies on it.

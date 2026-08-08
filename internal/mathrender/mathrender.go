// Package mathrender renders LaTeX formulas for WeChat articles.
//
// WeChat public account articles do not execute JavaScript and have no LaTeX
// renderer — MathJax/KaTeX running in the browser are useless there. Two
// battle-tested approaches remain:
//
//  1. Inline SVG (preferred): render each formula to a self-contained SVG
//     (MathJax 3, the exact output mdnice uses — WeChat keeps and renders
//     <svg> elements, as proven by countless published articles). SVG is
//     vector (crisp on retina), baseline-aligned (MathJax emits
//     vertical-align in ex units), and needs no upload at all.
//
//  2. PNG (fallback): render via tectonic + pdftoppm to <dir>/formula-<md5>.png
//     and inject an <img> tag. The image flows through the existing upload
//     pipeline (ExtractImagePaths → uploadcache md5 → WeChat CDN URL), so
//     identical formulas across articles reuse the same uploaded URL.
//
// Formula detection runs over the generated WeChat HTML:
//
//	$...$                  inline  (ox-md emits \(...\) this way)
//	\(...\)                inline  (from .md input)
//	$$...$$                display
//	\[...\]                display
//	\begin{equation}...\end{equation}  display (stripped, no numbers)
//	\begin{align}...\end{align} etc.  display (kept verbatim)
//
// Code blocks, code spans and existing <img> tags are protected from
// substitution. The pass is idempotent.
//
// System dependencies (auto-detected, degrade gracefully):
//
//	SVG: node + mathjax-full   — npm install --prefix ~/.local mathjax-full
//	PNG: tectonic + pdftoppm   — apt install tectonic poppler-utils
package mathrender

import (
	"context"
	"crypto/md5"
	"errors"
	"fmt"
	"html"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// Mode controls how formulas are handled during conversion.
type Mode int

const (
	// ModeAuto renders with the best available engine (SVG, then PNG);
	// if none is available, keeps the LaTeX text with a warning.
	ModeAuto Mode = iota
	// ModeSVG always renders inline SVG; errors if the engine is missing.
	ModeSVG
	// ModePNG always renders PNG images; errors if the engine is missing.
	ModePNG
	// ModeText leaves formulas untouched.
	ModeText
)

// ParseMode parses a --math flag value.
func ParseMode(s string) (Mode, error) {
	switch strings.ToLower(s) {
	case "auto":
		return ModeAuto, nil
	case "svg":
		return ModeSVG, nil
	case "png":
		return ModePNG, nil
	case "text":
		return ModeText, nil
	}
	return ModeAuto, fmt.Errorf("invalid math mode %q (want auto, svg, png or text)", s)
}

func (m Mode) String() string {
	switch m {
	case ModeSVG:
		return "svg"
	case ModePNG:
		return "png"
	case ModeText:
		return "text"
	default:
		return "auto"
	}
}

// --- Engines ---

// tex2svgJS is the MathJax 3 conversion script, embedded verbatim. It reads
// the TeX from stdin and writes the <svg> element (fontCache none → every
// glyph is inlined as a path, so each SVG is self-contained — required for
// WeChat, which cannot share a font cache across elements).
const tex2svgJS = `const fs = require('fs');
const {mathjax} = require('mathjax-full/js/mathjax.js');
const {TeX} = require('mathjax-full/js/input/tex.js');
const {SVG} = require('mathjax-full/js/output/svg.js');
const {liteAdaptor} = require('mathjax-full/js/adaptors/liteAdaptor.js');
const {RegisterHTMLHandler} = require('mathjax-full/js/handlers/html.js');
const {AllPackages} = require('mathjax-full/js/input/tex/AllPackages.js');

const adaptor = liteAdaptor();
RegisterHTMLHandler(adaptor);
const tex = new TeX({packages: AllPackages});
const svg = new SVG({fontCache: 'none'});
const html = mathjax.document('', {InputJax: tex, OutputJax: svg});

const display = process.argv[2] === 'display';
const input = fs.readFileSync(0, 'utf8').trim();
const node = html.convert(input, {display: display});
const mjx = adaptor.innerHTML(node);
// Keep the <svg> element (mdnice's published articles contain bare <svg>
// inside spans/sections; WeChat renders them).
const m = mjx.match(/<svg[\s\S]*<\/svg>/);
process.stdout.write(m ? m[0] : mjx);
`

// findMathJaxRoot locates the mathjax-full package via npm's global root or
// common user-level prefixes.
func findMathJaxRoot() (string, error) {
	// 1. npm root -g (honors npm prefix config).
	if out, err := exec.Command("npm", "root", "-g").Output(); err == nil {
		p := strings.TrimSpace(string(out))
		if mathjaxPresent(p) {
			return p, nil
		}
	}
	// 2. Common locations: user-level npm prefixes (npm install --prefix
	//    ~/.local puts packages in ~/.local/node_modules), brew, distro.
	home, _ := os.UserHomeDir()
	for _, p := range []string{
		filepath.Join(home, ".local", "node_modules"),
		filepath.Join(home, ".local", "lib", "node_modules"),
		filepath.Join(home, "node_modules"),
		"/usr/local/lib/node_modules",
		"/usr/lib/node_modules",
	} {
		if mathjaxPresent(p) {
			return p, nil
		}
	}
	return "", errors.New("mathjax-full not found (install via: npm install --prefix ~/.local mathjax-full)")
}

func mathjaxPresent(root string) bool {
	_, err := os.Stat(filepath.Join(root, "mathjax-full", "js", "mathjax.js"))
	return err == nil
}

// svgEngineAvailable reports whether node + mathjax-full are usable.
func svgEngineAvailable() error {
	if _, err := exec.LookPath("node"); err != nil {
		return fmt.Errorf("node not found: %w", err)
	}
	return findMathJaxRootErr()
}

// findMathJaxRootErr wraps findMathJaxRoot with an explanatory prefix.
func findMathJaxRootErr() error {
	_, err := findMathJaxRoot()
	if err != nil {
		return fmt.Errorf("mathjax-full not found (install via 'npm install --prefix ~/.local mathjax-full'): %w", err)
	}
	return nil
}

// pngEnginesAvailable reports whether tectonic + pdftoppm are usable.
func pngEnginesAvailable() error {
	if _, err := exec.LookPath("tectonic"); err != nil {
		return fmt.Errorf("tectonic not found (install via 'apt install tectonic'): %w", err)
	}
	if _, err := exec.LookPath("pdftoppm"); err != nil {
		return fmt.Errorf("pdftoppm not found (install via 'apt install poppler-utils'): %w", err)
	}
	return nil
}

// renderSVG renders a formula to a self-contained MathJax SVG and returns
// the HTML fragment to inject (span/section wrapper + <svg>).
func renderSVG(tex string, display bool) (string, error) {
	if err := svgEngineAvailable(); err != nil {
		return "", err
	}
	nodePath, err := exec.LookPath("node")
	if err != nil {
		return "", err
	}
	mjRoot, err := findMathJaxRoot()
	if err != nil {
		return "", err
	}

	tmpDir, err := os.MkdirTemp("", "slingshot-mathjax-*")
	if err != nil {
		return "", fmt.Errorf("creating temp dir: %w", err)
	}
	defer os.RemoveAll(tmpDir)
	script := filepath.Join(tmpDir, "tex2svg.js")
	if err := os.WriteFile(script, []byte(tex2svgJS), 0644); err != nil {
		return "", fmt.Errorf("writing tex2svg script: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, nodePath, script)
	if display {
		cmd.Args = append(cmd.Args, "display")
	}
	cmd.Env = append(os.Environ(), "NODE_PATH="+mjRoot)
	cmd.Stdin = strings.NewReader(tex)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("mathjax failed: %w\n%s", err, truncate(out))
	}
	svg := strings.TrimSpace(string(out))
	if !strings.HasPrefix(svg, "<svg") {
		return "", fmt.Errorf("mathjax produced no svg: %s", truncate(out))
	}

	if display {
		return `<section style="text-align:center;overflow-x:auto;display:block">` + svg + `</section>`, nil
	}
	// Inline: nowrap span keeps the formula from being split across lines.
	return `<span style="white-space:nowrap;display:inline-block">` + svg + `</span>`, nil
}

// --- PNG fallback ---

const (
	// renderDPI is the PDF→PNG resolution for the tectonic fallback.
	renderDPI = 300
	// formulaBorder is the standalone-class padding around the math content.
	formulaBorder = 4
)

// texHeader/texFooter wrap a single formula in a standalone document.
// standalone clips the output to the content bounding box, so the resulting
// PNG is exactly the formula's size.
var texHeader = `\documentclass[border=` + strconv.Itoa(formulaBorder) + `pt]{standalone}
\usepackage{amsmath}
\begin{document}
`

const texFooter = `
\end{document}
`

// formulaKey returns the md5-based cache key for a formula. The display flag
// is part of the key: the same LaTeX may render at a different size inline
// vs. displayed.
func formulaKey(tex string, display bool) string {
	prefix := "inline:"
	if display {
		prefix = "display:"
	}
	sum := md5.Sum([]byte(prefix + tex))
	return fmt.Sprintf("%x", sum)
}

// renderPNG renders a formula to <outDir>/formula-<md5>.png and returns the
// <img> tag. Rendering is cached: if the PNG already exists it is reused.
func renderPNG(tex string, display bool, outDir string) (string, error) {
	if err := pngEnginesAvailable(); err != nil {
		return "", err
	}

	key := formulaKey(tex, display)
	pngPath := filepath.Join(outDir, "formula-"+key+".png")
	if _, err := os.Stat(pngPath); err == nil {
		return pngTag(pngPath, tex, display), nil // cache hit
	}

	tmpDir, err := os.MkdirTemp("", "slingshot-formula-*")
	if err != nil {
		return "", fmt.Errorf("creating temp dir: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	// Inline math renders as $...$; display math must also use inline math
	// syntax with \displaystyle — the standalone class's preview package is
	// broken for \[...\], $$...$$ and math environments under XeTeX
	// ("Missing $ inserted"). Multi-line environments are converted to their
	// aligned/gathered inline equivalents, which are valid in $...$.
	mathBody := "$" + tex + "$"
	if display {
		mathBody = displayBody(tex)
	}
	texSrc := texHeader + mathBody + texFooter
	if err := os.WriteFile(filepath.Join(tmpDir, "formula.tex"), []byte(texSrc), 0644); err != nil {
		return "", fmt.Errorf("writing formula.tex: %w", err)
	}

	// tectonic → PDF. The first run downloads packages; give it room.
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()
	tectonic := exec.CommandContext(ctx, "tectonic", "formula.tex")
	tectonic.Dir = tmpDir
	if out, err := tectonic.CombinedOutput(); err != nil {
		return "", fmt.Errorf("tectonic failed: %w\n%s", err, truncate(out))
	}

	// PDF → PNG (single page → formula-1.png)
	ctx, cancel = context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	ppm := exec.CommandContext(ctx, "pdftoppm", "-png", "-r", strconv.Itoa(renderDPI),
		"formula.pdf", "formula")
	ppm.Dir = tmpDir
	if out, err := ppm.CombinedOutput(); err != nil {
		return "", fmt.Errorf("pdftoppm failed: %w\n%s", err, truncate(out))
	}

	data, err := os.ReadFile(filepath.Join(tmpDir, "formula-1.png"))
	if err != nil {
		return "", fmt.Errorf("reading rendered PNG: %w", err)
	}
	if err := os.WriteFile(pngPath, data, 0644); err != nil {
		return "", fmt.Errorf("writing %q: %w", pngPath, err)
	}
	return pngTag(pngPath, tex, display), nil
}

// displayBody wraps display-mode formula content for the tectonic fallback:
// $\displaystyle ...$ (standalone + XeTeX cannot handle \[...\]/$$/math
// environments, see renderPNG). Multi-line environments are converted to
// their inline equivalents so they are valid inside $...$.
func displayBody(tex string) string {
	body := strings.TrimSpace(tex)
	// align/align* → aligned, gather/gather* → gathered, split stays split.
	for _, pair := range [][2]string{
		{`\begin{align*}`, `\begin{aligned}`},
		{`\end{align*}`, `\end{aligned}`},
		{`\begin{align}`, `\begin{aligned}`},
		{`\end{align}`, `\end{aligned}`},
		{`\begin{gather*}`, `\begin{gathered}`},
		{`\end{gather*}`, `\end{gathered}`},
		{`\begin{gather}`, `\begin{gathered}`},
		{`\end{gather}`, `\end{gathered}`},
	} {
		body = strings.ReplaceAll(body, pair[0], pair[1])
	}
	return "$\\displaystyle " + body + "$"
}

// pngTag builds the <img> tag for a rendered formula PNG.
func pngTag(pngPath, tex string, display bool) string {
	style := inlineStyle
	if display {
		style = displayStyle
	}
	return fmt.Sprintf(`<img src="%s" alt="%s" style="%s">`,
		filepath.Base(pngPath), html.EscapeString(tex), style)
}

// truncate caps error output at 1 KiB so a failing engine run cannot flood
// the terminal.
func truncate(b []byte) string {
	const max = 1024
	s := string(b)
	if len(s) > max {
		s = s[:max] + "... (truncated)"
	}
	return s
}

// --- HTML post-processing ---

// Formula shapes recognized in the WeChat HTML. Order matters: multi-char
// delimiters ($$, \begin{env}, \[) are matched before $...$ so they are not
// consumed piecewise.
var (
	// Display: \begin{equation}...\end{equation} (or align/gather/...) —
	// emitted verbatim by ox-md from Org's latex-environment blocks.
	reBlockEnv = regexp.MustCompile(`(?s)\\begin\{[a-zA-Z*]+\}(.+?)\\end\{[a-zA-Z*]+\}`)
	// Display: $$...$$ (common in .md input).
	reBlockDollar = regexp.MustCompile(`(?s)\$\$(.+?)\$\$`)
	// Display: \[...\] — \\\[ is literal backslash + literal [ in RE2
	// (\\ = escaped backslash, \[ = escaped bracket).
	reBlockBracket = regexp.MustCompile(`(?s)\\\[(.+?)\\\]`)
	// Inline: $...$ (ox-md emits \(...\) this way; may span soft line
	// breaks inside a paragraph, so [^$]+ includes \n).
	reInlineDollar = regexp.MustCompile(`\$([^$]+)\$`)
	// Inline: \(...\) (from .md input).
	reInlineParen = regexp.MustCompile(`(?s)\\\((.+?)\\\)`)

	// reEnvName extracts the environment name from a matched \begin{env} block.
	reEnvName = regexp.MustCompile(`\\begin\{([a-zA-Z*]+)\}`)

	// Protected regions: content that must never be touched by formula
	// matching — code blocks (the <section class="code-snippet..."> wrapper
	// contains <pre>/<code>), inline code spans, and <img> tags (whose
	// title/alt attributes could contain $).
	reSection = regexp.MustCompile(`(?s)<section class="code-snippet[^>]*>.*?</section>`)
	rePre     = regexp.MustCompile(`(?s)<pre[^>]*>.*?</pre>`)
	reCode    = regexp.MustCompile(`(?s)<code[^>]*>.*?</code>`)
	reImg     = regexp.MustCompile(`<img[^>]*>`)
)

// inlineStyle/displayStyle are applied to PNG formula <img> tags.
const (
	inlineStyle  = "display:inline-block;vertical-align:middle;max-width:100%"
	displayStyle = "display:block;margin:0.8em auto;max-width:100%"
)

// placeholderPrefix separates protected regions from live text.
const placeholderPrefix = "\x00slingshot-formula-"

// processor carries the state of one ProcessMath call.
type processor struct {
	outDir string
	mode   Mode
	stderr io.Writer
	// render injects one formula (svg/img fragment); nil means no engine.
	render func(tex string, display bool) (string, error)
	// placeholders holds extracted regions in insertion order; restore
	// replaces them back after formula substitution.
	placeholders []string
}

// ProcessMath replaces LaTeX formulas in WeChat HTML with rendered SVG
// fragments or <img> tags.
//
// mode selects the behavior (see Mode). ModeAuto uses the best engine
// available (SVG → PNG), keeping the LaTeX text with a warning when none is;
// ModeSVG/ModePNG return an error when their engine is missing. Failed
// individual renders (syntax errors) always degrade to keeping the original
// text with a warning — one broken formula must not block publishing.
//
// The function is idempotent: already-processed HTML contains no formula
// delimiters, so a second pass is a no-op.
func ProcessMath(htmlData []byte, baseDir string, mode Mode, stderr io.Writer) ([]byte, error) {
	if mode == ModeText {
		return htmlData, nil
	}

	var render func(tex string, display bool) (string, error)
	switch mode {
	case ModeSVG:
		if err := svgEngineAvailable(); err != nil {
			return htmlData, fmt.Errorf("svg math engine unavailable: %w", err)
		}
		render = renderSVG
	case ModePNG:
		if err := pngEnginesAvailable(); err != nil {
			return htmlData, fmt.Errorf("png math engine unavailable: %w", err)
		}
		render = func(tex string, display bool) (string, error) {
			return renderPNG(tex, display, baseDir)
		}
	default: // ModeAuto
		svgErr := svgEngineAvailable()
		pngErr := pngEnginesAvailable()
		switch {
		case svgErr == nil:
			render = func(tex string, display bool) (string, error) {
				// SVG first; fall back to PNG for formulas MathJax cannot
				// handle (e.g. exotic packages).
				if s, err := renderSVG(tex, display); err == nil {
					return s, nil
				} else if p, perr := renderPNG(tex, display, baseDir); perr == nil {
					return p, nil
				} else {
					return "", err
				}
			}
		case pngErr == nil:
			render = func(tex string, display bool) (string, error) {
				return renderPNG(tex, display, baseDir)
			}
		default:
			fmt.Fprintf(stderr, "Warning: formula rendering unavailable (svg: %v; png: %v); keeping LaTeX text\n", svgErr, pngErr)
			return htmlData, nil
		}
	}

	p := &processor{outDir: baseDir, mode: mode, stderr: stderr, render: render}
	return p.run(htmlData), nil
}

// run protects code/image regions, substitutes formulas, restores regions.
func (p *processor) run(data []byte) []byte {
	if p.mode == ModeText {
		return data
	}
	data = p.protect(data)
	data = p.substitute(data)
	return p.restore(data)
}

// protect extracts regions whose content must not be treated as formulas
// (code blocks, code spans, existing <img> tags) and replaces them with
// unique placeholders.
func (p *processor) protect(data []byte) []byte {
	for _, re := range []*regexp.Regexp{reSection, rePre, reCode, reImg} {
		data = re.ReplaceAllFunc(data, func(m []byte) []byte {
			return []byte(p.hold(string(m)))
		})
	}
	return data
}

// hold registers a protected region and returns its placeholder.
func (p *processor) hold(region string) string {
	ph := fmt.Sprintf("%s%d\x00", placeholderPrefix, len(p.placeholders))
	p.placeholders = append(p.placeholders, region)
	return ph
}

// restore puts protected regions back.
func (p *processor) restore(data []byte) []byte {
	for i := len(p.placeholders) - 1; i >= 0; i-- {
		ph := fmt.Sprintf("%s%d\x00", placeholderPrefix, i)
		data = bytesReplaceAll(data, []byte(ph), []byte(p.placeholders[i]))
	}
	return data
}

// substitute runs the formula matchers in order: display forms first (so
// multi-char delimiters win), then inline forms. The body passed to the
// renderer is the raw formula content (delimiters stripped); each renderer
// wraps it in its own syntax. equation loses its environment (rendered
// without numbers); other environments (align, gather, ...) stay verbatim.
func (p *processor) substitute(data []byte) []byte {
	data = reBlockEnv.ReplaceAllFunc(data, func(m []byte) []byte {
		inner := string(reBlockEnv.FindSubmatch(m)[1])
		if isEquationEnv(m) {
			return []byte(p.replace(string(m), inner, true))
		}
		return []byte(p.replace(string(m), string(m), true))
	})
	// $$...$$ and \[...\] → plain content.
	data = reBlockDollar.ReplaceAllFunc(data, func(m []byte) []byte {
		body := reBlockDollar.FindSubmatch(m)
		return []byte(p.replace(string(m), string(body[1]), true))
	})
	data = reBlockBracket.ReplaceAllFunc(data, func(m []byte) []byte {
		body := reBlockBracket.FindSubmatch(m)
		return []byte(p.replace(string(m), string(body[1]), true))
	})
	// Inline forms.
	data = reInlineDollar.ReplaceAllFunc(data, func(m []byte) []byte {
		body := reInlineDollar.FindSubmatch(m)
		return []byte(p.replace(string(m), string(body[1]), false))
	})
	data = reInlineParen.ReplaceAllFunc(data, func(m []byte) []byte {
		body := reInlineParen.FindSubmatch(m)
		return []byte(p.replace(string(m), string(body[1]), false))
	})
	return data
}

// isEquationEnv reports whether the matched \begin{...} block is an
// equation-style environment (rendered without numbers).
func isEquationEnv(m []byte) bool {
	sm := reEnvName.FindSubmatch(m)
	if len(sm) < 2 {
		return false
	}
	name := string(sm[1])
	return name == "equation" || name == "equation*"
}

// replace is the common substitution path: clean the formula, render it via
// the active engine, return the injected fragment. On any failure the
// original text is kept.
func (p *processor) replace(match, body string, display bool) string {
	// The body came from HTML: entities like &lt; must become < for the
	// TeX/MathJax input.
	body = html.UnescapeString(body)
	// Soft line breaks inside a paragraph become spaces; collapse runs.
	body = strings.Join(strings.Fields(body), " ")
	if body == "" {
		return match // empty formula — leave as-is
	}
	if p.render == nil {
		return match // no engine (ModeAuto degraded)
	}
	inject, err := p.render(body, display)
	if err != nil {
		fmt.Fprintf(p.stderr, "Warning: formula render failed (%s): %v; keeping original text\n", body, err)
		return match
	}
	// Hold the injected fragment behind a placeholder so later matchers in
	// this pass (and the restore step) cannot re-process it — a stub or
	// exotic renderer could otherwise echo formula delimiters back.
	return p.hold(inject)
}

// bytesReplaceAll is strings.ReplaceAll for []byte.
func bytesReplaceAll(s, old, new []byte) []byte {
	return []byte(strings.ReplaceAll(string(s), string(old), string(new)))
}

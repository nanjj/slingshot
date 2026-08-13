package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/fatih/color"
	"github.com/spf13/cobra"

	cli "github.com/nanjj/slingshot/internal/cmd"
	"github.com/nanjj/slingshot/internal/i18n"
	u "github.com/nanjj/slingshot/internal/usage"
)

// tikz 子命令语法: tikz <in-file> <out-file>
var tikzUsage = u.Usage{
	u.File, // <in-file>
	u.File, // <out-file>
}

// cmdTikz 实现 "slingshot tikz" 子命令: 把 TikZ 片段渲染成图片。
type cmdTikz struct {
	global *cmdGlobal
}

func (c *cmdTikz) command() *cobra.Command {
	cmd := &cobra.Command{}
	cmd.Use = "tikz " + u.File.Render() + " " + u.File.Render()
	cmd.Short = i18n.G("Render a TikZ snippet to png/jpg/svg/pdf")
	cmd.Long = cli.FormatSection(
		color.CyanString("Description:"),
		i18n.G(`Render a TikZ snippet to an image file (.png / .jpg / .svg / .pdf).

The input file may contain a full tikzpicture environment or just the
commands inside it — a tikzpicture wrapper is added automatically when
missing. \usetikzlibrary lines are hoisted above the environment.

Packages are loaded automatically by content detection (tkz-euclide,
tikz-cd, pgfplots, circuitikz, forest, ...); explicit \usepackage lines
in the input are hoisted into the preamble.

The output format is determined by the output file extension.
Pipeline: tectonic -> PDF -> mutool / ghostscript rasterization.
Chinese (CJK) text is supported via xeCJK; the font can be overridden
with the TIKZ_CJK_FONT environment variable (default: Noto Sans CJK SC).

Examples:
  slingshot tikz fig.tikz fig.png
  slingshot tikz fig.tikz fig.svg
  slingshot tikz fig.tikz fig.pdf`),
	)
	cmd.RunE = c.run
	cmd.Args = cobra.ArbitraryArgs
	return cmd
}

func (c *cmdTikz) run(cmd *cobra.Command, args []string) error {
	parsed, err := c.global.Parse(tikzUsage, cmd, args)
	if err != nil {
		return err
	}
	if len(parsed) < 2 || parsed[0].Skipped || parsed[1].Skipped {
		return errors.New(i18n.G("expected IN-FILE and OUT-FILE arguments"))
	}
	inFile := parsed[0].String
	outFile := parsed[1].String
	if err := renderTikz(inFile, outFile); err != nil {
		return err
	}
	fmt.Fprintf(color.Output, "%s %s\n", i18n.G("Wrote"), color.GreenString(outFile))
	return nil
}

// tikzWrapper 是 tectonic 编译用的 standalone 模板。
// 四个 %s 依次为: 额外加载的包、自动探测的 tikz 库、兼容 shim、CJK 字体名
// (后两者可为空; 字体由 TIKZ_CJK_FONT 控制, 默认 Noto Sans CJK SC)。
// \usetikzlibrary 放在所有 \usepackage 之后、\begin{document} 之前,
// 确保 fit / calc 等库在输入内容执行前生效。
const tikzWrapper = `\documentclass[border=2pt]{standalone}
\usepackage{tikz}
\usepackage{xcolor}
\usepackage{amsmath}
%s%s%s\usepackage{fontspec}
\usepackage{xeCJK}
\setCJKmainfont{%s}
\begin{document}
\input{input.tikz}
\end{document}
`

// tikzBuzzerShim 为旧版 circuitikz 补齐 buzzer / rbuzzer 双端元件。
// buzzer (蜂鸣器) 是 circuitikz 1.5.0 才加入的元件, tectonic bundle 自带的
// 1.4.6 不认识 /tikz/buzzer key; 底层的 \pgfcircdeclarebipolescaled /
// \pgfcirc@activate@bipole@simple 宏 1.4.6 已有, 因此直接复用新版定义即可。
// 双重守卫: \ifcsname ctikzset 要求 circuitikz 已加载;
// \pgfkeysifdefined{/tikz/buzzer} 保证未来 bundle 升级后不重复定义。
const tikzBuzzerShim = `\makeatletter
\ifcsname ctikzset\endcsname
\pgfkeysifdefined{/tikz/buzzer}{}{%
\ctikzset{bipoles/buzzer/height/.initial=0.6}
\ctikzset{bipoles/buzzer/width/.initial=.4}%
\ctikzset{bipoles/buzzer/span/.initial=.6}%

\pgfcircdeclarebipolescaled{misc}
{}
{0}
{buzzer}
{\ctikzvalof{bipoles/buzzer/height}}
{\ctikzvalof{bipoles/buzzer/width}}{
    \pgf@circ@res@other=\dimexpr\pgf@circ@res@up-\pgf@circ@res@right\relax
    \pgfscope
        \pgf@circ@setlinewidth{bipoles}{\pgfstartlinewidth}
        \pgfpathmoveto{\pgfpoint{\pgf@circ@res@left}{\pgf@circ@res@other}}
        \pgfpathlineto{\pgfpoint{\pgf@circ@res@right}{\pgf@circ@res@other}}
        \pgfpatharc{0}{180}{\pgf@circ@res@right}
        \pgfpathclose
        \pgf@circ@draworfill
    \endpgfscope
    \pgfpathmoveto{\pgfpoint{\ctikzvalof{bipoles/buzzer/span}*\pgf@circ@res@left}{\pgf@circ@res@other}}
    \pgfpathlineto{\pgfpoint{\ctikzvalof{bipoles/buzzer/span}*\pgf@circ@res@left}{0pt}}
    \pgfpathlineto{\pgfpoint{\pgf@circ@res@left}{0pt}}
    \pgfpathmoveto{\pgfpoint{\ctikzvalof{bipoles/buzzer/span}*\pgf@circ@res@right}{\pgf@circ@res@other}}
    \pgfpathlineto{\pgfpoint{\ctikzvalof{bipoles/buzzer/span}*\pgf@circ@res@right}{0pt}}
    \pgfpathlineto{\pgfpoint{\pgf@circ@res@right}{0pt}}
    \pgfusepath{draw}
}
\pgfcirc@activate@bipole@simple{l}{buzzer}

\pgfcircdeclarebipolescaled{misc}
{}
{0}
{rbuzzer}
{\ctikzvalof{bipoles/buzzer/height}}
{\ctikzvalof{bipoles/buzzer/width}}{
    \pgf@circ@res@other=\dimexpr\pgf@circ@res@up-\pgf@circ@res@right\relax
    \pgfmathsetlength\pgf@circ@res@temp{\pgf@circ@res@up-
        \pgf@circ@res@right*sqrt(1-\ctikzvalof{bipoles/buzzer/span}*\ctikzvalof{bipoles/buzzer/span})}
    \pgfscope
        \pgf@circ@setlinewidth{bipoles}{\pgfstartlinewidth}
        \pgfpathmoveto{\pgfpoint{\pgf@circ@res@left}{\pgf@circ@res@up}}
        \pgfpathlineto{\pgfpoint{\pgf@circ@res@right}{\pgf@circ@res@up}}
        \pgfpatharc{0}{-180}{\pgf@circ@res@right}
        \pgfpathclose
        \pgf@circ@draworfill
    \endpgfscope
    \pgfpathmoveto{\pgfpoint{\ctikzvalof{bipoles/buzzer/span}*\pgf@circ@res@left}{\pgf@circ@res@temp}}
    \pgfpathlineto{\pgfpoint{\ctikzvalof{bipoles/buzzer/span}*\pgf@circ@res@left}{0pt}}
    \pgfpathlineto{\pgfpoint{\pgf@circ@res@left}{0pt}}
    \pgfpathmoveto{\pgfpoint{\ctikzvalof{bipoles/buzzer/span}*\pgf@circ@res@right}{\pgf@circ@res@temp}}
    \pgfpathlineto{\pgfpoint{\ctikzvalof{bipoles/buzzer/span}*\pgf@circ@res@right}{0pt}}
    \pgfpathlineto{\pgfpoint{\pgf@circ@res@right}{0pt}}
    \pgfusepath{draw}
}
\pgfcirc@activate@bipole@simple{l}{rbuzzer}
}
\fi
\makeatother`

// tikzSiunitxShim 为 siunitx v3 补齐在 \si 外独立使用单位命令的能力。
// v3 中 \micro 等单位命令只在 \si{...} 内可用, 单独出现会展开为 \ERROR
// (circuitikz 标签 l=10<\micro\farad> 是 siunitx v2 时代的惯用写法)。
// 前缀命令 (micro/nano/...) 直接输出符号, 单位命令 (farad/ohm/...) 委托
// \si{...} 排版; \si / \SI 正规用法不受影响 (v3 在 \si 内部会临时覆盖定义)。
// 双重守卫: \ifcsname 分支兼容已定义 (v2/v3) 与未定义两种情况。
const tikzSiunitxShim = `\makeatletter
\newcommand{\tikzsiunitx@alias}[2]{%
  \ifcsname #1\endcsname
    \expandafter\RenewDocumentCommand\csname #1\endcsname{}{#2}%
  \else
    \expandafter\NewDocumentCommand\csname #1\endcsname{}{#2}%
  \fi}
\tikzsiunitx@alias{micro}{\ensuremath{\mathrm{\mu}}}
\tikzsiunitx@alias{nano}{\ensuremath{\mathrm{n}}}
\tikzsiunitx@alias{pico}{\ensuremath{\mathrm{p}}}
\tikzsiunitx@alias{milli}{\ensuremath{\mathrm{m}}}
\tikzsiunitx@alias{kilo}{\ensuremath{\mathrm{k}}}
\tikzsiunitx@alias{mega}{\ensuremath{\mathrm{M}}}
\tikzsiunitx@alias{giga}{\ensuremath{\mathrm{G}}}
\tikzsiunitx@alias{farad}{\si{\farad}}
\tikzsiunitx@alias{ohm}{\si{\ohm}}
\tikzsiunitx@alias{henry}{\si{\henry}}
\tikzsiunitx@alias{volt}{\si{\volt}}
\tikzsiunitx@alias{ampere}{\si{\ampere}}
\tikzsiunitx@alias{metre}{\si{\metre}}
\tikzsiunitx@alias{second}{\si{\second}}
\tikzsiunitx@alias{hertz}{\si{\hertz}}
\makeatother
`

// siunitxAliasShim 检测到 siunitx 包时返回兼容 shim, 否则返回空串。
func siunitxAliasShim(pkgs []string) string {
	for _, p := range pkgs {
		if p == "siunitx" {
			return tikzSiunitxShim
		}
	}
	return ""
}

// buzzerBipoleRe 匹配 circuitikz 的 buzzer / rbuzzer 双端元件用法。
var buzzerBipoleRe = regexp.MustCompile(`to\[\s*r?buzzer\b`)

// circuitikzBuzzerShim 检测输入是否用到 buzzer 元件, 命中时返回兼容 shim。
// 注释行中的用法不触发 (LaTeX 中 % 到行尾都是注释)。
func circuitikzBuzzerShim(content string) string {
	for _, m := range buzzerBipoleRe.FindAllStringIndex(content, -1) {
		lineStart := strings.LastIndex(content[:m[0]], "\n") + 1
		if strings.Contains(content[lineStart:m[0]], "%") {
			continue
		}
		return tikzBuzzerShim + "\n"
	}
	return ""
}

// tikzExtraPackages 是 "内容特征 → 需要额外加载的 LaTeX 包" 探测表。
// 顺序即 \usepackage 的加载顺序; 命中特征说明 tikzpicture 用到了该包的命令/环境。
var tikzExtraPackages = []struct{ marker, pkg string }{
	{`\tkz`, "tkz-euclide"}, // \tkzDefPoint, \tkzDrawPoints, \tkzLabelPoints 等
	{`\begin{tikzcd}`, "tikz-cd"},
	{`\tikzcdset`, "tikz-cd"},
	{`\begin{axis}`, "pgfplots"},
	{`\begin{semilogxaxis}`, "pgfplots"},
	{`\begin{semilogyaxis}`, "pgfplots"},
	{`\begin{loglogaxis}`, "pgfplots"},
	{`\begin{polaraxis}`, "pgfplots"},
	{`\addplot`, "pgfplots"},
	{`\pgfplotsset`, "pgfplots"},
	{`\begin{circuitikz}`, "circuitikz"},
	{`\begin{forest}`, "forest"},
	{`\tdplotsetmaincoords`, "tikz-3dplot"},
	{`\tdplotsetrotatedcoords`, "tikz-3dplot"},
	{`\smartdiagram`, "smartdiagram"},
	{`\begin{venndiagram}`, "venndiagram"},
}

// tikzExtraLibraries 是 "内容特征 → 需要额外加载的 tikz 库" 探测表。
// 与 tikzExtraPackages 不同, 库特征更微妙 (fit= / ($ / right=of 等),
// 因此用正则而非子串匹配; 顺序即 \usetikzlibrary 的加载顺序。
// circuitikz 会顺带加载 calc / arrows.meta / bending / fpu,
// 但 fit / positioning / patterns 等库必须显式加载。
var tikzExtraLibraries = []struct {
	re  *regexp.Regexp
	lib string
}{
	{regexp.MustCompile(`\bfit\s*=`), "fit"}, // node[fit=(a)(b)] 包围盒
	{regexp.MustCompile(`\(\$`), "calc"},     // ($(a)!0.5!(b)$) 坐标运算
	{regexp.MustCompile(`\b(?:above|below|left|right)\s*=\s*of\b`), "positioning"},
	{regexp.MustCompile(`-\{?(?:Stealth|Latex|Triangle|Circle|Square|Diamond|Kite|To)\b`), "arrows.meta"},
	{regexp.MustCompile(`-\{?(?:stealth|latex|to|triangle)\b`), "arrows"},
	{regexp.MustCompile(`\bpattern\s*=`), "patterns"},
	{regexp.MustCompile(`\bdecorate\b|\bdecoration\s*=`), "decorations.pathreplacing"},
	{regexp.MustCompile(`\bsnake\b`), "decorations.pathmorphing"},
	{regexp.MustCompile(`\bname\s+intersections\b`), "intersections"},
	{regexp.MustCompile(`(?:to|edge)\s*\["`), "quotes"},
	{regexp.MustCompile(`node\s*\[[^\]]*\b(?:ellipse|diamond|cylinder|regular\s+polygon|star|cloud|trapezium)\b`), "shapes.geometric"},
}

// detectTikzLibraries 从输入内容推断需要的 tikz 库, 按表顺序收集、去重。
func detectTikzLibraries(content string) []string {
	var libs []string
	seen := make(map[string]bool)
	for _, e := range tikzExtraLibraries {
		if !e.re.MatchString(content) || seen[e.lib] {
			continue
		}
		seen[e.lib] = true
		libs = append(libs, e.lib)
	}
	return libs
}

// tikzLibraryLines 把库名列表渲染成 \usetikzlibrary 行; 空列表返回空串。
func tikzLibraryLines(libs []string) string {
	if len(libs) == 0 {
		return ""
	}
	return `\usetikzlibrary{` + strings.Join(libs, ",") + "}\n"
}

// tikzExtraPackageRes 是正则匹配的额外包探测——子串匹配无法精确表达的条目。
// \up 前缀的直立希腊字母 (\upalpha / \upmu 等) 由 upgreek 包提供,
// 但 \uparrow 等是 LaTeX 内核符号, 不能按 "\up" 子串一概而论。
var tikzExtraPackageRes = []struct {
	re  *regexp.Regexp
	pkg string
}{
	// siunitx: \micro \farad \ohm 等前缀/单位命令只能由 siunitx 提供,
	// circuitikz 标签 l=10<\micro\farad> 是典型用法。\b 边界排除内核命令:
	// \sin \sigma \sim 不是 \si, \number \numexpr 不是 \num, \unitlength 不是 \unit。
	{regexp.MustCompile(`\\(?:micro|nano|pico|milli|kilo|mega|giga|farad|ohm|henry|volt|ampere|metre|second|hertz)\b`), "siunitx"},
	{regexp.MustCompile(`\\(?:SI|si|num|qty|unit|sisetup)\b`), "siunitx"},
	// \up 前缀的直立希腊字母 (\upalpha / \upmu 等) 由 upgreek 包提供,
	// 但 \uparrow 等是 LaTeX 内核符号, 不能按 "\up" 子串一概而论。
	{regexp.MustCompile(`\\up(?:alpha|beta|gamma|delta|varepsilon|zeta|eta|theta|vartheta|iota|kappa|lambda|mu|nu|xi|omicron|pi|varpi|rho|varrho|sigma|varsigma|tau|upsilon|phi|varphi|chi|psi|omega)\b`), "upgreek"},
}

// detectTikzPackages 从输入内容推断需要的额外包: 命中特征的包按表顺序收集, 去重。
func detectTikzPackages(content string) []string {
	var pkgs []string
	seen := make(map[string]bool)
	for _, e := range tikzExtraPackages {
		if !strings.Contains(content, e.marker) || seen[e.pkg] {
			continue
		}
		seen[e.pkg] = true
		pkgs = append(pkgs, e.pkg)
	}
	for _, e := range tikzExtraPackageRes {
		if !e.re.MatchString(content) || seen[e.pkg] {
			continue
		}
		seen[e.pkg] = true
		pkgs = append(pkgs, e.pkg)
	}
	return pkgs
}

// userPackageRe 匹配行首的 \usepackage{...} (可带可选参数), 连同行尾换行。
var userPackageRe = regexp.MustCompile(`(?m)^[ \t]*\\usepackage(?:\[[^]]*\])?\{([^}]*)\}[ \t]*\r?\n?`)

// extractUserPackages 提取输入中显式出现的 \usepackage 行并提升到 preamble,
// 返回提取的包名列表与剩余内容——未知包可以通过这个兜底入口加载。
func extractUserPackages(content string) ([]string, string) {
	var pkgs []string
	for _, m := range userPackageRe.FindAllStringSubmatch(content, -1) {
		for name := range strings.SplitSeq(m[1], ",") {
			if name = strings.TrimSpace(name); name != "" {
				pkgs = append(pkgs, name)
			}
		}
	}
	return pkgs, userPackageRe.ReplaceAllString(content, "")
}

// mergeTikzPackages 合并显式包与自动探测包, 保持顺序并去重 (显式优先)。
func mergeTikzPackages(explicit, detected []string) []string {
	seen := make(map[string]bool)
	var pkgs []string
	for _, p := range append(explicit, detected...) {
		if !seen[p] {
			seen[p] = true
			pkgs = append(pkgs, p)
		}
	}
	return pkgs
}

// tikzPackageLines 把包名列表渲染成 \usepackage 行; 空列表返回空串。
func tikzPackageLines(pkgs []string) string {
	if len(pkgs) == 0 {
		return ""
	}
	lines := make([]string, len(pkgs))
	for i, p := range pkgs {
		lines[i] = `\usepackage{` + p + `}`
	}
	return strings.Join(lines, "\n") + "\n"
}

// 匹配 \usetikzlibrary{...} 及其行尾换行, 避免替换后留下空行。
var usetikzlibraryRe = regexp.MustCompile(`\\usetikzlibrary\{[^}]*\}\r?\n?`)

// tikzSelfContainedEnvs 是自身即为完整 TikZ 环境的顶层环境。
// circuitikz / tikzcd / forest 内部都会再开 tikzpicture，
// 把它们包进外层 tikzpicture 会造成嵌套错误 (内容缺失/布局错乱)。
// 注意: axis / venndiagram 等不是自包含环境, 必须在 tikzpicture 内, 不能加入。
var tikzSelfContainedEnvs = []string{
	`\begin{tikzpicture}`,
	`\begin{circuitikz}`,
	`\begin{tikzcd}`,
	`\begin{forest}`,
}

// normalizeTikz 保证输入包含完整的环境:
// 已有自包含环境 (tikzpicture / circuitikz / tikzcd / forest) 则原样返回;
// 否则补一层 tikzpicture, 并把 \usetikzlibrary 提到环境外
// (它在 document body 中有效, 但在 tikzpicture 内行为不受保证)。
func normalizeTikz(content string) string {
	for _, env := range tikzSelfContainedEnvs {
		if strings.Contains(content, env) {
			return content
		}
	}
	var libs []string
	content = usetikzlibraryRe.ReplaceAllStringFunc(content, func(m string) string {
		// m 可能带行尾换行 (被正则吃掉), 存库时去掉, Join 时统一补 \n。
		libs = append(libs, strings.TrimRight(m, "\r\n"))
		return ""
	})
	head := ""
	if len(libs) > 0 {
		head = strings.Join(libs, "\n") + "\n"
	}
	return head + "\\begin{tikzpicture}\n" + content + "\n\\end{tikzpicture}\n"
}

// tikzOutputFormat 从输出文件扩展名解析目标格式。
func tikzOutputFormat(outFile string) (string, error) {
	ext := strings.ToLower(filepath.Ext(outFile))
	switch ext {
	case ".png", ".jpg", ".jpeg", ".svg", ".pdf":
		if ext == ".jpeg" {
			ext = ".jpg"
		}
		return strings.TrimPrefix(ext, "."), nil
	}
	return "", fmt.Errorf("unsupported output format %q (want .png, .jpg, .svg or .pdf)", filepath.Ext(outFile))
}

// renderTikz 渲染 inFile 为 outFile 指定格式的图片。
// 在临时目录中工作 (tectonic 会产出 aux 文件); 失败时保留现场便于调试。
func renderTikz(inFile, outFile string) (err error) {
	format, err := tikzOutputFormat(outFile)
	if err != nil {
		return err
	}

	content, err := os.ReadFile(inFile)
	if err != nil {
		return fmt.Errorf("reading input: %w", err)
	}
	// 提升显式 \usepackage + 自动探测内容所需的包和 tikz 库
	// (tkz-euclide / circuitikz / fit / calc 等)。
	raw := string(content)
	explicitPkgs, raw := extractUserPackages(raw)
	pkgs := mergeTikzPackages(explicitPkgs, detectTikzPackages(raw))
	libs := detectTikzLibraries(raw)
	outAbs, err := filepath.Abs(outFile)
	if err != nil {
		return fmt.Errorf("resolving output path: %w", err)
	}

	tmpDir, err := os.MkdirTemp("", "slingshot-tikz-")
	if err != nil {
		return fmt.Errorf("creating temp dir: %w", err)
	}
	// 成功时清理, 失败时保留现场。
	defer func() {
		if err == nil {
			os.RemoveAll(tmpDir)
		}
	}()

	font := os.Getenv("TIKZ_CJK_FONT")
	if font == "" {
		font = "Noto Sans CJK SC"
	}
	if err := os.WriteFile(filepath.Join(tmpDir, "input.tikz"),
		[]byte(normalizeTikz(raw)), 0644); err != nil {
		return fmt.Errorf("writing input.tikz: %w", err)
	}
	if err := os.WriteFile(filepath.Join(tmpDir, "input.tex"),
		fmt.Appendf(nil, tikzWrapper, tikzPackageLines(pkgs),
			tikzLibraryLines(libs), circuitikzBuzzerShim(raw)+siunitxAliasShim(pkgs), font), 0644); err != nil {
		return fmt.Errorf("writing input.tex: %w", err)
	}

	// tectonic input.tex -> input.pdf
	if err := runCmd(tmpDir, "tectonic", "input.tex"); err != nil {
		return fmt.Errorf("tectonic failed (workdir kept: %s): %w", tmpDir, err)
	}

	pdf := filepath.Join(tmpDir, "input.pdf")
	base := filepath.Join(tmpDir, "out")
	var produced string
	switch format {
	case "pdf":
		produced = pdf
	case "png":
		if err := runCmd(tmpDir, "mutool", "draw", "-o", base+".png", "-r", "150", "input.pdf"); err != nil {
			return fmt.Errorf("mutool draw failed (workdir kept: %s): %w", tmpDir, err)
		}
		produced = resolveMutoolOutput(base + ".png")
	case "svg":
		if err := runCmd(tmpDir, "mutool", "convert", "-o", base+".svg", "input.pdf"); err != nil {
			return fmt.Errorf("mutool convert failed (workdir kept: %s): %w", tmpDir, err)
		}
		produced = resolveMutoolOutput(base + ".svg")
	case "jpg":
		if err := runCmd(tmpDir, "gs", "-sDEVICE=jpeg", "-r150", "-o", base+".jpg", "input.pdf"); err != nil {
			return fmt.Errorf("ghostscript failed (workdir kept: %s): %w", tmpDir, err)
		}
		produced = resolveMutoolOutput(base + ".jpg")
	}
	if produced == "" {
		return fmt.Errorf("conversion produced no output (workdir kept: %s)", tmpDir)
	}

	// 把产物复制到最终输出路径 (临时目录可能跨文件系统, 用 copy 而非 rename)。
	in, err := os.Open(produced)
	if err != nil {
		return fmt.Errorf("opening result: %w", err)
	}
	defer in.Close()
	out, err := os.Create(outAbs)
	if err != nil {
		return fmt.Errorf("creating output: %w", err)
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return fmt.Errorf("writing output: %w", err)
	}
	return out.Close()
}

// runCmd 在 dir 中执行命令, 失败时返回包含输出的错误。
// LaTeX 引擎的日志可能很长, 只保留尾部 (错误汇总通常在末尾)。
func runCmd(dir, name string, args ...string) error {
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		msg := strings.TrimSpace(string(out))
		if len(msg) > 2000 {
			msg = "..." + msg[len(msg)-2000:]
		}
		return fmt.Errorf("%s: %v\n%s", name, err, msg)
	}
	return nil
}

// resolveMutoolOutput 处理 mutool 的页码后缀命名:
// draw 单页输出原名, 多页输出 OUT-1.png; convert 总是输出 OUT1.svg。
func resolveMutoolOutput(expected string) string {
	if fileExists(expected) {
		return expected
	}
	ext := filepath.Ext(expected)
	stem := strings.TrimSuffix(expected, ext)
	for _, cand := range []string{
		fmt.Sprintf("%s1%s", stem, ext),
		fmt.Sprintf("%s-1%s", stem, ext),
	} {
		if fileExists(cand) {
			return cand
		}
	}
	return ""
}

func fileExists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}

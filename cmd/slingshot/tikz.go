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

// tikzMotorShim 为旧版 circuitikz 补齐 motor (电动机: 圆圈 + M) 双端元件。
// circuitikz 只有 elmech (矩形穿圆) 的机电抽象符号, 没有教科书式
// "圆圈内一个 M" 的电动机符号; 以 ammeter (圆 + A, 仪表) 为模板定制:
// 左右接线 + 圆圈 + 加粗 M, 去掉 ammeter 的指针箭头。
// 双重守卫: \ifcsname ctikzset 要求 circuitikz 已加载;
// \pgfkeysifdefined{/tikz/motor} 保证未来 bundle 升级后不重复定义。
const tikzMotorShim = `\makeatletter
\ifcsname ctikzset\endcsname
\pgfkeysifdefined{/tikz/motor}{}{%
\ctikzset{bipoles/motor/height/.initial=.60}
\ctikzset{bipoles/motor/width/.initial=.60}%

\pgfcircdeclarebipolescaled{instruments}
{}
{\ctikzvalof{bipoles/motor/height}}
{motor}
{\ctikzvalof{bipoles/motor/height}}
{\ctikzvalof{bipoles/motor/width}}
{
    % draw connections to circle
    \pgfpathmoveto{\pgfpoint{\pgf@circ@res@left}{\pgf@circ@res@zero}}
    \pgfpathlineto{\pgfpoint{.9\pgf@circ@res@left}{\pgf@circ@res@zero}}
    \pgfpathmoveto{\pgfpoint{.9\pgf@circ@res@right}{\pgf@circ@res@zero}}
    \pgfpathlineto{\pgfpoint{\pgf@circ@res@right}{\pgf@circ@res@zero}}
    \pgfusepath{draw}
    % draw circle
    \pgfscope
        \pgf@circ@setlinewidth{bipoles}{\pgfstartlinewidth}
        \pgfpathcircle{\pgfpointorigin}{.9\pgf@circ@res@up}
        \pgf@circ@draworfill
    \endpgfscope
    % draw M
    \pgfnode{circle}{center}{\pgf@circ@font@bold{M}}{}{}
}
\pgfcirc@activate@bipole@simple{l}{motor}
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

// motorBipoleRe 匹配 circuitikz 的 motor 双端元件用法。
// circuitikz 上游没有 motor 元件 (圆 + M), 由 tikzMotorShim 定制补齐。
var motorBipoleRe = regexp.MustCompile(`to\[\s*motor\b`)

// circuitikzMotorShim 检测输入是否用到 motor 元件, 命中时返回定制 shim。
// 注释行中的用法不触发 (LaTeX 中 % 到行尾都是注释)。
func circuitikzMotorShim(content string) string {
	for _, m := range motorBipoleRe.FindAllStringIndex(content, -1) {
		lineStart := strings.LastIndex(content[:m[0]], "\n") + 1
		if strings.Contains(content[lineStart:m[0]], "%") {
			continue
		}
		return tikzMotorShim + "\n"
	}
	return ""
}

// circuitEeIECRe 匹配 TikZ IEC 电路风格 tikzpicture[circuit ee IEC]。
var circuitEeIECRe = regexp.MustCompile(`\bcircuit\s+ee\s+IEC\b`)

// tikzIecShim 为 tectonic bundle 补齐 TikZ circuits.ee.IEC 库的 IEC 风格。
// bundle 不含该库 (tikzlibrarycircuits.ee.IEC.code.tex 缺失), 但 circuitikz
// 元件可以等价表达 IEC 符号: resistor→欧标矩形 (generic), diode→D,
// capacitor→C, ... 元件 {name=R} 里的内容在 TikZ IEC 库中是节点选项
// (命名节点), 所以 IEC 样式先以空标签展开后端, 再把参数当键列表原样传入。
// 后端统一使用私有名 tikzcirciec@*, 普通 / [compatibility] 两种 circuitikz
// 模式下内部路径宏都可用, 与自带的 * 样式互不干扰。
// \pgfkeysifdefined{/tikz/circuit ee IEC} 守卫: 未来 bundle 带上真库时不覆盖。
// 注意 \pgf@circ@emptydiode@path 是 1.4.6 默认 (empty diode) 的路径宏,
// 即空心三角+横杠, 与 IEC 二极管一致; 其余路径宏由 activate@bipole 动态创建。
const tikzIecShim = `\makeatletter
\pgfkeysifdefined{/tikz/circuit ee IEC}{}{%
  \tikzset{
    tikzcirciec@resistor/.style={\circuitikzbasekey, /tikz/to path=\pgf@circ@bipole@path{generic}, l={#1}},
    tikzcirciec@var resistor/.style={\circuitikzbasekey, /tikz/to path=\pgf@circ@bipole@path{tgeneric}, l={#1}},
    tikzcirciec@diode/.style={\circuitikzbasekey, /tikz/to path=\pgf@circ@emptydiode@path, l={#1}},
    tikzcirciec@inductor/.style={\circuitikzbasekey, /tikz/to path=\pgf@circ@inductor@path, l={#1}},
    tikzcirciec@capacitor/.style={\circuitikzbasekey, /tikz/to path=\pgf@circ@capacitor@path, l={#1}},
    tikzcirciec@battery/.style={\circuitikzbasekey, /tikz/to path=\pgf@circ@battery@path, l={#1}},
    tikzcirciec@bulb/.style={\circuitikzbasekey, /tikz/to path=\pgf@circ@bulb@path, l={#1}},
    tikzcirciec@ammeter/.style={\circuitikzbasekey, /tikz/to path=\pgf@circ@ammeter@path, l={#1}},
    tikzcirciec@voltmeter/.style={\circuitikzbasekey, /tikz/to path=\pgf@circ@voltmeter@path, l={#1}},
    tikzcirciec@ohmmeter/.style={\circuitikzbasekey, /tikz/to path=\pgf@circ@ohmmeter@path, l={#1}},
  }%
  \tikzset{
    circuit ee IEC/.style={},
    resistor/.style={tikzcirciec@resistor={}, #1},
    var resistor/.style={tikzcirciec@var resistor={}, #1},
    diode/.style={tikzcirciec@diode={}, #1},
    inductor/.style={tikzcirciec@inductor={}, #1},
    capacitor/.style={tikzcirciec@capacitor={}, #1},
    battery/.style={tikzcirciec@battery={}, #1},
    bulb/.style={tikzcirciec@bulb={}, #1},
    amperemeter/.style={tikzcirciec@ammeter={}, #1},
    voltmeter/.style={tikzcirciec@voltmeter={}, #1},
    ohmmeter/.style={tikzcirciec@ohmmeter={}, #1},
  }%
}
\makeatother`

// circuitikzIecShim 检测输入是否使用 circuit ee IEC 风格, 命中时返回 IEC shim。
func circuitikzIecShim(content string) string {
	if circuitEeIECRe.MatchString(content) {
		return tikzIecShim + "\n"
	}
	return ""
}

// converterShapeRe 匹配 AC-DC / DC-AC 三相转换器 shape 名 (手册示例写法)。
var converterShapeRe = regexp.MustCompile(`\b(?:tacdcshape|tdcacshape)\b`)

// tikzConverterShim 为 bundle 的转换器 shape 补 mnemonic anchor。
// 新版 circuitikz (twoport 重构后) 的 tacdc / tdcac 使用记忆名 anchor
// (ac up in / ac mid in / dc up out 等), 老手册示例 (circuitikzmanual.tex
// "power electronics" 一节) 则用 tacdcshape / tdcacshape 形状名, 两者结合
// 的写法 (node[tacdcshape, anchor=ac mid in]) 在 bundle (1.5.x) 上会挂:
// bundle 的 tacdcshape 由 \pgfcircdeclarebipolescaled{...}{tacdc} 生成,
// 只有 legacy anchor (dc1/dc2/ac1/ac2/ac3); 记忆名 anchor 缺失时
// \pgfpointanchor 把 anchor 名当角度数学表达式解析 (\pgfqpointpolar 兜底),
// 报 "PGF Math Error: Unknown function `ac' (in 'ac mid in')"。
// pgf 的 anchor 就是 \csname pgf@anchor@<shape>@<名字>\endcsname,
// 直接 \gdef 追加即可, 无需重声明 shape; \northeast 在 anchor 调用时
// 由节点上下文提供 (与 bundle 自带 anchor 写法一致)。
// 守卫: shape 存在 (circuitikz 已加载) 且 mnemonic anchor 尚缺才注入,
// 未来 bundle 升级自带这些 anchor 时不重复定义。
// 注意 shape 注册表是 \pgf@sh@s@<名> (由 \pgfdeclareshape 定义);
// \pgf@sh@ns@<名> 是节点实例指针, 不能用作 shape 存在性检测。
const tikzConverterShim = `\makeatletter
\ifcsname pgf@sh@s@tacdcshape\endcsname
\ifcsname pgf@anchor@tacdcshape@ac mid in\endcsname\else
  \expandafter\gdef\csname pgf@anchor@tacdcshape@ac up in\endcsname{\northeast\pgf@y=.6\pgf@y\pgf@x=-\pgf@x}
  \expandafter\gdef\csname pgf@anchor@tacdcshape@ac mid in\endcsname{\northeast\pgf@y=0\pgf@y\pgf@x=-\pgf@x}
  \expandafter\gdef\csname pgf@anchor@tacdcshape@ac down in\endcsname{\northeast\pgf@y=-.6\pgf@y\pgf@x=-\pgf@x}
  \expandafter\gdef\csname pgf@anchor@tacdcshape@dc up out\endcsname{\northeast\pgf@y=.4\pgf@y}
  \expandafter\gdef\csname pgf@anchor@tacdcshape@dc down out\endcsname{\northeast\pgf@y=-.4\pgf@y}
\fi
\ifcsname pgf@anchor@tdcacshape@dc up in\endcsname\else
  \expandafter\gdef\csname pgf@anchor@tdcacshape@dc up in\endcsname{\northeast\pgf@y=.4\pgf@y\pgf@x=-\pgf@x}
  \expandafter\gdef\csname pgf@anchor@tdcacshape@dc down in\endcsname{\northeast\pgf@y=-.4\pgf@y\pgf@x=-\pgf@x}
  \expandafter\gdef\csname pgf@anchor@tdcacshape@ac up out\endcsname{\northeast\pgf@y=.6\pgf@y}
  \expandafter\gdef\csname pgf@anchor@tdcacshape@ac mid out\endcsname{\northeast\pgf@y=0\pgf@y}
  \expandafter\gdef\csname pgf@anchor@tdcacshape@ac down out\endcsname{\northeast\pgf@y=-.6\pgf@y}
\fi
\fi
\makeatother`

// circuitikzConverterShim 检测输入是否用到转换器 shape 名, 命中时返回
// mnemonic anchor 补齐 shim。注释行中的用法不触发 (% 到行尾都是注释)。
func circuitikzConverterShim(content string) string {
	for _, m := range converterShapeRe.FindAllStringIndex(content, -1) {
		lineStart := strings.LastIndex(content[:m[0]], "\n") + 1
		if strings.Contains(content[lineStart:m[0]], "%") {
			continue
		}
		return tikzConverterShim + "\n"
	}
	return ""
}

// circuitikzCompatRe 匹配显式 \usepackage[compatibility]{circuitikz(git)}。
// circuitikzgit 是本地开发包名 (bundle 里是 circuitikz), 同样按 compat 处理。
var circuitikzCompatRe = regexp.MustCompile(`\\usepackage\s*\[\s*[^\]]*compatibility[^\]]*\]\s*\{circuitikz(?:git)?\}`)

// circuitikzStarRe 匹配 circuitikz [compatibility] 的星号元件 to[*R=$R_1$]。
// 与 to[*-*] / to[o-*] 等端点样式区分: * 后必须紧跟元件名 (字母)。
var circuitikzStarRe = regexp.MustCompile(`to\s*\[\s*\*[A-Za-z]`)

// circuitikzCompatDetected 判断 circuitikz 是否应以 [compatibility] 模式加载:
// 内容中出现星号元件写法 (老式语法), 或显式写了 compatibility 选项。
func circuitikzCompatDetected(content string) bool {
	return circuitikzCompatRe.MatchString(content) || circuitikzStarRe.MatchString(content)
}

// usetikzlibraryIECRe 匹配 \usetikzlibrary{circuits.ee.IEC} 行 (含行尾换行)。
// bundle 缺该库, IEC 组件由 tikzIecShim 提供, 显式加载行直接丢弃, 否则
// tectonic 会报文件不存在。
var usetikzlibraryIECRe = regexp.MustCompile(`(?m)^[ \t]*\\usetikzlibrary\{circuits\.ee\.IEC\}[ \t]*\r?\n?`)

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
	// circuit ee IEC 是 TikZ circuits.ee.IEC 库提供的风格; bundle 缺该库,
	// tikzIecShim 把它映射到 circuitikz 元件, 所以需要加载 circuitikz。
	{regexp.MustCompile(`\bcircuit\s+ee\s+IEC\b`), "circuitikz"},
	// to[*R=$R_1$] 是 circuitikz [compatibility] 的星号元件写法 (老式语法)。
	{regexp.MustCompile(`to\s*\[\s*\*[A-Za-z]`), "circuitikz"},
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
				if name == "circuitikzgit" { // 本地开发包名, tectonic bundle 里是 circuitikz
					name = "circuitikz"
				}
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
// compatCircuitikz 为真时 circuitikz 以 [compatibility] 加载 (星号元件语法)。
func tikzPackageLines(pkgs []string, compatCircuitikz bool) string {
	if len(pkgs) == 0 {
		return ""
	}
	lines := make([]string, len(pkgs))
	for i, p := range pkgs {
		if p == "circuitikz" && compatCircuitikz {
			lines[i] = `\usepackage[compatibility]{circuitikz}`
		} else {
			lines[i] = `\usepackage{` + p + `}`
		}
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

// tkzpictureBeginRe 匹配 tkz-base 风格的 tkzpicture 环境 (可带可选参数)。
// tkzpicture 是 tkz-base (tkz-euclide 2.x 时代的依赖) 提供的 tikzpicture
// 别名; tkz-euclide 4.x 重写后不再定义该环境, 旧示例直接编译会报
// "Environment tkzpicture undefined", 因此归一化为标准 tikzpicture。
var tkzpictureBeginRe = regexp.MustCompile(`\\begin\{tkzpicture\}(\[[^]]*\])?`)

// tkzpictureEndRe 匹配 tkzpicture 环境的结束标记。
var tkzpictureEndRe = regexp.MustCompile(`\\end\{tkzpicture\}`)

// selfContainedStart 报告内容是否以自包含 TikZ 环境开头。
func selfContainedStart(content string) bool {
	for _, env := range tikzSelfContainedEnvs {
		if strings.HasPrefix(content, env) {
			return true
		}
	}
	return false
}

// stripOuterTikzShells 迭代剥离误包的 tikzpicture 空外壳。
// 用户常把完整示例再套一层 tikzpicture (如外层 tikzpicture 套 tkz-euclide
// 示例), 但 pgf 不允许直接嵌套 picture 环境——字体钩子会无限递归
// (\pgf@selectfontorig 自引用, TeX capacity exceeded, input stack size=5000),
// 必须消除嵌套。
// 仅当剥掉外壳后内部以另一个自包含环境 (tikzpicture/circuitikz/tikzcd/forest)
// 开头时才剥离: \node 内嵌 picture、\begin{scope} 等合法结构不受影响。
func stripOuterTikzShells(content string) string {
	const begin, end = `\begin{tikzpicture}`, `\end{tikzpicture}`
	for {
		trimmed := strings.TrimSpace(content)
		if !strings.HasPrefix(trimmed, begin) || !strings.HasSuffix(trimmed, end) {
			return content
		}
		inner := strings.TrimSpace(trimmed[len(begin) : len(trimmed)-len(end)])
		if !selfContainedStart(inner) {
			return content
		}
		content = inner
	}
}

// normalizeTikz 保证输入包含完整的环境:
// 已有自包含环境 (tikzpicture / circuitikz / tikzcd / forest) 则原样返回;
// 否则补一层 tikzpicture, 并把 \usetikzlibrary 提到环境外
// (它在 document body 中有效, 但在 tikzpicture 内行为不受保证)。
func normalizeTikz(content string) string {
	// tkzpicture (tkz-base 环境名, tkz-euclide 4.x 已移除) 归一化为 tikzpicture。
	content = tkzpictureBeginRe.ReplaceAllString(content, `\begin{tikzpicture}$1`)
	content = tkzpictureEndRe.ReplaceAllString(content, `\end{tikzpicture}`)
	// 剥掉误包的空外壳, 避免 pgf 嵌套 picture 崩溃。
	content = stripOuterTikzShells(content)
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
	compat := circuitikzCompatDetected(raw)
	// bundle 无 circuits.ee.IEC 库 (tikzIecShim 提供等价 IEC 组件), 丢弃显式加载行。
	raw = usetikzlibraryIECRe.ReplaceAllString(raw, "")
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
		fmt.Appendf(nil, tikzWrapper, tikzPackageLines(pkgs, compat),
			tikzLibraryLines(libs),
			circuitikzBuzzerShim(raw)+circuitikzMotorShim(raw)+circuitikzIecShim(raw)+circuitikzConverterShim(raw)+siunitxAliasShim(pkgs), font), 0644); err != nil {
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

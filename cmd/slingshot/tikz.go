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
// 第一个 %s 是额外加载的包 (由输入内容自动探测, 可为空)，
// 第二个 %s 是 CJK 字体名 (由 TIKZ_CJK_FONT 控制, 默认 Noto Sans CJK SC)。
const tikzWrapper = `\documentclass[border=2pt]{standalone}
\usepackage{tikz}
\usepackage{xcolor}
\usepackage{amsmath}
%s\usepackage{fontspec}
\usepackage{xeCJK}
\setCJKmainfont{%s}
\begin{document}
\input{input.tikz}
\end{document}
`

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
	// 提升显式 \usepackage + 自动探测内容所需的包 (tkz-euclide / tikz-cd 等)。
	raw := string(content)
	explicitPkgs, raw := extractUserPackages(raw)
	pkgs := mergeTikzPackages(explicitPkgs, detectTikzPackages(raw))
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
			font), 0644); err != nil {
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

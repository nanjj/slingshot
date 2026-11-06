package main

import (
	"fmt"
	"os/exec"
	"strings"

	"github.com/nanjj/slingshot/internal/i18n"
)

// tikzEngine 标识 tikz 子命令使用的 LaTeX 编译后端。
type tikzEngine string

// 目前只有 xelatex 一个后端。pdf/lua 实现容易但没测, 等有测试再放开。
const (
	engineXe tikzEngine = "xe" // xelatex (默认, 唯一实现)
)

// tikzProfile 声明某个后端的能力。目前只有 xelatex 一个后端。
type tikzProfile struct {
	// supportsMinted 报告该后端能否加载 minted 库 (tcblisting 的
	// listing engine=minted)。latexmk: TL2026 的 texmf.cnf 开启受限 shell
	// escape (shell_escape = p), 白名单 shell_escape_commands 里含
	// latexminted —— minted v3 把高亮交给该助手执行, 因此**不需要**
	// -shell-escape 就能工作。
	supportsMinted bool
}

// latexmkProfile 是唯一后端: TL2026 的 latexmk -xelatex, 支持 minted
// (受限 shell escape 白名单里有 latexminted, 见 tikzProfile.supportsMinted)。
func latexmkProfile() tikzProfile { return tikzProfile{supportsMinted: true} }

// latexmkEngineFlag 返回 latexmk 驱动所需的标志。目前只有 xelatex。
func latexmkEngineFlag(eng tikzEngine) string {
	return "-xelatex"
}

// latexmkCompileArgs 是 latexmk 后端的固定编译参数。
// 绝不加 -shell-escape。minted 也不需要: TL2026 的 minted v3 经 latexminted
// 助手工作, 而 latexminted 已在 texmf.cnf 的受限白名单里 (shell_escape = p
// + shell_escape_commands, 见 tikzProfile.supportsMinted)。
func latexmkCompileArgs(eng tikzEngine) []string {
	return []string{latexmkEngineFlag(eng), "-interaction=nonstopmode", "-halt-on-error", "input.tex"}
}

// lookPathFn 是 exec.LookPath 的可测试 seam。
var lookPathFn = exec.LookPath

// kpsewhichHasFn 是 kpsewhichHas 的可测试 seam。
var kpsewhichHasFn = kpsewhichHas

// kpsewhichHas 用 kpsewhich 判断文件是否存在于 TeX 搜索路径。
func kpsewhichHas(file string) bool {
	out, err := exec.Command("kpsewhich", file).Output()
	return err == nil && strings.TrimSpace(string(out)) != ""
}

// latexmkInstallHint 返回 latexmk / XeLaTeX 缺失时的安装提示。
func latexmkInstallHint(needCJK bool) string {
	hint := i18n.G("Install TeX Live: Arch: sudo pacman -S texlive-binextra texlive-bin texlive-core; Debian: sudo apt install latexmk texlive-xetex texlive-latex-base; macOS/Windows: install TeX Live or MiKTeX (MiKTeX auto-installs packages)")
	if needCJK {
		hint += " " + i18n.G("For CJK also: Arch: sudo pacman -S texlive-langcjk; Debian: sudo apt install texlive-lang-chinese")
	}
	return hint
}

// latexmkAvailable 报告 latexmk + xelatex 是否可用; needCJK 时额外要求
// xeCJK.sty 命中。绝不假设 CJK 包一定存在。
func latexmkAvailable(needCJK bool) error {
	if _, err := lookPathFn("latexmk"); err != nil {
		return fmt.Errorf("latexmk not found: %w; %s", err, latexmkInstallHint(needCJK))
	}
	if _, err := lookPathFn("xelatex"); err != nil {
		return fmt.Errorf("xelatex not found: %w; %s", err, latexmkInstallHint(needCJK))
	}
	if needCJK && !kpsewhichHasFn("xeCJK.sty") {
		return fmt.Errorf("xeCJK.sty not found (needed for CJK content); %s", latexmkInstallHint(true))
	}
	return nil
}

// selectTikzEngine 根据请求值选择渲染后端。目前只有 xe。
func selectTikzEngine(requested, content string) (tikzEngine, error) {
	switch requested {
	case "", string(engineXe):
		if err := latexmkAvailable(contentHasCJK(content)); err != nil {
			return engineXe, fmt.Errorf("latexmk unavailable: %w", err)
		}
		return engineXe, nil
	default:
		return "", fmt.Errorf(i18n.G("invalid engine %q (only xe is implemented)"), requested)
	}
}

// contentHasCJK 按 rune 判断内容是否含常见 CJK 区段(中文/日文/韩文)。
func contentHasCJK(s string) bool {
	for _, r := range s {
		switch {
		case r >= 0x2E80 && r <= 0x2EFF, // CJK 部首补充
			r >= 0x3000 && r <= 0x303F,   // CJK 符号和标点
			r >= 0x3040 && r <= 0x30FF,   // 日文平假名/片假名
			r >= 0x3100 && r <= 0x312F,   // 注音符号
			r >= 0x31A0 && r <= 0x31BF,   // 注音符号扩展
			r >= 0x31C0 && r <= 0x31EF,   // CJK 笔画
			r >= 0x3400 && r <= 0x4DBF,   // CJK 扩展 A
			r >= 0x4E00 && r <= 0x9FFF,   // CJK 统一表意文字
			r >= 0xF900 && r <= 0xFAFF,   // CJK 兼容表意文字
			r >= 0xFE30 && r <= 0xFE4F,   // CJK 兼容形式
			r >= 0xFF00 && r <= 0xFFEF,   // 全角形式
			r >= 0xAC00 && r <= 0xD7AF,   // 韩文音节
			r >= 0x1100 && r <= 0x11FF,   // 韩文谚文字母
			r >= 0x3130 && r <= 0x318F,   // 韩文兼容字母
			r >= 0x20000 && r <= 0x2FA1F: // CJK 扩展 B+
			return true
		}
	}
	return false
}

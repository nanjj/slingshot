package main

import (
	"fmt"
	"os/exec"
	"strings"

	"github.com/nanjj/slingshot/internal/i18n"
)

// tikzEngine 标识 tikz 子命令使用的 LaTeX 编译后端。
type tikzEngine string

// --engine 的取值命名的是 TeX 引擎而不是驱动: xe/pdf/lua 都由 latexmk 驱动
// (latexmk -xelatex / -pdf / -lualatex), tectonic 自带驱动与 bundle。
const (
	engineXe       tikzEngine = "xe"       // xelatex (默认)
	engineTectonic tikzEngine = "tectonic" // tectonic bundle (2021, 旧版语法)
	enginePDF      tikzEngine = "pdf"      // pdflatex — 尚未实现
	engineLua      tikzEngine = "lua"      // lualatex — 尚未实现
	engineAuto     tikzEngine = "auto"     // xe 优先, tectonic 回退
)

// tikzProfile 选择某个后端需要的兼容处理。
//
// latexmk (TeX Live 2026) 走的是新版 tkz-euclide / circuitikz 5.x 语法,
// 无需翻译; tectonic bundle 内置的是旧版 tkz-euclide 4.051b / circuitikz
// 1.4.x, 才需要把这些 5.x 语法翻译回旧版等价形式。错误地启用 legacy
// 处理会在 TL2026 上"反向出错"(见 tikz_engine 门控矩阵)。
type tikzProfile struct {
	legacyVeclen     bool // [veclen] -> [xfp]
	legacyThrough    bool // through= center..angle..point.. 重排
	legacyDefCircleR bool // \tkzDefCircle[R](A,r) 展开
	legacyNextTo     bool // \tkzInterLC[next to=..] -> near
	legacyTangentAt  bool // \tkzDefLine[tangent at=X](O) -> \tkzTgtAt
	legacyApollonius bool // 覆盖 \tkzDefApolloniusCircle
	legacyIEC        bool // tectonic: 丢弃 \usetikzlibrary{circuits.ee.IEC} + 注入 IEC shim; latexmk: 注入真库加载行, 见 iecNeedsLibrary
	legacyBuzzer     bool // buzzer shim
	legacyConverter  bool // tacdc/tdcac mnemonic anchor shim
}

// latexmkProfile 是 TL2026 主后端: 全 false, 不做 legacy 翻译。
func latexmkProfile() tikzProfile { return tikzProfile{} }

// tectonicProfile 是 tectonic 后备后端: 全 true, 启用 legacy 翻译。
func tectonicProfile() tikzProfile {
	return tikzProfile{
		legacyVeclen:     true,
		legacyThrough:    true,
		legacyDefCircleR: true,
		legacyNextTo:     true,
		legacyTangentAt:  true,
		legacyApollonius: true,
		legacyIEC:        true,
		legacyBuzzer:     true,
		legacyConverter:  true,
	}
}

// latexmkEngineFlag 返回 latexmk 驱动 eng 所需的标志。
// pdf/lua 尚未接入 (selectTikzEngine 提前报错), 但标志先就位, 将来只需放开校验。
func latexmkEngineFlag(eng tikzEngine) string {
	switch eng {
	case enginePDF:
		return "-pdf"
	case engineLua:
		return "-lualatex"
	default:
		return "-xelatex"
	}
}

// latexmkCompileArgs 是 latexmk 后端的固定编译参数。
// 绝不加 -shell-escape。
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

// tectonicAvailable 报告 tectonic 是否可用。
func tectonicAvailable() error {
	if _, err := lookPathFn("tectonic"); err != nil {
		return fmt.Errorf("tectonic not found (install via 'apt install tectonic' or from https://tectonic-typesetting.github.io): %w", err)
	}
	return nil
}

// selectTikzEngine 根据请求值与内容选择渲染后端。
//
// - xe (默认)/tectonic: 显式指定时探测失败直接报错, 不静默回退
// - auto: xe 可用则 xe; 否则 tectonic 可用则 tectonic; 都不可用报错(两个安装提示都给出)
// - pdf/lua: 保留取值但尚未实现, 明确报错而不是当成非法值
func selectTikzEngine(requested, content string) (tikzEngine, error) {
	switch requested {
	case "", string(engineXe):
		if err := latexmkAvailable(contentHasCJK(content)); err != nil {
			// 默认引擎不再静默回退, 所以这里必须指出替代方案。
			return engineXe, fmt.Errorf("latexmk unavailable: %w; %s",
				err, i18n.G("use --engine tectonic to fall back to the bundled tectonic engine"))
		}
		return engineXe, nil
	case string(engineAuto):
		needCJK := contentHasCJK(content)
		lmkErr := latexmkAvailable(needCJK)
		if lmkErr == nil {
			return engineXe, nil
		}
		tectErr := tectonicAvailable()
		if tectErr == nil {
			return engineTectonic, nil
		}
		return engineAuto, fmt.Errorf("latexmk unavailable: %v; %s; %s",
			lmkErr, latexmkInstallHint(needCJK), tectErr)
	case string(engineTectonic):
		if err := tectonicAvailable(); err != nil {
			return engineTectonic, fmt.Errorf("tectonic unavailable: %w", err)
		}
		return engineTectonic, nil
	case string(enginePDF), string(engineLua):
		return tikzEngine(requested), fmt.Errorf(i18n.G("engine %q is not implemented yet; use xe or tectonic"), requested)
	default:
		return "", fmt.Errorf(i18n.G("invalid engine %q (want xe, tectonic, pdf, lua or auto)"), requested)
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

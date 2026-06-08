// Package cmd provides shared CLI utilities for flag definitions and usage formatting.
package cmd

import (
	"fmt"
	"strings"

	"github.com/spf13/pflag"
)

// --- Flag 封装 ---
// 这些函数在 pflag 基础上增加 name|shorthand 解析和 usage 前缀处理。

// AddStringFlag 添加字符串标志, name 支持 "name|s" 格式。
func AddStringFlag(flags *pflag.FlagSet, flag *string, name string, defVal string, usage string) {
	n, s, _ := strings.Cut(name, "|")
	flags.StringVarP(flag, n, s, defVal, "``"+usage)
}

// AddStringArrayFlag 添加字符串数组标志。
func AddStringArrayFlag(flags *pflag.FlagSet, flag *[]string, name string, usage string) {
	n, s, _ := strings.Cut(name, "|")
	flags.StringArrayVarP(flag, n, s, nil, "``"+usage)
}

// AddIntFlag 添加整数标志。
func AddIntFlag(flags *pflag.FlagSet, flag *int, name string, defVal int, usage string) {
	n, s, _ := strings.Cut(name, "|")
	flags.IntVarP(flag, n, s, defVal, "``"+usage)
}

// AddBoolFlag 添加布尔标志。
func AddBoolFlag(flags *pflag.FlagSet, flag *bool, name string, usage string) {
	n, s, _ := strings.Cut(name, "|")
	flags.BoolVarP(flag, n, s, false, "``"+usage)
}

// --- Usage 格式化 ---

// Usage 格式化命令名和子命令名 (用于 cobra 的 Use 字段)。
func Usage(name string, args ...string) string {
	if len(args) == 0 {
		return name
	}
	return name + " " + args[0]
}

// U 格式化命令名和 Atom 参数 (用于 cobra 的 Use 字段, 自动 Render)。
// args 是 usage.Atom 的 Render() 结果字符串。
func U(name string, args ...string) string {
	if len(args) == 0 {
		return name
	}
	return name + " " + strings.Join(args, " ")
}

// FormatSection 格式化帮助文本的 section (带缩进)。
func FormatSection(header string, content string) string {
	var out strings.Builder

	if header != "" {
		out.WriteString(header)
		out.WriteByte('\n')
	}

	for line := range strings.SplitSeq(content, "\n") {
		if line != "" {
			out.WriteString("  ")
		}
		out.WriteString(line)
		out.WriteByte('\n')
	}

	if header != "" {
		out.WriteByte('\n')
	} else {
		s := out.String()
		s = strings.TrimSuffix(s, "\n")
		return s
	}

	return out.String()
}

// ColorizePrefix 创建带颜色的前缀字符串 (如 "Description:" )。
// colorCode: ANSI 颜色代码, text: 前缀文本。
func ColorizePrefix(colorCode string, text string) string {
	return fmt.Sprintf("\033[%sm%s\033[0m", colorCode, text)
}

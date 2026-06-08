package usage

import (
	"strings"
)

// Usage 是参数语法定义: 一组 Atom 的序列。
type Usage []Atom

// Atom 是参数语法的最小单位。
// 每个 Atom 同时定义 解析逻辑(Parse) 和 帮助文本(Render)。
type Atom interface {
	// Parse 从 args 头部消费参数并返回解析结果。
	// 成功时修改 *args, 失败时返回 error (不修改 *args)。
	Parse(args *[]string) (*Parsed, error)

	// Render 生成帮助文本片段 (用于 --help 输出)。
	Render() string

	// List 将 atom 包装为可重复列表。
	// minOccurrences: 最少出现次数
	// separator: 分隔符 (默认 " ")
	List(minOccurrences int, separator ...string) Atom

	// Optional 将 atom 包装为可选。
	Optional() Atom
}

// --- 预定义 atom 变量 ---

var (
	File    = placeholder{"file"}
	ID      = placeholder{"id"}
	Key     = placeholder{"key"}
	Value   = placeholder{"value"}
	Name    = placeholder{"name"}
	KV      = compound{"=", []Atom{Key, Value}}
)

// --- 构造函数 ---

func Verbatim(element string) verbatim {
	return verbatim{element}
}

func Placeholder(element string) placeholder {
	return placeholder{element}
}

func Either(atoms ...Atom) Atom {
	return alternative{atoms}
}

func EitherVerbatim(elements ...string) Atom {
	atoms := make([]Atom, len(elements))
	for i, e := range elements {
		atoms[i] = verbatim{e}
	}
	return alternative{atoms}
}

func Sequence(atoms ...Atom) Atom {
	return compound{" ", atoms}
}

func MakePath(atoms ...Atom) Atom {
	return compound{"/", atoms}
}

func Colon(a Atom) Atom {
	return compound{":", []Atom{a, verbatim{""}}}
}

func Flag(name string) Atom {
	return flag{name}
}


// helper: 渲染不带颜色的原始文本
func renderRaw(a Atom) string {
	switch v := a.(type) {
	case verbatim:
		return v.element
	case placeholder:
		return "<" + v.element + ">"
	case alternative:
		els := make([]string, len(v.atoms))
		for i, a := range v.atoms {
			els[i] = renderRaw(a)
		}
		return "(" + strings.Join(els, "|") + ")"
	case compound:
		els := make([]string, len(v.atoms))
		for i, a := range v.atoms {
			els[i] = renderRaw(a)
		}
		return strings.Join(els, v.separator)
	case optional:
		return "[" + renderRaw(v.atom) + "]"
	case list:
		s := renderRaw(v.atom)
		if v.separator == " " {
			return s + "..."
		}
		return s + v.separator + s + "..."
	case flag:
		return "--" + v.name
	case hide:
		return renderRaw(v.replacement)
	case deprecated:
		return renderRaw(v.atom)
	default:
		return a.Render()
	}
}

func quote(s string) string {
	return "'" + s + "'"
}

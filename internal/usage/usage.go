// Package usage implements a declarative command-line argument parser.
//
// 核心设计: 用 Atom 树声明语法, 自动推导解析逻辑和帮助文本。
// 这是从 incus CLI 提炼的精简实现, 移除了 remote server、color 等
// incus 特定依赖, 保留了完整的 Atom 组合体系。
//
// Atom 接口的四组操作:
//   - Parse: 从 args 头部消费参数
//   - Render: 生成帮助文本片段
//   - List: 包装为可重复的列表
//   - Optional: 包装为可选的
package usage

import (
	"fmt"
	"os"
	"strings"

	"github.com/fatih/color"

	"github.com/nanjj/slingshot/internal/i18n"
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

// --- 辅助函数 ---

func makeList(atom Atom, min int, sep ...string) Atom {
	if len(sep) == 0 {
		return list{atom, min, " "}
	}
	return list{atom, min, sep[0]}
}

// --- verbatim: 字面量匹配 ---

type verbatim struct{ element string }

func (v verbatim) Parse(args *[]string) (*Parsed, error) {
	if len(*args) == 0 {
		return nil, &notEnoughArgumentsError{v}
	}
	arg := (*args)[0]
	if arg != v.element {
		return nil, &argumentMismatchError{arg, []string{v.element}}
	}
	*args = (*args)[1:]
	return &Parsed{source: v, String: arg}, nil
}

func (v verbatim) Render() string { return v.element }
func (v verbatim) List(min int, sep ...string) Atom  { return makeList(v, min, sep...) }
func (v verbatim) Optional() Atom                     { return optional{v} }

// --- placeholder: <name> 位置参数 ---

type placeholder struct{ element string }

func (p placeholder) Parse(args *[]string) (*Parsed, error) {
	if len(*args) == 0 {
		return nil, &notEnoughArgumentsError{p}
	}
	arg := (*args)[0]
	*args = (*args)[1:]
	return &Parsed{source: p, String: arg}, nil
}

func (p placeholder) Render() string { return color.GreenString("<" + p.element + ">") }
func (p placeholder) List(min int, sep ...string) Atom  { return makeList(p, min, sep...) }
func (p placeholder) Optional() Atom                     { return optional{p} }

// --- alternative: A|B 选择 ---

type alternative struct{ atoms []Atom }

func (a alternative) Parse(args *[]string) (*Parsed, error) {
	verbatimOnly := true
	verbatimElements := make([]string, len(a.atoms))

	for i, atom := range a.atoms {
		if v, ok := atom.(verbatim); ok {
			verbatimElements[i] = v.Render()
		} else {
			verbatimOnly = false
		}

		argsCopy := make([]string, len(*args))
		copy(argsCopy, *args)
		p, err := atom.Parse(&argsCopy)
		if err != nil {
			if isParsingError(err) {
				continue
			}
			return nil, err
		}
		*args = argsCopy
		p.BranchID = i
		return p, nil
	}

	if len(*args) == 0 {
		return nil, &notEnoughArgumentsError{a}
	}

	arg := (*args)[0]
	if verbatimOnly {
		return nil, &argumentMismatchError{arg, verbatimElements}
	}
	return nil, &argumentMismatchError{arg, []string{}}
}

func (a alternative) Render() string {
	elements := make([]string, len(a.atoms))
	for i, atom := range a.atoms {
		elements[i] = atom.Render()
	}
	faint := color.New(color.Faint)
	return faint.Sprint("(") + strings.Join(elements, faint.Sprint("|")) + faint.Sprint(")")
}

func (a alternative) List(min int, sep ...string) Atom  { return makeList(a, min, sep...) }
func (a alternative) Optional() Atom                     { return optional{a} }

// --- compound: 带分隔符的序列 ---

type compound struct {
	separator string
	atoms     []Atom
}

func (c compound) Parse(args *[]string) (*Parsed, error) {
	var consumed []string
	n := len(c.atoms)
	ps := make([]*Parsed, n)
	atoms := c.atoms

	// 数末尾的 optional 数量
	nOpt := 0
	for i := range n {
		if _, ok := atoms[n-1-i].(optional); !ok {
			break
		}
		nOpt++
	}

	if c.separator == " " {
		nSub := len(*args)

		// 跳过因参数不足而无法匹配的 optional
		i := 0
		for i < n-nOpt-nSub {
			o, ok := atoms[i].(optional)
			if !ok {
				break
			}
			p, err := o.Parse(&[]string{})
			if err != nil {
				return nil, err
			}
			ps[i] = p
			i++
		}

		for i < n {
			p, err := atoms[i].Parse(args)
			if err != nil {
				return nil, err
			}
			ps[i] = p
			if !p.Skipped {
				consumed = append(consumed, p.String)
			}
			i++
		}
	} else {
		if len(*args) == 0 {
			return nil, &notEnoughArgumentsError{c}
		}

		arg := (*args)[0]
		consumed = []string{arg}
		subArgs := strings.Split(arg, c.separator)
		nSub := len(subArgs)
		i := 0

		// 跳过因参数不足而无法匹配的 optional
		for i < n-nOpt-nSub {
			o, ok := atoms[i].(optional)
			if !ok {
				break
			}
			p, err := o.Parse(&[]string{})
			if err != nil {
				return nil, err
			}
			ps[i] = p
			i++
		}

		// 解析到倒数第二个 atom
		for i < n-1 {
			p, err := atoms[i].Parse(&subArgs)
			if err != nil {
				return nil, err
			}
			ps[i] = p
			i++
		}

		// 最后一个 atom 消费所有剩余
		lastArgs := []string{}
		if len(subArgs) > 0 {
			lastArgs = append(lastArgs, strings.Join(subArgs, c.separator))
		}
		p, err := atoms[n-1].Parse(&lastArgs)
		if err != nil {
			return nil, err
		}
		ps[n-1] = p
		*args = (*args)[1:]
	}

	stringList := make([]string, len(ps))
	for i, p := range ps {
		stringList[i] = p.String
	}

	return &Parsed{source: c, String: strings.Join(consumed, " "), List: ps, StringList: stringList}, nil
}

func (c compound) Render() string {
	if len(c.atoms) == 1 {
		return c.atoms[0].Render()
	}

	var sb strings.Builder
	firstNonOptionalAtom := 0
	for i, atom := range c.atoms {
		o, ok := atom.(optional)
		if ok && c.separator != " " {
			if i == firstNonOptionalAtom {
				sb.WriteString(optional{verbatim{o.atom.Render() + c.separator}}.Render())
				firstNonOptionalAtom++
			} else {
				sb.WriteString(optional{verbatim{c.separator + o.atom.Render()}}.Render())
			}
		} else {
			if i == firstNonOptionalAtom {
				sb.WriteString(atom.Render())
			} else {
				sb.WriteString(c.separator + atom.Render())
			}
		}
	}
	return sb.String()
}

func (c compound) List(min int, sep ...string) Atom  { return makeList(c, min, sep...) }
func (c compound) Optional() Atom                     { return optional{c} }

// --- optional: 可选 [...] ---

type optional struct{ atom Atom }

func (o optional) Parse(args *[]string) (*Parsed, error) {
	argsCopy := make([]string, len(*args))
	copy(argsCopy, *args)
	p, err := o.atom.Parse(&argsCopy)
	if err != nil {
		if isParsingError(err) {
			return &Parsed{source: o, err: err, Skipped: true}, nil
		}
		return nil, err
	}
	*args = argsCopy
	return p, nil
}

func (o optional) Render() string {
	faint := color.New(color.Faint)
	return faint.Sprint("[") + o.atom.Render() + faint.Sprint("]")
}

func (o optional) List(min int, sep ...string) Atom {
	return makeList(o.atom, min, sep...).Optional()
}

func (o optional) Optional() Atom {
	return optional{o.atom}
}

// --- list: 重复列表 ---

type list struct {
	atom           Atom
	minOccurrences int
	separator      string
}

func (l list) Parse(args *[]string) (*Parsed, error) {
	var ps []*Parsed
	var consumed []string

	if l.separator == " " {
		for range l.minOccurrences {
			p, err := l.atom.Parse(args)
			if err != nil {
				return nil, err
			}
			ps = append(ps, p)
			consumed = append(consumed, p.String)
		}

		for {
			argsCopy := make([]string, len(*args))
			copy(argsCopy, *args)
			p, err := l.atom.Parse(&argsCopy)
			if err != nil {
				if isParsingError(err) {
					break
				}
				return nil, err
			}
			*args = argsCopy
			ps = append(ps, p)
			consumed = append(consumed, p.String)
		}
	} else {
		var subArgs []string
		if len(*args) == 0 {
			subArgs = []string{}
			consumed = []string{}
		} else {
			arg := (*args)[0]
			subArgs = strings.Split(arg, l.separator)
			consumed = []string{arg}
		}

		i := 0
		for i < l.minOccurrences || len(subArgs) > 0 {
			p, err := l.atom.Parse(&subArgs)
			if err != nil {
				if l.minOccurrences == 0 && isParsingError(err) {
					return &Parsed{source: l, err: err, Skipped: true}, nil
				}
				return nil, err
			}
			ps = append(ps, p)
			i++
		}

		if len(*args) > 0 {
			*args = (*args)[1:]
		}
	}

	stringList := make([]string, len(ps))
	for i, p := range ps {
		stringList[i] = p.String
	}

	skipped := len(ps) == 0
	return &Parsed{source: l, String: strings.Join(consumed, " "), List: ps, StringList: stringList, Skipped: skipped}, nil
}

func (l list) Render() string {
	faint := color.New(color.Faint)
	switch l.minOccurrences {
	case 0:
		return optional{list{l.atom, 1, l.separator}}.Render()
	case 1:
		element := l.atom.Render()
		if l.separator == " " {
			return element + faint.Sprint("...")
		}
		return element + optional{verbatim{l.separator + element + faint.Sprint("...")}}.Render()
	default:
		return l.atom.Render() + l.separator + list{l.atom, l.minOccurrences - 1, l.separator}.Render()
	}
}

func (l list) List(min int, sep ...string) Atom {
	return makeList(l, min, sep...)
}

func (l list) Optional() Atom {
	return optional{list{l.atom, max(l.minOccurrences, 1), l.separator}}
}

// --- flag: 引用标志 ---

type flag struct{ name string }

func (f flag) Parse(args *[]string) (*Parsed, error) {
	for _, arg := range *args {
		if arg == "--"+f.name || strings.HasPrefix(arg, "--"+f.name+"=") {
			return &Parsed{source: f, String: "--" + f.name}, nil
		}
	}
	return nil, &argumentMismatchError{"", []string{"--" + f.name}}
}

func (f flag) Render() string { return verbatim{"--" + f.name}.Render() }
func (f flag) List(min int, sep ...string) Atom  { return makeList(f, min, sep...) }
func (f flag) Optional() Atom                     { return optional{f} }

// --- remote: remote: 前缀 ---

type remote struct {
	atom     Atom
	suffix   Atom
	optional bool
}

func (r remote) Parse(args *[]string) (*Parsed, error) {
	if len(*args) == 0 {
		if r.optional {
			return &Parsed{source: r, Skipped: true, String: ""}, nil
		}
		return nil, &notEnoughArgumentsError{r}
	}

	arg := (*args)[0]

	if !strings.Contains(arg, ":") {
		if r.optional {
			return &Parsed{source: r, Skipped: true, String: ""}, nil
		}
		return nil, &argumentMismatchError{arg, []string{"<remote>:"}}
	}

	parts := strings.SplitN(arg, ":", 2)
	remoteName := parts[0]
	rest := parts[1]

	if r.suffix != nil {
		restArgs := []string{}
		if rest != "" {
			restArgs = append(restArgs, rest)
		}
		p, err := r.suffix.Parse(&restArgs)
		if err != nil {
			return nil, err
		}
		*args = (*args)[1:]
		return &Parsed{source: r, String: arg, RemoteName: remoteName, RemoteObject: p}, nil
	}

	if rest != "" {
		return nil, &argumentNotFullyConsumedError{rest, arg}
	}

	*args = (*args)[1:]
	return &Parsed{source: r, String: arg, RemoteName: remoteName, Skipped: false}, nil
}

func (r remote) Render() string {
	suffix := r.suffix
	if suffix == nil {
		suffix = verbatim{}
	}

	var prefix Atom
	prefix = compound{":", []Atom{r.atom, verbatim{""}}}
	if r.optional {
		prefix = prefix.Optional()
	}

	return prefix.Render() + suffix.Render()
}

func (r remote) List(min int, sep ...string) Atom  { return makeList(r, min, sep...) }
func (r remote) Optional() Atom                     { return optional{r} }

// --- hide: 内部值隐藏, 显示替代文本 ---

type hide struct {
	atom        Atom
	replacement Atom
}

func (h hide) Parse(args *[]string) (*Parsed, error) { return h.atom.Parse(args) }
func (h hide) Render() string                         { return h.replacement.Render() }
func (h hide) List(min int, sep ...string) Atom       { return makeList(h, min, sep...) }
func (h hide) Optional() Atom                          { return optional{h} }

// --- deprecated: 废弃语法 ---

type deprecated struct {
	atom    Atom
	warning string
}

func (d deprecated) Parse(args *[]string) (*Parsed, error) {
	parsed, err := d.atom.Parse(args)
	if err != nil {
		return nil, err
	}
	syntax := renderRaw(d.atom)
	fmt.Fprintf(os.Stderr, "%s the %s syntax is deprecated; %s\n",
		color.YellowString(i18n.G("Warning:")), quote(syntax), d.warning)
	return parsed, nil
}

func (d deprecated) Render() string                        { return d.atom.Render() }
func (d deprecated) List(min int, sep ...string) Atom      { return makeList(d, min, sep...) }
func (d deprecated) Optional() Atom                         { return optional{d} }

// --- 预定义 atom 变量 ---

var (
	File     = placeholder{"file"}
	ID       = placeholder{"id"}
	Remote   = placeholder{"remote"}
	Key      = placeholder{"key"}
	Value    = placeholder{"value"}
	Name     = placeholder{"name"}
	KV       = compound{"=", []Atom{Key, Value}}
	RemoteColon    = remote{Remote, nil, false}
	RemoteColonOpt = remote{Remote, nil, true}
)

// --- 构造函数 ---

func Verbatim(element string) verbatim { return verbatim{element} }
func Placeholder(element string) placeholder { return placeholder{element} }
func Either(atoms ...Atom) Atom { return alternative{atoms} }
func EitherVerbatim(elements ...string) Atom {
	atoms := make([]Atom, len(elements))
	for i, e := range elements {
		atoms[i] = verbatim{e}
	}
	return alternative{atoms}
}
func Sequence(atoms ...Atom) Atom { return compound{" ", atoms} }
func MakePath(atoms ...Atom) Atom { return compound{"/", atoms} }
func Colon(a Atom) Atom { return compound{":", []Atom{a, verbatim{""}}} }
func Flag(name string) Atom { return flag{name} }
func MakeRemote(atom Atom, optional bool) remote { return remote{atom, nil, optional} }

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
	case remote:
		s := renderRaw(v.atom) + ":"
		if v.optional {
			s = "[" + s + "]"
		}
		if v.suffix != nil {
			s += renderRaw(v.suffix)
		}
		return s
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

// --- Parser ---

// Config 是解析器配置 (预留)。
type Config struct{}

// Parsed 是解析结果。
type Parsed struct {
	source       Atom
	RemoteName   string
	RemoteObject *Parsed
	String       string
	List         []*Parsed
	StringList   []string
	err          error
	Skipped      bool
	BranchID     int
}

// Get 获取解析结果的字符串值, 如果被跳过则返回默认值。
func (p Parsed) Get(def string) string {
	if p.Skipped {
		return def
	}
	return p.String
}

// Parse 解析整个 Usage。
func (u Usage) Parse(args []string) ([]*Parsed, error) {
	var result []*Parsed
	argsInUse := args

	for _, atom := range u {
		p, err := atom.Parse(&argsInUse)
		if err != nil {
			u.diagnose(result)
			return nil, err
		}
		result = append(result, p)
	}

	if len(argsInUse) != 0 {
		err := &tooManyArgumentsError{argsInUse}
		u.diagnose(result)
		return nil, err
	}

	return result, nil
}

// --- 错误类型 ---

type notEnoughArgumentsError struct{ atom Atom }

func (e *notEnoughArgumentsError) Error() string {
	if v, ok := e.atom.(verbatim); ok && v.element == "" {
		return i18n.G("unexpected end of argument; did you forget a suffix?")
	}
	return fmt.Sprintf(i18n.G("not enough arguments; expected a value for %s"), quote(renderRaw(e.atom)))
}

type tooManyArgumentsError struct{ args []string }

func (e *tooManyArgumentsError) Error() string {
	return fmt.Sprintf(i18n.G("too many arguments; unexpected %s"), quote(strings.Join(e.args, " ")))
}

type argumentMismatchError struct {
	arg      string
	expected []string
}

func (e *argumentMismatchError) Error() string {
	n := len(e.expected)
	if n == 0 {
		return fmt.Sprintf(i18n.G("unexpected %s"), quote(e.arg))
	}
	if e.arg == "" {
		return fmt.Sprintf(i18n.G("expected %s"), formatAlternatives(e.expected))
	}
	return fmt.Sprintf(i18n.G("unexpected %s; expected %s"), quote(e.arg), formatAlternatives(e.expected))
}

type argumentNotFullyConsumedError struct {
	rest   string
	parent string
}

func (e *argumentNotFullyConsumedError) Error() string {
	if e.rest == e.parent {
		return fmt.Sprintf(i18n.G("cannot parse this argument; unexpected %s"), quote(e.rest))
	}
	return fmt.Sprintf(i18n.G("cannot parse this argument; unexpected %s in %s"), quote(e.rest), quote(e.parent))
}

func isParsingError(err error) bool {
	switch err.(type) {
	case *notEnoughArgumentsError, *tooManyArgumentsError, *argumentMismatchError, *argumentNotFullyConsumedError:
		return true
	default:
		return false
	}
}

func formatAlternatives(alternatives []string) string {
	n := len(alternatives)
	if n == 1 {
		return fmt.Sprintf(i18n.G("one of %s"), quote(alternatives[0]))
	}
	quoted := make([]string, n)
	for i, exp := range alternatives {
		quoted[i] = quote(exp)
	}
	return fmt.Sprintf(i18n.G("one of %s or %s"), strings.Join(quoted[:n-1], i18n.G(", ")), quoted[n-1])
}

// diagnose 输出参数解析的可视化诊断。
func (u Usage) diagnose(parsed []*Parsed) {
	if len(parsed) == 0 {
		return
	}

	rendered := make([]string, len(u))
	for i, atom := range u {
		rendered[i] = atom.Render()
	}

	fmt.Fprintf(os.Stderr, i18n.G("Usage: %s\n"), strings.Join(rendered, " "))
	for i, p := range parsed {
		prefix := "  "
		for range i {
			prefix += "  "
		}
		if p.Skipped {
			fmt.Fprintf(os.Stderr, "%s└─ %s\n", prefix, i18n.G("(skipped: no value given)"))
		} else if p.err != nil {
			fmt.Fprintf(os.Stderr, "%s└─ %s: %s\n", prefix, i18n.G("(skipped)"), p.err)
		} else {
			fmt.Fprintf(os.Stderr, "%s└─ %s\n", prefix, quote(p.String))
		}
	}
}

// ParseString 将字符串包装为 Parsed (用于测试或手动构造)。
func ParseString(s string) *Parsed {
	return &Parsed{String: s}
}

// ParseDefault 解析单个 atom。
func ParseDefault(atom Atom, args ...string) (*Parsed, error) {
	argsCopy := make([]string, len(args))
	copy(argsCopy, args)
	return atom.Parse(&argsCopy)
}

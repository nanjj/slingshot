package usage

import (
	"strings"

	"github.com/fatih/color"
)

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

func (l list) List(minOccurrences int, sep ...string) Atom {
	return makeList(l, minOccurrences, sep...)
}

func (l list) Optional() Atom {
	return optional{list{l.atom, max(l.minOccurrences, 1), l.separator}}
}

// --- 辅助函数 ---

func makeList(atom Atom, minOccurrences int, sep ...string) Atom {
	switch len(sep) {
	case 0:
		return list{atom, minOccurrences, " "}
	case 1:
		return list{atom, minOccurrences, sep[0]}
	default:
		panic("usage: List accepts at most one separator")
	}
}

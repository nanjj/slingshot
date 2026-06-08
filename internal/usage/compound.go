package usage

import "strings"

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
				sb.WriteString(c.separator)
				sb.WriteString(atom.Render())
			}
		}
	}
	return sb.String()
}

func (c compound) List(minOccurrences int, sep ...string) Atom {
	return makeList(c, minOccurrences, sep...)
}

func (c compound) Optional() Atom {
	return optional{c}
}

package usage

import "github.com/fatih/color"

// --- placeholder: <name> 位置参数 ---

type placeholder struct {
	element string
}

func (p placeholder) Parse(args *[]string) (*Parsed, error) {
	if len(*args) == 0 {
		return nil, &notEnoughArgumentsError{p}
	}
	arg := (*args)[0]
	*args = (*args)[1:]
	return &Parsed{source: p, String: arg}, nil
}

func (p placeholder) Render() string {
	return color.GreenString("<" + p.element + ">")
}

func (p placeholder) List(minOccurrences int, sep ...string) Atom {
	return makeList(p, minOccurrences, sep...)
}

func (p placeholder) Optional() Atom {
	return optional{p}
}

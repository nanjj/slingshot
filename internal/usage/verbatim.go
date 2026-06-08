package usage

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

func (v verbatim) Render() string {
	return v.element
}

func (v verbatim) List(minOccurrences int, sep ...string) Atom {
	return makeList(v, minOccurrences, sep...)
}

func (v verbatim) Optional() Atom {
	return optional{v}
}

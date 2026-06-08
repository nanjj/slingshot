package usage

// --- hide: 内部值隐藏, 显示替代文本 ---

type hide struct {
	atom        Atom
	replacement Atom
}

func (h hide) Parse(args *[]string) (*Parsed, error) {
	return h.atom.Parse(args)
}

func (h hide) Render() string {
	return h.replacement.Render()
}

func (h hide) List(min int, sep ...string) Atom {
	return makeList(h, min, sep...)
}

func (h hide) Optional() Atom {
	return optional{h}
}

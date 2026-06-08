package usage

import "strings"

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

func (f flag) Render() string {
	return verbatim{"--" + f.name}.Render()
}

func (f flag) List(min int, sep ...string) Atom {
	return makeList(f, min, sep...)
}

func (f flag) Optional() Atom {
	return optional{f}
}


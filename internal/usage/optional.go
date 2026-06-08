package usage

import "github.com/fatih/color"

// --- optional: 可选 [...] ---

type optional struct {
	atom Atom
}

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

func (o optional) List(minOccurrences int, sep ...string) Atom {
	return makeList(o.atom, minOccurrences, sep...).Optional()
}

func (o optional) Optional() Atom {
	return optional{o.atom}
}

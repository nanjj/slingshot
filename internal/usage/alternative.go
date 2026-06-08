package usage

import (
	"strings"

	"github.com/fatih/color"
)

// --- alternative: A|B 选择 ---

type alternative struct {
	atoms []Atom
}

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
	return faint.Sprint("(") +
		strings.Join(elements, faint.Sprint("|")) +
		faint.Sprint(")")
}

func (a alternative) List(minOccurrences int, sep ...string) Atom {
	return makeList(a, minOccurrences, sep...)
}

func (a alternative) Optional() Atom {
	return optional{a}
}

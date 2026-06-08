package usage

import "strings"

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

func (r remote) List(min int, sep ...string) Atom {
	return makeList(r, min, sep...)
}

func (r remote) Optional() Atom {
	return optional{r}
}

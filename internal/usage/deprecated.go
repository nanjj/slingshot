package usage

import (
	"fmt"
	"os"

	"github.com/fatih/color"
	"github.com/nanjj/slingshot/internal/i18n"
)

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

func (d deprecated) Render() string {
	return d.atom.Render()
}

func (d deprecated) List(min int, sep ...string) Atom {
	return makeList(d, min, sep...)
}

func (d deprecated) Optional() Atom {
	return optional{d}
}

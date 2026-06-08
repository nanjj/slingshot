package usage

import (
	"fmt"
	"io"
	"strings"

	"github.com/nanjj/slingshot/internal/i18n"
)

// diagnose 输出参数解析的可视化诊断。
// 当 --explain 时输出到 stdout, 错误时输出到 stderr。
func (u Usage) diagnose(w io.Writer, parsed []*Parsed) {
	if len(parsed) == 0 {
		return
	}

	rendered := make([]string, len(u))
	for i, atom := range u {
		rendered[i] = atom.Render()
	}

	fmt.Fprintf(w, "%s %s\n", i18n.G("Usage:"), strings.Join(rendered, " "))
	for i, p := range parsed {
		var prefix strings.Builder
		prefix.WriteString("  ")
		for range i {
			prefix.WriteString("  ")
		}
		if p.Skipped {
			fmt.Fprintf(w, "%s└─ %s\n", prefix.String(), i18n.G("(skipped: no value given)"))
		} else if p.err != nil {
			fmt.Fprintf(w, "%s└─ %s: %s\n", prefix.String(), i18n.G("(skipped)"), p.err)
		} else {
			fmt.Fprintf(w, "%s└─ '%s'\n", prefix.String(), p.String)
		}
	}
}

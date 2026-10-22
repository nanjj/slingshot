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

	// 每个 atom 是同级关系, 不是嵌套。用序号区分, 不递增缩进,
	// 否则第二个参数看起来像第一个的子项。
	for i, p := range parsed {
		label := fmt.Sprintf("  [%d] └─ ", i+1)
		if p.Skipped {
			fmt.Fprintf(w, "%s%s\n", label, i18n.G("(skipped: no value given)"))
		} else if p.err != nil {
			fmt.Fprintf(w, "%s%s: %s\n", label, i18n.G("(skipped)"), p.err)
		} else {
			fmt.Fprintf(w, "%s'%s'\n", label, p.String)
		}
	}
}

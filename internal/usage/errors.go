package usage

import (
	"fmt"
	"strings"

	"github.com/nanjj/slingshot/internal/i18n"
)

// ErrExplainOnly 是一个哨兵错误, 表示 --explain 模式解析成功但无需执行。
var ErrExplainOnly = fmt.Errorf("explain only")

// --- 错误类型 ---

type notEnoughArgumentsError struct {
	atom Atom
}

func (e *notEnoughArgumentsError) Error() string {
	if v, ok := e.atom.(verbatim); ok && v.element == "" {
		return i18n.G("unexpected end of argument; did you forget a suffix?")
	}
	return fmt.Sprintf(i18n.G("not enough arguments; expected a value for %s"), quote(renderRaw(e.atom)))
}

type tooManyArgumentsError struct{ args []string }

func (e *tooManyArgumentsError) Error() string {
	return fmt.Sprintf(i18n.G("too many arguments; unexpected %s"), quote(strings.Join(e.args, " ")))
}

type argumentMismatchError struct {
	arg      string
	expected []string
}

func (e *argumentMismatchError) Error() string {
	n := len(e.expected)
	if n == 0 {
		return fmt.Sprintf(i18n.G("unexpected %s"), quote(e.arg))
	}
	if e.arg == "" {
		return fmt.Sprintf(i18n.G("expected %s"), formatAlternatives(e.expected))
	}
	return fmt.Sprintf(i18n.G("unexpected %s; expected %s"), quote(e.arg), formatAlternatives(e.expected))
}

// isParsingError 判断 err 是否为解析错误。
func isParsingError(err error) bool {
	switch err.(type) {
	case *notEnoughArgumentsError, *tooManyArgumentsError, *argumentMismatchError:
		return true
	default:
		return false
	}
}

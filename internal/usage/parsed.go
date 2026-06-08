package usage

import (
	"fmt"
	"os"
	"strings"

	"github.com/nanjj/slingshot/internal/i18n"
)

// --- Parser ---

// Config 是解析器配置。
type Config struct {
	// ExplainOnly 为 true 时, 解析成功后调用 diagnose 并返回 ErrExplainOnly。
	ExplainOnly bool
}

func formatAlternatives(alternatives []string) string {
	n := len(alternatives)
	if n == 1 {
		return fmt.Sprintf(i18n.G("one of %s"), quote(alternatives[0]))
	}
	quoted := make([]string, n)
	for i, exp := range alternatives {
		quoted[i] = quote(exp)
	}
	return fmt.Sprintf(i18n.G("one of %s or %s"), strings.Join(quoted[:n-1], i18n.G(", ")), quoted[n-1])
}

// Parsed 是解析结果。
type Parsed struct {
	source     Atom
	String     string
	List       []*Parsed
	StringList []string
	err        error
	Skipped    bool
	BranchID   int
}

// Get 获取解析结果的字符串值, 如果被跳过则返回默认值。
func (p Parsed) Get(def string) string {
	if p.Skipped {
		return def
	}
	return p.String
}

// Parse 解析整个 Usage。
// 如果 conf.ExplainOnly 为 true, 解析成功后调用 diagnose 并返回 ErrExplainOnly。
func (u Usage) Parse(args []string, conf ...Config) ([]*Parsed, error) {
	var cfg Config
	if len(conf) > 0 {
		cfg = conf[0]
	}

	var result []*Parsed
	argsInUse := args

	for _, atom := range u {
		p, err := atom.Parse(&argsInUse)
		if err != nil {
			u.diagnose(os.Stderr, result)
			return nil, err
		}
		result = append(result, p)
	}

	if len(argsInUse) != 0 {
		err := &tooManyArgumentsError{argsInUse}
		u.diagnose(os.Stderr, result)
		return nil, err
	}

	if cfg.ExplainOnly {
		u.diagnose(os.Stdout, result)
		return nil, ErrExplainOnly
	}

	return result, nil
}

// ParseString 将字符串包装为 Parsed (用于测试或手动构造)。
func ParseString(s string) *Parsed {
	return &Parsed{String: s}
}

// ParseDefault 解析单个 atom。
func ParseDefault(atom Atom, args ...string) (*Parsed, error) {
	argsCopy := make([]string, len(args))
	copy(argsCopy, args)
	return atom.Parse(&argsCopy)
}

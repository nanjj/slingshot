package usage

import (
	"bytes"
	"strings"
	"testing"

	"github.com/nanjj/slingshot/internal/i18n"
)

// runInLocales runs fn with each listed locale, restoring the original locale after.
func runInLocales(t *testing.T, locales []string, fn func(t *testing.T)) {
	t.Helper()
	orig := i18n.CurrentLocale()
	for _, lang := range locales {
		t.Run(lang, func(t *testing.T) {
			i18n.SetLocale(lang)
			defer i18n.SetLocale(orig)
			fn(t)
		})
	}
}

func TestDiagnose(t *testing.T) {
	runInLocales(t, []string{"en_US", "zh_CN"}, func(t *testing.T) {
		u := Usage{Verbatim("add"), Placeholder("name")}
		parsed, err := u.Parse([]string{"add", "hello"})
		if err != nil {
			t.Fatal(err)
		}

		var buf bytes.Buffer
		u.diagnose(&buf, parsed)
		output := buf.String()

		if !strings.Contains(output, "hello") {
			t.Errorf("diagnose output missing 'hello': %s", output)
		}

		// Check that output contains the translated "Usage:" prefix
		usageLabel := i18n.G("Usage:")
		if !strings.Contains(output, usageLabel) {
			t.Errorf("diagnose output missing %q: %s", usageLabel, output)
		}
	})
}

func TestDiagnoseEmpty(t *testing.T) {
	runInLocales(t, []string{"en_US", "zh_CN"}, func(t *testing.T) {
		u := Usage{Verbatim("x")}
		u.diagnose(&bytes.Buffer{}, nil) // should not panic
	})
}

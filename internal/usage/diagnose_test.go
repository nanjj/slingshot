package usage

import (
	"bytes"
	"strings"
	"testing"

	"github.com/nanjj/slingshot/internal/i18n"
)

func TestDiagnose(t *testing.T) {
	// Force English locale so test output is predictable
	i18n.SetLocale("en_US")

	u := Usage{Verbatim("add"), Placeholder("name")}
	parsed, err := u.Parse([]string{"add", "hello"})
	if err != nil {
		t.Fatal(err)
	}

	var buf bytes.Buffer
	u.diagnose(&buf, parsed)
	output := buf.String()

	if !strings.Contains(output, "Usage:") {
		t.Errorf("diagnose output missing 'Usage:': %s", output)
	}
	if !strings.Contains(output, "hello") {
		t.Errorf("diagnose output missing 'hello': %s", output)
	}
}

func TestDiagnoseEmpty(t *testing.T) {
	i18n.SetLocale("en_US")
	u := Usage{Verbatim("x")}
	u.diagnose(&bytes.Buffer{}, nil) // should not panic
}

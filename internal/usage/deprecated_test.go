package usage

import (
	"bytes"
	"os"
	"strings"
	"testing"
)

func TestDeprecated(t *testing.T) {
	// deprecated 没有公开构造函数, 但包内测试可以直接构造。
	warning := "use 'newcmd' instead"
	d := deprecated{atom: Verbatim("old"), warning: warning}

	// capture stderr
	r, w, _ := os.Pipe()
	stdErr := os.Stderr
	os.Stderr = w

	p, err := d.Parse(stringPtr("old"))
	_ = w.Close()
	os.Stderr = stdErr

	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	if p.String != "old" {
		t.Errorf("got %q, want %q", p.String, "old")
	}

	// check warning text
	var buf bytes.Buffer
	buf.ReadFrom(r)
	output := buf.String()
	if !strings.Contains(output, warning) {
		t.Errorf("stderr missing warning %q: %s", warning, output)
	}
}

func TestDeprecatedRender(t *testing.T) {
	d := deprecated{atom: Verbatim("old"), warning: "use new"}
	if d.Render() != "old" {
		t.Errorf("got %q, want %q", d.Render(), "old")
	}
}

// stringPtr returns a pointer to a string slice, matching Parse signature.
func stringPtr(s ...string) *[]string { return &s }

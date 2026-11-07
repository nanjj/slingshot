package main

import (
	"strings"
	"testing"
)

func TestWritePotShape(t *testing.T) {
	infos := map[string]*msgInfo{
		"Show help": {
			Locations: []string{"cmd/slingshot/main.go:10"},
		},
		"Error: %v": {
			Locations: []string{"cmd/slingshot/err.go:4"},
			Flags:     []string{"c-format"},
		},
		"line1\nline2": {
			Locations: []string{"cmd/slingshot/x.go:2"},
		},
	}

	got := writePot(infos, potHeader("demo", "bugs@example.com", "2026-01-01 00:00+0000"))

	// Keywords are aligned to column 8 and continuation lines indented by 8,
	// which is what xgettext emits and what msgmerge expects.
	for _, want := range []string{
		"#: cmd/slingshot/err.go:4\n#, c-format\nmsgid   \"Error: %v\"\nmsgstr  \"\"\n\n",
		"#: cmd/slingshot/main.go:10\nmsgid   \"Show help\"\nmsgstr  \"\"\n\n",
		"msgid   \"line1\\n\"\n        \"line2\"\nmsgstr  \"\"\n\n",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("writePot output missing:\n%s\n--- got ---\n%s", want, got)
		}
	}

	// Header fields come from arguments, not hard-coded constants.
	for _, want := range []string{
		"Project-Id-Version: demo\\n",
		"Report-Msgid-Bugs-To: bugs@example.com\\n",
		"POT-Creation-Date: 2026-01-01 00:00+0000\\n",
		"charset=UTF-8",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("header missing %q:\n%s", want, got)
		}
	}

	if strings.Contains(got, "slingshot 0.1.0") {
		t.Errorf("header leaked the slingshot default package name")
	}
}

func TestPotSegments(t *testing.T) {
	for _, tc := range []struct {
		in   string
		want []string
	}{
		{"plain", []string{"plain"}},
		{"a\nb", []string{"a\n", "b"}},
		{"a\n", []string{"a\n"}},
		{"a\n\nb", []string{"a\n", "\n", "b"}},
	} {
		got := potSegments(tc.in)
		if len(got) != len(tc.want) {
			t.Fatalf("potSegments(%q) = %q, want %q", tc.in, got, tc.want)
		}
		for i := range got {
			if got[i] != tc.want[i] {
				t.Errorf("potSegments(%q)[%d] = %q, want %q", tc.in, i, got[i], tc.want[i])
			}
		}
	}
}

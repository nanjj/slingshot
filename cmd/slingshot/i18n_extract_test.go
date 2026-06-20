package main

import (
	"os"
	"path/filepath"
	"sort"
	"testing"
)

// TestASTExtraction tests the AST-based msgid extractor against various
// Go source patterns.
func TestASTExtraction(t *testing.T) {
	tests := []struct {
		name    string
		source  string // Go source code containing i18n.G() calls
		want    []string
		wantErr bool
	}{
		{
			name:   "double-quoted string",
			source: `package p; func f() { _ = i18n.G("hello world") }`,
			want:   []string{"hello world"},
		},
		{
			name:   "backtick string",
			source: "package p; func f() { _ = i18n.G(`hello world`) }",
			want:   []string{"hello world"},
		},
		{
			name:   "string with escape sequences",
			source: `package p; func f() { _ = i18n.G("hello\nworld") }`,
			want:   []string{"hello\nworld"},
		},
		{
			name:   "string with quote escape",
			source: `package p; func f() { _ = i18n.G("say \"hello\"") }`,
			want:   []string{`say "hello"`},
		},
		{
			name:   "string with backslash",
			source: `package p; func f() { _ = i18n.G("path\\to\\file") }`,
			want:   []string{`path\to\file`},
		},
		{
			name:   "string concatenation",
			source: `package p; func f() { _ = i18n.G("hello " + "world") }`,
			want:   []string{"hello world"},
		},
		{
			name:   "multi-part concatenation",
			source: `package p; func f() { _ = i18n.G("a" + "b" + "c") }`,
			want:   []string{"abc"},
		},
		{
			name:   "concatenation with newline escape",
			source: `package p; func f() { _ = i18n.G("line1\n" + "line2") }`,
			want:   []string{"line1\nline2"},
		},
		{
			name:   "realistic: two i18n.G calls with +",
			source: `package p; func f() { _ = i18n.G("part1" + " part2") }`,
			want:   []string{"part1 part2"},
		},
		{
			name:   "no i18n.G calls",
			source: `package p; func f() { _ = "hello" }`,
			want:   nil,
		},
		{
			name:   "i18n not our package",
			source: `package p; func f() { _ = other.G("hello") }`,
			want:   nil,
		},
		{
			name:   "method call not G",
			source: `package p; func f() { _ = i18n.H("hello") }`,
			want:   nil,
		},
		{
			name:   "tab escape",
			source: `package p; func f() { _ = i18n.G("tab\there") }`,
			want:   []string{"tab\there"},
		},
		{
			name:   "hex escape",
			source: `package p; func f() { _ = i18n.G("\x41\x42\x43") }`,
			want:   []string{"ABC"},
		},
		{
			name:   "unicode escape",
			source: `package p; func f() { _ = i18n.G("\u0041\u0042") }`,
			want:   []string{"AB"},
		},
		{
			name:   "empty string",
			source: `package p; func f() { _ = i18n.G("") }`,
			want:   nil, // empty strings are skipped
		},
		{
			name:   "backtick with newlines",
			source: "package p; func f() { _ = i18n.G(`hello\nworld`) }",
			want:   []string{"hello\nworld"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a temporary .go file
			dir := t.TempDir()
			path := filepath.Join(dir, "test.go")
			if err := os.WriteFile(path, []byte(tt.source), 0644); err != nil {
				t.Fatal(err)
			}

			got, err := extractMsgids(dir)
			if (err != nil) != tt.wantErr {
				t.Errorf("extractMsgids() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			gotSlice := make([]string, 0, len(got))
			for m := range got {
				gotSlice = append(gotSlice, m)
			}
			sort.Strings(gotSlice)
			sort.Strings(tt.want)

			if len(gotSlice) != len(tt.want) {
				t.Errorf("extractMsgids() = %v, want %v", gotSlice, tt.want)
				return
			}
			for i := range gotSlice {
				if gotSlice[i] != tt.want[i] {
					t.Errorf("extractMsgids() = %v, want %v", gotSlice, tt.want)
					return
				}
			}
		})
	}
}

// TestASTExtractionIntegration tests that the extractor works correctly
// across a multi-file package structure.
func TestASTExtractionIntegration(t *testing.T) {
	dir := t.TempDir()

	files := map[string]string{
		"a.go": `package main

import "github.com/nanjj/slingshot/internal/i18n"

func hello() string {
    return i18n.G("Hello, World!")
}
`,
		"b.go": `package main

import "github.com/nanjj/slingshot/internal/i18n"

func goodbye() string {
    return i18n.G("Goodbye, " + "World!")
}
`,
		"skip_test.go": `package main

import "testing"

func TestX(t *testing.T) {
    // .go files are always scanned (no _test.go exclusion)
    _ = t
}
`,
	}

	for name, content := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0644); err != nil {
			t.Fatal(err)
		}
	}

	got, err := extractMsgids(dir)
	if err != nil {
		t.Fatal(err)
	}

	expected := []string{"Hello, World!", "Goodbye, World!"}
	for _, e := range expected {
		if !got[e] {
			t.Errorf("expected msgid %q not found in extracted set: %v", e, got)
		}
	}

	if len(got) != len(expected) {
		t.Errorf("expected %d msgids, got %d: %v", len(expected), len(got), got)
	}
}

// TestASTExtractionSkippedDirs verifies that vendor/ and hidden dirs are skipped.
func TestASTExtractionSkippedDirs(t *testing.T) {
	dir := t.TempDir()

	// Create source file in root
	if err := os.WriteFile(filepath.Join(dir, "main.go"), []byte(`package main

import "github.com/nanjj/slingshot/internal/i18n"

func f() { _ = i18n.G("visible") }
`), 0644); err != nil {
		t.Fatal(err)
	}

	// Create file in vendor/ — should be skipped
	for _, skipDir := range []string{"vendor", ".hidden", "node_modules"} {
		d := filepath.Join(dir, skipDir)
		if err := os.MkdirAll(d, 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(d, "pkg.go"), []byte(`package pkg

import "github.com/nanjj/slingshot/internal/i18n"

func f() { _ = i18n.G("should-be-skipped") }
`), 0644); err != nil {
			t.Fatal(err)
		}
	}

	got, err := extractMsgids(dir)
	if err != nil {
		t.Fatal(err)
	}

	if len(got) != 1 {
		t.Fatalf("expected 1 msgid (from visible file only), got %d: %v", len(got), got)
	}
	if !got["visible"] {
		t.Errorf("expected 'visible' to be extracted")
	}
	if got["should-be-skipped"] {
		t.Errorf("'should-be-skipped' from %s should not be extracted", "vendor")
	}
}

// TestExtractStringLiteral tests the string literal extraction directly.
func TestExtractStringLiteral(t *testing.T) {
	tests := []struct {
		name    string
		source  string // Go source expression string
		want    string
		wantErr bool
	}{
		{
			name:   "simple string",
			source: `"hello"`,
			want:   "hello",
		},
		{
			name:   "escaped quote",
			source: `"say \"hi\""`,
			want:   `say "hi"`,
		},
		{
			name:   "newline escape",
			source: `"line1\nline2"`,
			want:   "line1\nline2",
		},
		{
			name:   "backslash",
			source: `"a\\b"`,
			want:   `a\b`,
		},
	}
	// Note: we can't easily test extractStringLiteral directly since it
	// needs AST nodes. This is a placeholder for future refactoring.
	_ = tests
}

// BenchmarkASTExtraction benchmarks the AST extractor against a realistic
// project structure. Requires project source to be available.
func BenchmarkASTExtraction(b *testing.B) {
	// Use the actual cmd/slingshot directory as a realistic test
	projectDir := "." // relative to CWD which should be project root
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := extractMsgids(projectDir)
		if err != nil {
			b.Fatal(err)
		}
	}
}

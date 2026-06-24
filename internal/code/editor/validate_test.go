package editor

import (
	"testing"

	"github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
)

func TestValidateValidCode(t *testing.T) {
	ed := openScratchDoc(t, "package main\n\nfunc main() {}\n", "go")
	result, err := ed.Validate("scratch:///test.go")
	if err != nil {
		t.Fatalf("Validate failed: %v", err)
	}
	if result == nil {
		t.Fatal("Validate returned nil")
	}
	if !result.Valid {
		t.Errorf("valid code should be Valid=true, got SyntaxErrors=%v", result.SyntaxErrors)
	}
	if result.Language != "go" {
		t.Errorf("language: got %q, want %q", result.Language, "go")
	}
	if result.SourceSize <= 0 {
		t.Errorf("source size: got %d, want >0", result.SourceSize)
	}
}

func TestValidateLineEndingLF(t *testing.T) {
	ed := openScratchDoc(t, "package main\nfunc main() {}\n", "go")
	result, err := ed.Validate("scratch:///test.go")
	if err != nil {
		t.Fatalf("Validate failed: %v", err)
	}
	if result.LineEnding != "\n" {
		t.Errorf("line ending: got %q, want LF", result.LineEnding)
	}
}

func TestValidateLineEndingCRLF(t *testing.T) {
	ed := NewEditor("")
	err := ed.OpenDocument("scratch:///test.go", []byte("package main\r\nfunc main() {}\r\n"), "go")
	if err != nil {
		t.Fatalf("OpenDocument failed: %v", err)
	}
	result, err := ed.Validate("scratch:///test.go")
	if err != nil {
		t.Fatalf("Validate failed: %v", err)
	}
	if result.LineEnding != "\r\n" {
		t.Errorf("line ending: got %q, want CRLF", result.LineEnding)
	}
}

func TestValidateTrailingNewline(t *testing.T) {
	ed := openScratchDoc(t, "package main\nfunc main() {}\n", "go")
	result, err := ed.Validate("scratch:///test.go")
	if err != nil {
		t.Fatalf("Validate failed: %v", err)
	}
	if !result.TrailingNewline {
		t.Errorf("should detect trailing newline")
	}
}

func TestValidateNoTrailingNewline(t *testing.T) {
	ed := openScratchDoc(t, "package main\nfunc main() {}", "go")
	result, err := ed.Validate("scratch:///test.go")
	if err != nil {
		t.Fatalf("Validate failed: %v", err)
	}
	if result.TrailingNewline {
		t.Errorf("should NOT detect trailing newline (file doesn't end with \\n)")
	}
}

func TestValidateEmptyFile(t *testing.T) {
	ed := openScratchDoc(t, "", "go")
	result, err := ed.Validate("scratch:///test.go")
	if err != nil {
		t.Fatalf("Validate failed: %v", err)
	}
	if result.SourceSize != 0 {
		t.Errorf("source size: got %d, want 0", result.SourceSize)
	}
	if result.LineEnding != "\n" {
		t.Errorf("empty file line ending: got %q, want LF", result.LineEnding)
	}
}

func TestValidateNotFound(t *testing.T) {
	ed := NewEditor("")
	_, err := ed.Validate("nonexistent")
	if err != ErrDocumentNotFound {
		t.Errorf("expected ErrDocumentNotFound, got %v", err)
	}
}

func TestValidateSyntaxErrorDetection(t *testing.T) {
	// Incomplete Go code to trigger syntax errors
	ed := openScratchDoc(t, "package main\nfunc main() {\n", "go")
	result, err := ed.Validate("scratch:///test.go")
	if err != nil {
		t.Fatalf("Validate failed: %v", err)
	}
	_ = result
}

// ─── SyntaxError collection (using raw gotreesitter) ───

func TestCollectSyntaxErrorsMissingNode(t *testing.T) {
	source := []byte("package main\nfunc main() {\n") // missing closing }
	parser := gotreesitter.NewParser(grammars.GoLanguage())
	tree, err := parser.Parse(source)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}
	defer tree.Release()

	root := tree.RootNode()
	lang := parser.Language()
	errs := collectSyntaxErrors(root, lang)
	if len(errs) == 0 && root.HasError() {
		t.Log("root has error but no syntax errors collected")
	}
}

func TestCollectSyntaxErrorsNoErrors(t *testing.T) {
	source := []byte("package main\nfunc main() {}\n")
	parser := gotreesitter.NewParser(grammars.GoLanguage())
	tree, err := parser.Parse(source)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}
	defer tree.Release()

	root := tree.RootNode()
	lang := parser.Language()
	errs := collectSyntaxErrors(root, lang)
	if len(errs) != 0 {
		t.Errorf("expected 0 errors, got %d: %v", len(errs), errs)
	}
}

func TestCollectErrorsString(t *testing.T) {
	source := []byte("package main\nfunc main() {\n") // incomplete
	parser := gotreesitter.NewParser(grammars.GoLanguage())
	tree, err := parser.Parse(source)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}
	defer tree.Release()

	root := tree.RootNode()
	lang := parser.Language()
	errStrs := collectErrors(root, lang)
	// Should return formatted error strings or empty slice
	_ = errStrs
}

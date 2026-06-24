package editor

import (
	"testing"
)

// ─── detectEncoding ───

func TestDetectEncodingNoBOM(t *testing.T) {
	enc := detectEncoding([]byte("hello"))
	if enc != InputEncodingUTF8 {
		t.Errorf("no BOM: got %v, want UTF8", enc)
	}
}

func TestDetectEncodingEmpty(t *testing.T) {
	enc := detectEncoding([]byte{})
	if enc != InputEncodingUTF8 {
		t.Errorf("empty: got %v, want UTF8", enc)
	}
}

func TestDetectEncodingUTF16BE(t *testing.T) {
	enc := detectEncoding([]byte{0xFE, 0xFF, 0x00, 0x68})
	if enc != InputEncodingUTF16 {
		t.Errorf("UTF-16BE BOM: got %v, want UTF16", enc)
	}
}

func TestDetectEncodingUTF16LE(t *testing.T) {
	enc := detectEncoding([]byte{0xFF, 0xFE, 0x68, 0x00})
	if enc != InputEncodingUTF16 {
		t.Errorf("UTF-16LE BOM: got %v, want UTF16", enc)
	}
}

// ─── detectLineEnding ───

func TestDetectLineEndingEmpty(t *testing.T) {
	le := detectLineEnding([]byte{})
	if le != "\n" {
		t.Errorf("empty: got %q, want LF", le)
	}
}

func TestDetectLineEndingLF(t *testing.T) {
	le := detectLineEnding([]byte("line1\nline2\n"))
	if le != "\n" {
		t.Errorf("LF: got %q, want LF", le)
	}
}

func TestDetectLineEndingCRLF(t *testing.T) {
	le := detectLineEnding([]byte("line1\r\nline2\r\n"))
	if le != "\r\n" {
		t.Errorf("CRLF: got %q, want CRLF", le)
	}
}

func TestDetectLineEndingMixedLFFirst(t *testing.T) {
	// First newline determines the result
	le := detectLineEnding([]byte("line1\nline2\r\n"))
	if le != "\n" {
		t.Errorf("LF first: got %q, want LF", le)
	}
}

func TestDetectLineEndingMixedCRLFFirst(t *testing.T) {
	le := detectLineEnding([]byte("line1\r\nline2\n"))
	if le != "\r\n" {
		t.Errorf("CRLF first: got %q, want CRLF", le)
	}
}

func TestDetectLineEndingNoNewline(t *testing.T) {
	le := detectLineEnding([]byte("hello world"))
	if le != "\n" {
		t.Errorf("no newline: got %q, want LF (default)", le)
	}
}

func TestDetectLineEndingSingleLineWithNewline(t *testing.T) {
	le := detectLineEnding([]byte("hello\n"))
	if le != "\n" {
		t.Errorf("single LF: got %q, want LF", le)
	}
}

func TestDetectLineEndingSingleLineCRLF(t *testing.T) {
	le := detectLineEnding([]byte("hello\r\n"))
	if le != "\r\n" {
		t.Errorf("single CRLF: got %q, want CRLF", le)
	}
}

func TestDetectLineEndingBareCR(t *testing.T) {
	// \r without \n is not a line ending, should not be detected
	le := detectLineEnding([]byte("hello\rworld"))
	if le != "\n" {
		t.Errorf("bare CR: got %q, want LF (default)", le)
	}
}

// ─── normalizeLineEndings ───

func TestNormalizeLineEndingsLFDocWithLFInput(t *testing.T) {
	doc := &Document{source: []byte("line1\nline2\n")}
	result := doc.normalizeLineEndings([]byte("hello\nworld\n"))
	if string(result) != "hello\nworld\n" {
		t.Errorf("LF->LF: got %q, want %q", string(result), "hello\nworld\n")
	}
}

func TestNormalizeLineEndingsLFDocWithCRLFInput(t *testing.T) {
	doc := &Document{source: []byte("line1\nline2\n")}
	result := doc.normalizeLineEndings([]byte("hello\r\nworld\r\n"))
	if string(result) != "hello\nworld\n" {
		t.Errorf("CRLF->LF: got %q, want %q", string(result), "hello\nworld\n")
	}
}

func TestNormalizeLineEndingsCRLFDocWithLFInput(t *testing.T) {
	doc := &Document{source: []byte("line1\r\nline2\r\n")}
	result := doc.normalizeLineEndings([]byte("hello\nworld\n"))
	if string(result) != "hello\r\nworld\r\n" {
		t.Errorf("LF->CRLF: got %q, want %q", string(result), "hello\r\nworld\r\n")
	}
}

func TestNormalizeLineEndingsCRLFDocWithCRLFInput(t *testing.T) {
	doc := &Document{source: []byte("line1\r\nline2\r\n")}
	result := doc.normalizeLineEndings([]byte("hello\r\nworld\r\n"))
	// Should not double-convert: \r\n → \n → \r\n
	if string(result) != "hello\r\nworld\r\n" {
		t.Errorf("CRLF->CRLF no double: got %q, want %q", string(result), "hello\r\nworld\r\n")
	}
}

func TestNormalizeLineEndingsLFDocMixedInput(t *testing.T) {
	doc := &Document{source: []byte("line1\nline2\n")}
	// Mixed input with both LF and CRLF
	result := doc.normalizeLineEndings([]byte("hello\r\nworld\nfoo\r\nbar\n"))
	if string(result) != "hello\nworld\nfoo\nbar\n" {
		t.Errorf("mixed->LF: got %q, want %q", string(result), "hello\nworld\nfoo\nbar\n")
	}
}

func TestNormalizeLineEndingsNoNewlineInput(t *testing.T) {
	doc := &Document{source: []byte("line1\nline2\n")}
	result := doc.normalizeLineEndings([]byte("no newline"))
	if string(result) != "no newline" {
		t.Errorf("no newline: got %q, want %q", string(result), "no newline")
	}
}

func TestNormalizeLineEndingsLFToCRLFNoDouble(t *testing.T) {
	// Critical: ensure \r\n input doesn't become \r\r\n after CRLF conversion
	doc := &Document{source: []byte("line1\r\nline2\r\n")}
	result := doc.normalizeLineEndings([]byte("hello\r\nworld"))
	if string(result) != "hello\r\nworld" {
		t.Errorf("LF->CRLF no double: got %q, want %q", string(result), "hello\r\nworld")
	}
}

// ─── firstLineOf ───

func TestFirstLineOfEmpty(t *testing.T) {
	got := firstLineOf([]byte{})
	if got != "" {
		t.Errorf("empty: got %q, want empty", got)
	}
}

func TestFirstLineOfSingleLine(t *testing.T) {
	got := firstLineOf([]byte("hello"))
	if got != "hello" {
		t.Errorf("single: got %q, want %q", got, "hello")
	}
}

func TestFirstLineOfMultiLine(t *testing.T) {
	got := firstLineOf([]byte("first line\nsecond line\n"))
	if got != "first line" {
		t.Errorf("multi: got %q, want %q", got, "first line")
	}
}

func TestFirstLineOfCRLF(t *testing.T) {
	got := firstLineOf([]byte("first line\r\nsecond line"))
	if got != "first line\r" {
		// \r is preserved, it's just the first line before \n
		t.Errorf("CRLF: got %q, want %q", got, "first line\r")
	}
	// Actually detectLanguage uses firstLineOf for shebang detection,
	// and shebang doesn't have \r\n, so this is fine.
}

// ─── detectLanguage ───

func TestDetectLanguageGo(t *testing.T) {
	lang, err := detectLanguage("/path/to/file.go", []byte("package main\n"), "")
	if err != nil {
		t.Fatalf("detectLanguage Go file failed: %v", err)
	}
	if lang == nil {
		t.Fatal("detectLanguage returned nil")
	}
	if lang.Name != "go" {
		t.Errorf("language name: got %q, want %q", lang.Name, "go")
	}
}

func TestDetectLanguageByName(t *testing.T) {
	lang, err := detectLanguage("", nil, "go")
	if err != nil {
		t.Fatalf("detectLanguage by name failed: %v", err)
	}
	if lang == nil {
		t.Fatal("detectLanguage returned nil")
	}
	if lang.Name != "go" {
		t.Errorf("language name: got %q, want %q", lang.Name, "go")
	}
}

func TestDetectLanguageShebang(t *testing.T) {
	source := []byte("#!/usr/bin/env python3\nprint('hello')\n")
	lang, err := detectLanguage("/unknown/script", source, "")
	if err != nil {
		t.Fatalf("detectLanguage shebang failed: %v", err)
	}
	if lang == nil {
		t.Fatal("detectLanguage returned nil")
	}
	// Should detect Python from shebang
}

func TestDetectLanguageEmptyFileNoExtension(t *testing.T) {
	_, err := detectLanguage("/unknown/empty", []byte{}, "")
	if err == nil {
		t.Error("expected error for empty file with unknown extension and no shebang")
	}
}

func TestDetectLanguageEmptySourceNoExtension(t *testing.T) {
	// Empty source with no extension and no language name should fail
	_, err := detectLanguage("", nil, "")
	if err == nil {
		t.Error("expected error for empty source with no extension and no language name")
	}
}

func TestDetectLanguageUnsupportedName(t *testing.T) {
	_, err := detectLanguage("", nil, "nonexistent-lang-12345")
	if err != ErrUnsupportedLanguage {
		t.Errorf("expected ErrUnsupportedLanguage, got %v", err)
	}
}

// ─── resolveURI ───

func TestResolveURIFile(t *testing.T) {
	path, err := resolveURI("file:///path/to/file.go")
	if err != nil {
		t.Fatalf("resolveURI file:// failed: %v", err)
	}
	if path != "/path/to/file.go" {
		t.Errorf("got %q, want %q", path, "/path/to/file.go")
	}
}

func TestResolveURIFileNoLeadingSlash(t *testing.T) {
	// file://host/path (without leading slash after host)
	path, err := resolveURI("file://relative/path.go")
	if err != nil {
		t.Fatalf("resolveURI file:// relative failed: %v", err)
	}
	if path != "relative/path.go" {
		t.Errorf("got %q, want %q", path, "relative/path.go")
	}
}

func TestResolveURIAbsolute(t *testing.T) {
	path, err := resolveURI("/absolute/path/to/file.go")
	if err != nil {
		t.Fatalf("resolveURI absolute failed: %v", err)
	}
	if path != "/absolute/path/to/file.go" {
		t.Errorf("got %q, want %q", path, "/absolute/path/to/file.go")
	}
}

func TestResolveURIRelative(t *testing.T) {
	_, err := resolveURI("relative/path.go")
	if err != ErrNonFileURI {
		t.Errorf("expected ErrNonFileURI, got %v", err)
	}
}

func TestResolveURIScratch(t *testing.T) {
	_, err := resolveURI("scratch:///test.go")
	if err != ErrNonFileURI {
		t.Errorf("expected ErrNonFileURI, got %v", err)
	}
}

func TestResolveURIEmpty(t *testing.T) {
	_, err := resolveURI("")
	if err != ErrNonFileURI {
		t.Errorf("expected ErrNonFileURI, got %v", err)
	}
}

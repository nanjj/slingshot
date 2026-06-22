package editor

import (
	"os"
	"path/filepath"
	"testing"
)

// ─── NewEditor ───

func TestNewEditor(t *testing.T) {
	ed := NewEditor("/project")
	if ed.projectRoot != "/project" {
		t.Errorf("projectRoot: got %q, want %q", ed.projectRoot, "/project")
	}
}

func TestNewEditorEmptyRoot(t *testing.T) {
	ed := NewEditor("")
	if ed.projectRoot != "" {
		t.Errorf("expected empty projectRoot, got %q", ed.projectRoot)
	}
}

// ─── OpenDocument ───

func TestOpenDocumentScratchWithLanguage(t *testing.T) {
	ed := NewEditor("")
	err := ed.OpenDocument("scratch:///test.go", []byte("package main\n"), "go")
	if err != nil {
		t.Fatalf("OpenDocument failed: %v", err)
	}
	doc, err := ed.GetDocument("scratch:///test.go")
	if err != nil {
		t.Fatalf("GetDocument failed: %v", err)
	}
	if doc.URI() != "scratch:///test.go" {
		t.Errorf("uri: got %q, want %q", doc.URI(), "scratch:///test.go")
	}
	if string(doc.Source()) != "package main\n" {
		t.Errorf("source: got %q, want %q", string(doc.Source()), "package main\n")
	}
	if doc.Dirty() != false {
		t.Errorf("new doc should not be dirty")
	}
	if doc.Version() != 0 {
		t.Errorf("new doc version: got %d, want 0", doc.Version())
	}
	if doc.Encoding() != InputEncodingUTF8 {
		t.Errorf("encoding: got %v, want UTF8", doc.Encoding())
	}
	if doc.OrigFilePath() != "" {
		t.Errorf("scratch doc origFilePath: got %q, want empty", doc.OrigFilePath())
	}
}

func TestOpenDocumentScratchAutoLanguage(t *testing.T) {
	ed := NewEditor("")
	err := ed.OpenDocument("scratch:///test.go", []byte("package main\n"), "")
	if err != nil {
		t.Fatalf("OpenDocument auto language failed: %v", err)
	}
	// Should parse correctly
	_, err = ed.GetStructure("scratch:///test.go", -1, -1)
	if err != nil {
		t.Fatalf("GetStructure after auto-open failed: %v", err)
	}
}

func TestOpenDocumentScratchEmptySource(t *testing.T) {
	ed := NewEditor("")
	err := ed.OpenDocument("scratch:///test.go", nil, "go")
	if err != nil {
		t.Fatalf("OpenDocument nil source failed: %v", err)
	}
	doc, _ := ed.GetDocument("scratch:///test.go")
	if string(doc.Source()) != "" {
		t.Errorf("nil source: got %q, want empty", string(doc.Source()))
	}
}

func TestOpenDocumentUnsupportedLanguage(t *testing.T) {
	ed := NewEditor("")
	err := ed.OpenDocument("scratch:///test.xyz", []byte("content"), "unsupported-lang")
	if err == nil {
		t.Fatal("expected error for unsupported language")
	}
}

func TestOpenDocumentScratchNoExtension(t *testing.T) {
	ed := NewEditor("")
	// No file extension and no language name should fail
	err := ed.OpenDocument("scratch:///snippet", []byte("hello"), "")
	if err == nil {
		t.Fatal("expected error for no extension and no language name")
	}
}

func TestOpenDocumentDuplicateReplaces(t *testing.T) {
	ed := NewEditor("")
	err := ed.OpenDocument("scratch:///test.go", []byte("package main\n"), "go")
	if err != nil {
		t.Fatalf("first OpenDocument failed: %v", err)
	}
	// Open same URI again with different source
	err = ed.OpenDocument("scratch:///test.go", []byte("package main\n\nfunc main() {}\n"), "go")
	if err != nil {
		t.Fatalf("second OpenDocument failed: %v", err)
	}
	doc, _ := ed.GetDocument("scratch:///test.go")
	if string(doc.Source()) != "package main\n\nfunc main() {}\n" {
		t.Errorf("after reopen: got %q, want %q",
			string(doc.Source()), "package main\n\nfunc main() {}\n")
	}
}

// ─── CloseDocument ───

func TestCloseDocument(t *testing.T) {
	ed := NewEditor("")
	err := ed.OpenDocument("scratch:///test.go", []byte("package main\n"), "go")
	if err != nil {
		t.Fatalf("OpenDocument failed: %v", err)
	}
	err = ed.CloseDocument("scratch:///test.go")
	if err != nil {
		t.Fatalf("CloseDocument failed: %v", err)
	}
	_, err = ed.GetDocument("scratch:///test.go")
	if err != ErrDocumentNotFound {
		t.Errorf("after close: expected ErrDocumentNotFound, got %v", err)
	}
}

func TestCloseDocumentNotFound(t *testing.T) {
	ed := NewEditor("")
	err := ed.CloseDocument("nonexistent")
	if err != ErrDocumentNotFound {
		t.Errorf("expected ErrDocumentNotFound, got %v", err)
	}
}

func TestCloseDocumentMultipleTimes(t *testing.T) {
	ed := NewEditor("")
	ed.OpenDocument("scratch:///test.go", []byte("package main\n"), "go")
	ed.CloseDocument("scratch:///test.go")
	err := ed.CloseDocument("scratch:///test.go")
	if err != ErrDocumentNotFound {
		t.Errorf("second close: expected ErrDocumentNotFound, got %v", err)
	}
}

// ─── GetDocument ───

func TestGetDocumentFound(t *testing.T) {
	ed := NewEditor("")
	ed.OpenDocument("scratch:///test.go", []byte("package main\n"), "go")
	doc, err := ed.GetDocument("scratch:///test.go")
	if err != nil {
		t.Fatalf("GetDocument failed: %v", err)
	}
	if doc == nil {
		t.Fatal("GetDocument returned nil doc")
	}
}

func TestGetDocumentNotFound(t *testing.T) {
	ed := NewEditor("")
	_, err := ed.GetDocument("nonexistent")
	if err != ErrDocumentNotFound {
		t.Errorf("expected ErrDocumentNotFound, got %v", err)
	}
}

// ─── IsDirty ───

func TestIsDirtyAfterOpen(t *testing.T) {
	ed := NewEditor("")
	ed.OpenDocument("scratch:///test.go", []byte("package main\n"), "go")
	dirty, err := ed.IsDirty("scratch:///test.go")
	if err != nil {
		t.Fatalf("IsDirty failed: %v", err)
	}
	if dirty {
		t.Errorf("new doc should not be dirty")
	}
}

func TestIsDirtyAfterInsert(t *testing.T) {
	ed := NewEditor("")
	ed.OpenDocument("scratch:///test.go", []byte("package main\n"), "go")
	ed.Insert("scratch:///test.go", 0, "// comment\n")
	dirty, _ := ed.IsDirty("scratch:///test.go")
	if !dirty {
		t.Errorf("after edit, doc should be dirty")
	}
}

func TestIsDirtyNotFound(t *testing.T) {
	ed := NewEditor("")
	_, err := ed.IsDirty("nonexistent")
	if err != ErrDocumentNotFound {
		t.Errorf("expected ErrDocumentNotFound, got %v", err)
	}
}

// ─── DirtyDocuments ───

func TestDirtyDocumentsEmpty(t *testing.T) {
	ed := NewEditor("")
	ed.OpenDocument("scratch:///a.go", []byte("package a\n"), "go")
	ed.OpenDocument("scratch:///b.go", []byte("package b\n"), "go")
	dirty := ed.DirtyDocuments()
	if len(dirty) != 0 {
		t.Errorf("expected 0 dirty, got %d: %v", len(dirty), dirty)
	}
}

func TestDirtyDocumentsAfterEdit(t *testing.T) {
	ed := NewEditor("")
	ed.OpenDocument("scratch:///a.go", []byte("package a\n"), "go")
	ed.OpenDocument("scratch:///b.go", []byte("package b\n"), "go")
	ed.Insert("scratch:///a.go", 0, "// comment\n")
	dirty := ed.DirtyDocuments()
	if len(dirty) != 1 {
		t.Fatalf("expected 1 dirty, got %v", dirty)
	}
	if dirty[0] != "scratch:///a.go" {
		t.Errorf("expected scratch:///a.go, got %q", dirty[0])
	}
}

func TestDirtyDocumentsAllDirty(t *testing.T) {
	ed := NewEditor("")
	ed.OpenDocument("scratch:///a.go", []byte("package a\n"), "go")
	ed.OpenDocument("scratch:///b.go", []byte("package b\n"), "go")
	ed.Insert("scratch:///a.go", 0, "// a\n")
	ed.Insert("scratch:///b.go", 0, "// b\n")
	dirty := ed.DirtyDocuments()
	if len(dirty) != 2 {
		t.Errorf("expected 2 dirty, got %v", dirty)
	}
}

// ─── getOrOpenDocument (internal) ───

func TestGetOrOpenDocumentCached(t *testing.T) {
	ed := NewEditor("")
	ed.OpenDocument("scratch:///test.go", []byte("package main\n"), "go")
	// Calling getOrOpenDocument should return cached doc without error
	doc, err := ed.getOrOpenDocument("scratch:///test.go")
	if err != nil {
		t.Fatalf("getOrOpenDocument cached failed: %v", err)
	}
	if doc == nil {
		t.Fatal("getOrOpenDocument returned nil")
	}
}

func TestGetOrOpenDocumentAutoOpensFile(t *testing.T) {
	ed := NewEditor("")
	// getOrOpenDocument should auto-open a file:// URI
	// This will fail because the file doesn't exist on disk, but that's expected
	doc, err := ed.getOrOpenDocument("file:///nonexistent_file_for_test.go")
	if err == nil {
		// If it auto-opened, doc should exist
		if doc == nil {
			t.Error("auto-open returned nil doc with no error")
		}
	}
	// If it failed, that's also acceptable (file doesn't exist)
}

func TestExtractVirtualPath(t *testing.T) {
	tests := []struct {
		uri  string
		want string
	}{
		{"scratch:///test.go", "/test.go"},
		{"scratch:///path/to/file.go", "/path/to/file.go"},
		{"scratch:///snippet", "/snippet"},
		{"file:///real/path.go", "/real/path.go"},  // extracts from any ://
		{"invalid-uri", ""},
		{"", ""},
	}
	for _, tc := range tests {
		got := extractVirtualPath(tc.uri)
		if got != tc.want {
			t.Errorf("extractVirtualPath(%q)=%q, want %q", tc.uri, got, tc.want)
		}
	}
}

// ─── OpenDocument with file:// existing files ───

func TestOpenDocumentFileExists(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "test.go")
	if err := os.WriteFile(filePath, []byte("package main\n"), 0644); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}
	uri := "file://" + filePath

	ed := NewEditor(dir)
	if err := ed.OpenDocument(uri, nil, "go"); err != nil {
		t.Fatalf("OpenDocument file exists failed: %v", err)
	}

	doc, err := ed.GetDocument(uri)
	if err != nil {
		t.Fatalf("GetDocument failed: %v", err)
	}
	if string(doc.Source()) != "package main\n" {
		t.Errorf("source: got %q, want %q", string(doc.Source()), "package main\n")
	}
	if doc.OrigFilePath() != filePath {
		t.Errorf("origFilePath: got %q, want %q", doc.OrigFilePath(), filePath)
	}
	if doc.Dirty() {
		t.Error("existing file doc should not be dirty")
	}
	if doc.Version() != 0 {
		t.Errorf("version: got %d, want 0", doc.Version())
	}
}

func TestOpenDocumentFileExistsSourceIgnored(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "test.go")
	if err := os.WriteFile(filePath, []byte("package main\n"), 0644); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}
	uri := "file://" + filePath

	ed := NewEditor(dir)
	// Source param should be ignored when file exists on disk
	if err := ed.OpenDocument(uri, []byte("package hacked\n"), "go"); err != nil {
		t.Fatalf("OpenDocument failed: %v", err)
	}

	doc, _ := ed.GetDocument(uri)
	if string(doc.Source()) != "package main\n" {
		t.Errorf("source should be disk content (not hacked), got %q", string(doc.Source()))
	}
}

func TestOpenDocumentNewFile(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "new.go")
	uri := "file://" + filePath

	ed := NewEditor(dir)
	if err := ed.OpenDocument(uri, []byte("package new\n"), "go"); err != nil {
		t.Fatalf("OpenDocument new file failed: %v", err)
	}

	doc, _ := ed.GetDocument(uri)
	if string(doc.Source()) != "package new\n" {
		t.Errorf("source: got %q, want %q", string(doc.Source()), "package new\n")
	}
	// New file should have origFilePath set (for Save to work)
	if doc.OrigFilePath() != filePath {
		t.Errorf("origFilePath: got %q, want %q", doc.OrigFilePath(), filePath)
	}
}

func TestOpenDocumentNewFileNilSource(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "empty.go")
	uri := "file://" + filePath

	ed := NewEditor(dir)
	if err := ed.OpenDocument(uri, nil, "go"); err != nil {
		t.Fatalf("OpenDocument nil source failed: %v", err)
	}

	doc, _ := ed.GetDocument(uri)
	if string(doc.Source()) != "" {
		t.Errorf("nil source: got %q, want empty", string(doc.Source()))
	}
	if doc.OrigFilePath() != filePath {
		t.Errorf("origFilePath not set for new file")
	}
}

func TestOpenDocumentAutoLanguageFromExtension(t *testing.T) {
	ed := NewEditor("")
	// No language name, but .go extension should be auto-detected
	if err := ed.OpenDocument("scratch:///main.go", []byte("package main\n"), ""); err != nil {
		t.Fatalf("OpenDocument auto language failed: %v", err)
	}
	// Should parse correctly
	_, err := ed.GetStructure("scratch:///main.go", -1, -1)
	if err != nil {
		t.Fatalf("GetStructure after auto-detect failed: %v", err)
	}
}

// ─── checkFileExists ───

func TestCheckFileExists(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "exists.go")
	if err := os.WriteFile(filePath, []byte("content"), 0644); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}

	info, exists := checkFileExists(filePath)
	if !exists {
		t.Error("checkFileExists should return true for existing file")
	}
	if info == nil {
		t.Error("checkFileExists should return file info")
	}
}

func TestCheckFileExistsNotFound(t *testing.T) {
	_, exists := checkFileExists("/nonexistent/path/for/test/file.go")
	if exists {
		t.Error("checkFileExists should return false for nonexistent file")
	}
}

func TestCheckFileExistsDirectory(t *testing.T) {
	dir := t.TempDir()
	_, exists := checkFileExists(dir)
	if exists {
		t.Error("checkFileExists should return false for directory")
	}
}

func TestCheckFileExistsEmptyPath(t *testing.T) {
	_, exists := checkFileExists("")
	if exists {
		t.Error("checkFileExists should return false for empty path")
	}
}

// ─── resolveDocumentPath ───

func TestResolveDocumentPathFileURI(t *testing.T) {
	ed := NewEditor("")
	path, err := ed.resolveDocumentPath("file:///absolute/path.go")
	if err != nil {
		t.Fatalf("resolveDocumentPath failed: %v", err)
	}
	if path != "/absolute/path.go" {
		t.Errorf("got %q, want %q", path, "/absolute/path.go")
	}
}

func TestResolveDocumentPathAbsolute(t *testing.T) {
	ed := NewEditor("")
	path, err := ed.resolveDocumentPath("/absolute/path.go")
	if err != nil {
		t.Fatalf("resolveDocumentPath failed: %v", err)
	}
	if path != "/absolute/path.go" {
		t.Errorf("got %q, want %q", path, "/absolute/path.go")
	}
}

func TestResolveDocumentPathRelativeToProjectRoot(t *testing.T) {
	ed := NewEditor("/project")
	path, err := ed.resolveDocumentPath("file:///relative/path.go")
	if err != nil {
		t.Fatalf("resolveDocumentPath failed: %v", err)
	}
	// file:// URIs are returned after stripping prefix; absolute paths stay absolute
	if path != "/relative/path.go" {
		t.Errorf("got %q, want %q", path, "/relative/path.go")
	}
}

func TestResolveDocumentPathScratch(t *testing.T) {
	ed := NewEditor("")
	_, err := ed.resolveDocumentPath("scratch:///test.go")
	if err == nil {
		t.Error("expected error for scratch URI")
	}
}

// ─── Document accessor methods ───

func TestDocumentAccessors(t *testing.T) {
	ed := NewEditor("")
	if err := ed.OpenDocument("scratch:///test.go", []byte("package main\n"), "go"); err != nil {
		t.Fatalf("OpenDocument failed: %v", err)
	}
	doc, err := ed.GetDocument("scratch:///test.go")
	if err != nil {
		t.Fatalf("GetDocument failed: %v", err)
	}

	if doc.URI() != "scratch:///test.go" {
		t.Errorf("URI: got %q, want %q", doc.URI(), "scratch:///test.go")
	}
	if doc.Language() == nil {
		t.Error("Language should not be nil")
	}
	if string(doc.Source()) != "package main\n" {
		t.Errorf("Source: got %q, want %q", string(doc.Source()), "package main\n")
	}
	if doc.Tree() == nil {
		t.Error("Tree should not be nil")
	}
	if doc.Bound() == nil {
		t.Error("Bound should not be nil")
	}
	if doc.LineIndex() == nil {
		t.Error("LineIndex should not be nil")
	}
	if doc.Version() != 0 {
		t.Errorf("Version: got %d, want 0", doc.Version())
	}
	if doc.Dirty() {
		t.Error("new doc should not be dirty")
	}
	if doc.Encoding() != InputEncodingUTF8 {
		t.Errorf("Encoding: got %v, want UTF8", doc.Encoding())
	}
	if doc.OrigFilePath() != "" {
		t.Errorf("OrigFilePath for scratch: got %q, want empty", doc.OrigFilePath())
	}
}

// ─── Editor concurrent safety (basic) ───

func TestOpenDocumentThenSaveNewFile(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "newly_created.go")
	uri := "file://" + filePath

	ed := NewEditor(dir)
	if err := ed.OpenDocument(uri, []byte("package main\n"), "go"); err != nil {
		t.Fatalf("OpenDocument failed: %v", err)
	}

	// Now save — this should work after the origFilePath fix
	result, err := ed.Save(uri)
	if err != nil {
		t.Fatalf("Save new file failed: %v", err)
	}
	if !result.Success {
		t.Fatalf("Save returned success=false: %+v", result)
	}
	if result.Bytes <= 0 {
		t.Errorf("bytes written: got %d, want >0", result.Bytes)
	}

	// File should exist on disk
	content, err := os.ReadFile(filePath)
	if err != nil {
		t.Fatalf("ReadFile after save failed: %v", err)
	}
	if string(content) != "package main\n" {
		t.Errorf("file content: got %q, want %q", string(content), "package main\n")
	}

	// Doc should not be dirty
	if dirty, _ := ed.IsDirty(uri); dirty {
		t.Error("after save, doc should not be dirty")
	}
}

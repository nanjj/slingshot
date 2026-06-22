package editor

import (
	"testing"
)

// ─── applyEdit (via Insert) ───

func TestApplyEditIncrementalState(t *testing.T) {
	ed := NewEditor("")
	if err := ed.OpenDocument("scratch:///test.go", []byte("package main\n"), "go"); err != nil {
		t.Fatalf("OpenDocument failed: %v", err)
	}
	doc, _ := ed.GetDocument("scratch:///test.go")

	initialVersion := doc.Version()
	if initialVersion != 0 {
		t.Errorf("initial version: got %d, want 0", initialVersion)
	}
	if doc.Dirty() {
		t.Error("new doc should not be dirty")
	}

	// Insert triggers applyEdit internally
	result, err := ed.Insert("scratch:///test.go", 0, "// comment\n")
	if err != nil {
		t.Fatalf("Insert failed: %v", err)
	}
	if !result.Success {
		t.Fatal("Insert should succeed")
	}

	// Verify state after applyEdit
	if doc.Version() != 1 {
		t.Errorf("version after edit: got %d, want 1", doc.Version())
	}
	if !doc.Dirty() {
		t.Error("after edit, doc should be dirty")
	}
	if doc.Bound() == nil {
		t.Error("Bound should not be nil after edit")
	}
	if doc.Tree() == nil {
		t.Error("Tree should not be nil after edit")
	}
}

func TestApplyEditMultipleTimes(t *testing.T) {
	ed := NewEditor("")
	if err := ed.OpenDocument("scratch:///test.go", []byte("package main\n"), "go"); err != nil {
		t.Fatalf("OpenDocument failed: %v", err)
	}
	doc, _ := ed.GetDocument("scratch:///test.go")

	// "package main\n" = 13 bytes
	// Insert 1: "// first\n" at 0 → "// first\npackage main\n" (22 bytes)
	ed.Insert("scratch:///test.go", 0, "// first\n")
	if doc.Version() != 1 {
		t.Errorf("after first insert: version=%d, want 1", doc.Version())
	}

	// Insert 2: "// second\n" at 9 → "// first\n// second\npackage main\n"
	ed.Insert("scratch:///test.go", 9, "// second\n")
	if doc.Version() != 2 {
		t.Errorf("after second insert: version=%d, want 2", doc.Version())
	}

	// Verify the cumulative result
	want := "// first\n// second\npackage main\n"
	if string(doc.Source()) != want {
		t.Errorf("after multiple inserts: got %q, want %q", string(doc.Source()), want)
	}

	// Verify tree is still valid after all edits
	info, err := ed.GetStructure("scratch:///test.go", -1, -1)
	if err != nil {
		t.Fatalf("GetStructure failed after edits: %v", err)
	}
	if info.Type != "source_file" {
		t.Errorf("root type: got %q, want %q", info.Type, "source_file")
	}
}

// ─── applyEdits (via Replace, which uses Rewriter internally) ───

func TestApplyEditsWithRewriter(t *testing.T) {
	ed := openScratchDoc(t, "package main\n\nfunc oldFunc() {}\n", "go")
	doc, _ := ed.GetDocument("scratch:///test.go")

	// Replace uses Rewriter → many InputEdits → applyEdits
	// Replace "oldFunc" (19-26) with "newFunc"
	result, err := ed.Replace("scratch:///test.go", 19, 26, "newFunc")
	if err != nil {
		t.Fatalf("Replace failed: %v", err)
	}
	if !result.Success {
		t.Fatal("Replace should succeed")
	}

	want := "package main\n\nfunc newFunc() {}\n"
	if string(doc.Source()) != want {
		t.Errorf("source: got %q, want %q", string(doc.Source()), want)
	}

	// Version should increment
	if doc.Version() != 1 {
		t.Errorf("version: got %d, want 1", doc.Version())
	}

	// Tree should still be valid
	info, err := ed.GetStructure("scratch:///test.go", -1, -1)
	if err != nil {
		t.Fatalf("GetStructure failed: %v", err)
	}
	if info.Type != "source_file" {
		t.Errorf("root type: got %q, want %q", info.Type, "source_file")
	}
}

func TestApplyEditsInsertBefore(t *testing.T) {
	// InsertBefore also uses Rewriter → applyEdits
	ed := openScratchDoc(t, "package main\n\nfunc main() {}\n", "go")
	doc, _ := ed.GetDocument("scratch:///test.go")

	sel := NodeSelector{Path: []PathStep{
		{Type: "function_declaration", NamedOnly: true},
	}}

	result, err := ed.InsertBefore("scratch:///test.go", sel, "// before\n")
	if err != nil {
		t.Fatalf("InsertBefore failed: %v", err)
	}
	if !result.Success {
		t.Fatal("InsertBefore should succeed")
	}

	source := string(doc.Source())
	if !contains(source, "// before") {
		t.Errorf("InsertBefore content not found in source: %q", source)
	}
	if doc.Version() != 1 {
		t.Errorf("version: got %d, want 1", doc.Version())
	}
}

// ─── applyEdit with CRLF line endings ───

func TestApplyEditCRLFSource(t *testing.T) {
	ed := NewEditor("")
	if err := ed.OpenDocument("scratch:///test.go", []byte("package main\r\n"), "go"); err != nil {
		t.Fatalf("OpenDocument failed: %v", err)
	}
	doc, _ := ed.GetDocument("scratch:///test.go")

	// Insert with LF input → should be normalized to CRLF
	result, err := ed.Insert("scratch:///test.go", 0, "// header\n")
	if err != nil {
		t.Fatalf("Insert failed: %v", err)
	}
	if !result.Success {
		t.Fatal("Insert should succeed")
	}

	got := string(doc.Source())
	want := "// header\r\npackage main\r\n"
	if got != want {
		t.Errorf("CRLF normalization: got %q, want %q", got, want)
	}
}

// ─── applyEdit with empty source ───

func TestApplyEditEmptySource(t *testing.T) {
	ed := openScratchDoc(t, "", "go")
	doc, _ := ed.GetDocument("scratch:///test.go")

	result, err := ed.Insert("scratch:///test.go", 0, "package main\n")
	if err != nil {
		t.Fatalf("Insert into empty doc failed: %v", err)
	}
	if !result.Success {
		t.Fatal("Insert should succeed")
	}

	if string(doc.Source()) != "package main\n" {
		t.Errorf("source: got %q, want %q", string(doc.Source()), "package main\n")
	}
	if doc.Version() != 1 {
		t.Errorf("version: got %d, want 1", doc.Version())
	}
}

// ─── applyEdit at position past end ───

func TestApplyEditInvalidPosition(t *testing.T) {
	ed := openScratchDoc(t, "package main\n", "go")
	_, err := ed.Insert("scratch:///test.go", 100, "text")
	if err != ErrInvalidPosition {
		t.Errorf("expected ErrInvalidPosition, got %v", err)
	}
}

// ─── applyEdit ByteDiff in EditResult ───

func TestApplyEditByteDiff(t *testing.T) {
	ed := openScratchDoc(t, "package main\n", "go")

	// "// comment\n" is 11 bytes: / / ' ' c o m m e n t \n
	// "// header\n" is 10 bytes:  / / ' ' h e a d e r \n

	// Inserting "// comment\n" at position 0
	result, err := ed.Insert("scratch:///test.go", 0, "// comment\n")
	if err != nil {
		t.Fatalf("Insert failed: %v", err)
	}
	if result.ByteDiff != 11 {
		t.Errorf("ByteDiff: got %d, want 11", result.ByteDiff)
	}

	// Replacing with shorter text → ByteDiff = len(new) - len(old) = 10 - 11 = -1
	result, err = ed.Replace("scratch:///test.go", 0, 11, "// header\n")
	if err != nil {
		t.Fatalf("Replace failed: %v", err)
	}
	if result.ByteDiff != -1 {
		t.Errorf("ByteDiff for shorter replace: got %d, want -1", result.ByteDiff)
	}

	// Deleting the 10-byte header
	result, err = ed.Delete("scratch:///test.go", 0, 10)
	if err != nil {
		t.Fatalf("Delete failed: %v", err)
	}
	if result.ByteDiff != -10 {
		t.Errorf("ByteDiff for delete: got %d, want -10", result.ByteDiff)
	}

	// Verify final source equals original
	doc, _ := ed.GetDocument("scratch:///test.go")
	if string(doc.Source()) != "package main\n" {
		t.Errorf("final source: got %q, want %q", string(doc.Source()), "package main\n")
	}
}

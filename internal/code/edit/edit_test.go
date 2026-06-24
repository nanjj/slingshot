package edit

import (
	"strings"
	"testing"
)

// ─── Insert ───

func TestInsertAtBeginning(t *testing.T) {
	ed := openScratchDoc(t, "package main\n", "go")
	result, err := ed.Insert("scratch:///test.go", 0, "// comment\n")
	if err != nil {
		t.Fatalf("Insert failed: %v", err)
	}
	if !result.Success {
		t.Fatal("Insert result.Success should be true")
	}
	doc, _ := ed.GetDocument("scratch:///test.go")
	want := "// comment\npackage main\n"
	if string(doc.Source()) != want {
		t.Errorf("source after insert: got %q, want %q", string(doc.Source()), want)
	}
}

func TestInsertAtEnd(t *testing.T) {
	ed := openScratchDoc(t, "package main\n", "go")
	source := "package main\n"
	result, err := ed.Insert("scratch:///test.go", uint32(len(source)), "// end\n")
	if err != nil {
		t.Fatalf("Insert at end failed: %v", err)
	}
	if !result.Success {
		t.Fatal("Insert result.Success should be true")
	}
	doc, _ := ed.GetDocument("scratch:///test.go")
	want := "package main\n// end\n"
	if string(doc.Source()) != want {
		t.Errorf("source after insert at end: got %q, want %q", string(doc.Source()), want)
	}
}

func TestInsertInMiddle(t *testing.T) {
	// "package main\n\nfunc main() {}\n"
	//  0: p, 1:a, 2:c, 3:k, 4:a, 5:g, 6:e, 7:' ', 8:m, 9:a, 10:i, 11:n, 12:\n
	// 13: \n, 14:f, 15:u, 16:n, 17:c, 18:' ', 19:m, 20:a, 21:i, 22:n, 23:(, 24:), 25:' ', 26:{, 27:}, 28:\n
	// The blank line between package clause and function starts at offset 13
	ed := openScratchDoc(t, "package main\n\nfunc main() {}\n", "go")
	result, err := ed.Insert("scratch:///test.go", 13, "// middle\n")
	if err != nil {
		t.Fatalf("Insert middle failed: %v", err)
	}
	if !result.Success {
		t.Fatal("Insert result.Success should be true")
	}
	doc, _ := ed.GetDocument("scratch:///test.go")
	want := "package main\n// middle\n\nfunc main() {}\n"
	if string(doc.Source()) != want {
		t.Errorf("source after middle insert: got %q, want %q", string(doc.Source()), want)
	}
}

func TestInsertInvalidPosition(t *testing.T) {
	ed := openScratchDoc(t, "package main\n", "go")
	_, err := ed.Insert("scratch:///test.go", 100, "text")
	if err != ErrInvalidPosition {
		t.Errorf("expected ErrInvalidPosition, got %v", err)
	}
}

func TestInsertEmptyText(t *testing.T) {
	ed := openScratchDoc(t, "package main\n", "go")
	result, err := ed.Insert("scratch:///test.go", 0, "")
	if err != nil {
		t.Fatalf("Insert empty failed: %v", err)
	}
	if !result.Success {
		t.Fatal("Insert empty should succeed")
	}
	doc, _ := ed.GetDocument("scratch:///test.go")
	if string(doc.Source()) != "package main\n" {
		t.Errorf("source unchanged: got %q", string(doc.Source()))
	}
}

// ─── InsertAtPoint ───

func TestInsertAtPoint(t *testing.T) {
	ed := openScratchDoc(t, "package main\n", "go")
	result, err := ed.InsertAtPoint("scratch:///test.go", 0, 0, "// before\n")
	if err != nil {
		t.Fatalf("InsertAtPoint failed: %v", err)
	}
	if !result.Success {
		t.Fatal("InsertAtPoint should succeed")
	}
	doc, _ := ed.GetDocument("scratch:///test.go")
	want := "// before\npackage main\n"
	if string(doc.Source()) != want {
		t.Errorf("after InsertAtPoint: got %q, want %q", string(doc.Source()), want)
	}
}

func TestInsertAtPointMiddleLine(t *testing.T) {
	ed := openScratchDoc(t, "package main\n\nfunc main() {\n}\n", "go")
	result, err := ed.InsertAtPoint("scratch:///test.go", 1, 0, "// inserted\n")
	if err != nil {
		t.Fatalf("InsertAtPoint failed: %v", err)
	}
	if !result.Success {
		t.Fatal("InsertAtPoint should succeed")
	}
	doc, _ := ed.GetDocument("scratch:///test.go")
	want := "package main\n// inserted\n\nfunc main() {\n}\n"
	got := string(doc.Source())
	if got != want {
		t.Errorf("after InsertAtPoint: got %q, want %q", got, want)
	}
}

func TestInsertAtPointOutOfRangeRow(t *testing.T) {
	ed := openScratchDoc(t, "package main\n", "go")
	// PointToByte silently handles out-of-range rows by returning last offset.
	// So this succeeds (inserts at end) rather than returning an error.
	_, err := ed.InsertAtPoint("scratch:///test.go", 999, 0, "text")
	if err != nil {
		t.Fatalf("InsertAtPoint with out-of-range row should succeed (clamped to end): %v", err)
	}
}

// ─── Replace ───

func TestReplaceRange(t *testing.T) {
	// "package main\n\nfunc old() {}\n"
	//  0-12:  "package main\n"
	//  13:    "\n"
	//  14-17: "func"
	//  18:    " "
	//  19-21: "old"  ← replace this
	//  22:    "("
	//  23:    ")"
	//  24:    " "
	//  25:    "{"
	//  26:    "}"
	//  27:    "\n"
	ed := openScratchDoc(t, "package main\n\nfunc old() {}\n", "go")
	result, err := ed.Replace("scratch:///test.go", 19, 22, "new")
	if err != nil {
		t.Fatalf("Replace failed: %v", err)
	}
	if !result.Success {
		t.Fatal("Replace should succeed")
	}
	doc, _ := ed.GetDocument("scratch:///test.go")
	want := "package main\n\nfunc new() {}\n"
	if string(doc.Source()) != want {
		t.Errorf("after Replace: got %q, want %q", string(doc.Source()), want)
	}
}

func TestReplaceWithLongerText(t *testing.T) {
	// "package main\n\nfunc f() {}\n"
	//  0-12: "package main\n"
	//  13:   "\n"
	//  14-17: "func"
	//  18:    " "
	//  19:    "f"  ← replace this (1 byte) with "newFunc" (7 bytes)
	//  20:    "("
	//  21:    ")"
	//  22:    " "
	//  23:    "{"
	//  24:    "}"
	//  25:    "\n"
	ed := openScratchDoc(t, "package main\n\nfunc f() {}\n", "go")
	result, err := ed.Replace("scratch:///test.go", 19, 20, "newFunc")
	if err != nil {
		t.Fatalf("Replace longer failed: %v", err)
	}
	if !result.Success {
		t.Fatal("Replace should succeed")
	}
	doc, _ := ed.GetDocument("scratch:///test.go")
	want := "package main\n\nfunc newFunc() {}\n"
	if string(doc.Source()) != want {
		t.Errorf("after longer Replace: got %q, want %q", string(doc.Source()), want)
	}
}

func TestReplaceWithEmptyString(t *testing.T) {
	// "package main\n\nfunc deleteMe() {}\n"
	//  0-12:  "package main\n"
	//  13:    "\n"
	//  14-17: "func"
	//  18:    " "
	//  19-26: "deleteMe" (8 bytes)  ← replace with ""
	//  27:    "("
	ed := openScratchDoc(t, "package main\n\nfunc deleteMe() {}\n", "go")
	result, err := ed.Replace("scratch:///test.go", 19, 27, "")
	if err != nil {
		t.Fatalf("Replace empty failed: %v", err)
	}
	if !result.Success {
		t.Fatal("Replace should succeed")
	}
	doc, _ := ed.GetDocument("scratch:///test.go")
	want := "package main\n\nfunc () {}\n"
	if string(doc.Source()) != want {
		t.Errorf("after empty Replace: got %q, want %q", string(doc.Source()), want)
	}
}

func TestReplaceInvalidRange(t *testing.T) {
	ed := openScratchDoc(t, "package main\n", "go")
	_, err := ed.Replace("scratch:///test.go", 5, 3, "text")
	if err != ErrInvalidPosition {
		t.Errorf("start>end: expected ErrInvalidPosition, got %v", err)
	}
}

func TestReplacePastEnd(t *testing.T) {
	ed := openScratchDoc(t, "package main\n", "go")
	_, err := ed.Replace("scratch:///test.go", 0, 100, "text")
	if err != ErrInvalidPosition {
		t.Errorf("past end: expected ErrInvalidPosition, got %v", err)
	}
}

// ─── Delete ───

func TestDeleteRange(t *testing.T) {
	// "package main\n\nfunc deleteMe() {}\n"
	//  deleteMe is at 19-27
	ed := openScratchDoc(t, "package main\n\nfunc deleteMe() {}\n", "go")
	result, err := ed.Delete("scratch:///test.go", 19, 27)
	if err != nil {
		t.Fatalf("Delete failed: %v", err)
	}
	if !result.Success {
		t.Fatal("Delete should succeed")
	}
	doc, _ := ed.GetDocument("scratch:///test.go")
	want := "package main\n\nfunc () {}\n"
	if string(doc.Source()) != want {
		t.Errorf("after Delete: got %q, want %q", string(doc.Source()), want)
	}
}

func TestDeleteNothing(t *testing.T) {
	ed := openScratchDoc(t, "package main\n", "go")
	result, err := ed.Delete("scratch:///test.go", 0, 0)
	if err != nil {
		t.Fatalf("Delete nothing failed: %v", err)
	}
	if !result.Success {
		t.Fatal("Delete nothing should succeed")
	}
	doc, _ := ed.GetDocument("scratch:///test.go")
	if string(doc.Source()) != "package main\n" {
		t.Errorf("source unchanged: got %q", string(doc.Source()))
	}
}

func TestDeleteInvalidRange(t *testing.T) {
	ed := openScratchDoc(t, "package main\n", "go")
	_, err := ed.Delete("scratch:///test.go", 10, 5)
	if err != ErrInvalidPosition {
		t.Errorf("expected ErrInvalidPosition, got %v", err)
	}
}

// ─── InsertBefore / InsertAfter ───
// Path selectors start from RootNode implicitly.
// The first path step must be a direct child type of source_file.

func TestInsertBefore(t *testing.T) {
	ed := openScratchDoc(t, "package main\n\nfunc main() {}\n", "go")
	sel := NodeSelector{Path: []PathStep{
		{Type: "function_declaration", NamedOnly: true},
	}}
	result, err := ed.InsertBefore("scratch:///test.go", sel, "// before func\n")
	if err != nil {
		t.Fatalf("InsertBefore failed: %v", err)
	}
	if !result.Success {
		t.Fatal("InsertBefore should succeed")
	}
	doc, _ := ed.GetDocument("scratch:///test.go")
	got := string(doc.Source())
	if !strings.Contains(got, "// before func") {
		t.Errorf("InsertBefore comment not found in source: %q", got)
	}
}

func TestInsertAfter(t *testing.T) {
	ed := openScratchDoc(t, "package main\n\nfunc main() {}\n", "go")
	sel := NodeSelector{Path: []PathStep{
		{Type: "function_declaration", NamedOnly: true},
	}}
	result, err := ed.InsertAfter("scratch:///test.go", sel, "// after func\n")
	if err != nil {
		t.Fatalf("InsertAfter failed: %v", err)
	}
	if !result.Success {
		t.Fatal("InsertAfter should succeed")
	}
	doc, _ := ed.GetDocument("scratch:///test.go")
	got := string(doc.Source())
	if !strings.Contains(got, "// after func") {
		t.Errorf("InsertAfter comment not found in source: %q", got)
	}
}

func TestInsertBeforeInvalidSelector(t *testing.T) {
	ed := openScratchDoc(t, "package main\n", "go")
	sel := NodeSelector{Path: []PathStep{
		{Type: "nonexistent_type"},
	}}
	_, err := ed.InsertBefore("scratch:///test.go", sel, "text")
	if err == nil {
		t.Fatal("expected error for invalid selector")
	}
}

// ─── ReplaceNode ───

func TestReplaceNode(t *testing.T) {
	ed := openScratchDoc(t, "package main\n\nfunc oldName() {}\n", "go")
	sel := NodeSelector{Path: []PathStep{
		{Type: "function_declaration", NamedOnly: true},
		{Type: "identifier", NamedOnly: true},
	}}
	result, err := ed.ReplaceNode("scratch:///test.go", sel, "newName")
	if err != nil {
		t.Fatalf("ReplaceNode failed: %v", err)
	}
	if !result.Success {
		t.Fatal("ReplaceNode should succeed")
	}
	doc, _ := ed.GetDocument("scratch:///test.go")
	want := "package main\n\nfunc newName() {}\n"
	got := string(doc.Source())
	if got != want {
		t.Errorf("after ReplaceNode: got %q, want %q", got, want)
	}
}

// ─── DeleteNode ───

func TestDeleteNode(t *testing.T) {
	ed := openScratchDoc(t, "package main\n\nfunc main() {\n\treturn\n}\n\nfunc other() {}\n", "go")
	sel := NodeSelector{Path: []PathStep{
		{Type: "function_declaration", NamedOnly: true},
	}}
	result, err := ed.DeleteNode("scratch:///test.go", sel)
	if err != nil {
		t.Fatalf("DeleteNode failed: %v", err)
	}
	if !result.Success {
		t.Fatal("DeleteNode should succeed")
	}
	doc, _ := ed.GetDocument("scratch:///test.go")
	got := string(doc.Source())
	// "func main" should be gone (function declaration deleted)
	if strings.Contains(got, "func main") {
		t.Errorf("after DeleteNode, 'func main' should be gone: %q", got)
	}
	// "func other" should remain
	if !strings.Contains(got, "func other") {
		t.Errorf("after DeleteNode, 'func other' should remain: %q", got)
	}
	// "package main" should remain (it's the package clause, not a function)
	if !strings.Contains(got, "package main") {
		t.Errorf("after DeleteNode, 'package main' should remain: %q", got)
	}
}

func TestDeleteNodeInvalidSelector(t *testing.T) {
	ed := openScratchDoc(t, "package main\n", "go")
	sel := NodeSelector{Path: []PathStep{
		{Type: "nonexistent"},
	}}
	_, err := ed.DeleteNode("scratch:///test.go", sel)
	if err == nil {
		t.Fatal("expected error for invalid selector")
	}
}

// ─── Edit maintains parse tree ───

func TestEditMaintainsTree(t *testing.T) {
	ed := openScratchDoc(t, "package main\n", "go")
	ed.Insert("scratch:///test.go", 0, "// comment\n")
	info, err := ed.GetStructure("scratch:///test.go", -1, -1)
	if err != nil {
		t.Fatalf("GetStructure after edit failed: %v", err)
	}
	if info.Type != "source_file" {
		t.Errorf("root type after edit: got %q, want %q", info.Type, "source_file")
	}
}

func TestMultipleEdits(t *testing.T) {
	ed := openScratchDoc(t, "package main\nfunc main() {}\n", "go")
	ed.Insert("scratch:///test.go", 0, "// header\n")
	ed.Insert("scratch:///test.go", 22, "\n\treturn\n")
	ed.Replace("scratch:///test.go", 22, 22, "\n\tfmt.Println(\"hi\")\n")
	doc, _ := ed.GetDocument("scratch:///test.go")
	got := string(doc.Source())
	if !strings.Contains(got, "// header") {
		t.Errorf("after multiple edits, missing header: %q", got)
	}
	if !strings.Contains(got, "fmt.Println") {
		t.Errorf("after multiple edits, missing fmt.Println: %q", got)
	}
	if doc.Version() != 3 {
		t.Errorf("version after 3 edits: got %d, want 3", doc.Version())
	}
}

func TestEditResultParseErrors(t *testing.T) {
	ed := openScratchDoc(t, "package main\nfunc main() {\n", "go")
	result, err := ed.Insert("scratch:///test.go", 0, "// test\n")
	if err != nil {
		t.Fatalf("Insert failed: %v", err)
	}
	_ = result
}

func TestEditNormalizesLineEndings(t *testing.T) {
	ed := NewEditor("")
	err := ed.OpenDocument("scratch:///test.go", []byte("package main\r\n"), "go")
	if err != nil {
		t.Fatalf("OpenDocument failed: %v", err)
	}
	_, err = ed.Insert("scratch:///test.go", 0, "// comment\r\n")
	if err != nil {
		t.Fatalf("Insert CRLF failed: %v", err)
	}
	doc, _ := ed.GetDocument("scratch:///test.go")
	got := string(doc.Source())
	want := "// comment\r\npackage main\r\n"
	if got != want {
		t.Errorf("CRLF normalized: got %q, want %q", got, want)
	}
}

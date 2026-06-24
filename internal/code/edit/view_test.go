package edit

import (
	"testing"
)

func openScratchDoc(t *testing.T, source, lang string) *Editor {
	t.Helper()
	ed := NewEditor("")
	err := ed.OpenDocument("scratch:///test.go", []byte(source), lang)
	if err != nil {
		t.Fatalf("OpenDocument failed: %v", err)
	}
	return ed
}

// ─── GetStructure ───

func TestGetStructureRootType(t *testing.T) {
	ed := openScratchDoc(t, "package main\n", "go")
	info, err := ed.GetStructure("scratch:///test.go", -1, -1)
	if err != nil {
		t.Fatalf("GetStructure failed: %v", err)
	}
	if info.Type != "source_file" {
		t.Errorf("root type: got %q, want %q", info.Type, "source_file")
	}
	if info.StartByte != 0 {
		t.Errorf("startByte: got %d, want 0", info.StartByte)
	}
	if !info.IsNamed {
		t.Errorf("root should be named")
	}
}

func TestGetStructureByteRange(t *testing.T) {
	source := "package main\n"
	ed := openScratchDoc(t, source, "go")
	info, err := ed.GetStructure("scratch:///test.go", -1, -1)
	if err != nil {
		t.Fatalf("GetStructure failed: %v", err)
	}
	if info.EndByte != uint32(len(source)) {
		t.Errorf("endByte: got %d, want %d", info.EndByte, len(source))
	}
}

func TestGetStructureLeafText(t *testing.T) {
	source := "package main\n"
	ed := openScratchDoc(t, source, "go")
	info, err := ed.GetStructure("scratch:///test.go", -1, -1)
	if err != nil {
		t.Fatalf("GetStructure failed: %v", err)
	}
	// Root is a named node with children; at maxDepth=-1, children are populated
	if len(info.Children) == 0 {
		t.Fatal("expected children in source_file")
	}
	// First child should be package_clause
	child := info.Children[0]
	if child.Type != "package_clause" {
		t.Errorf("first child type: got %q, want %q", child.Type, "package_clause")
	}
	if !child.IsNamed {
		t.Errorf("package_clause should be named")
	}
}

func TestGetStructureMaxDepth(t *testing.T) {
	source := "package main\n\nfunc main() {\n\treturn\n}\n"
	ed := openScratchDoc(t, source, "go")
	// maxDepth=0: only root, no children
	info, err := ed.GetStructure("scratch:///test.go", 0, -1)
	if err != nil {
		t.Fatalf("GetStructure failed: %v", err)
	}
	if info.Type != "source_file" {
		t.Errorf("root: got %q, want %q", info.Type, "source_file")
	}
	if len(info.Children) != 0 {
		t.Errorf("maxDepth=0: expected 0 children, got %d", len(info.Children))
	}
	// Root text should be set at depth limit
	if info.Text == "" {
		t.Errorf("maxDepth=0: root text should be set at leaf level")
	}
}

func TestGetStructureMaxChildren(t *testing.T) {
	source := "package main\n\nfunc a() {}\nfunc b() {}\nfunc c() {}\nfunc d() {}\n"
	ed := openScratchDoc(t, source, "go")
	// maxChildren=2: only first 2 children
	info, err := ed.GetStructure("scratch:///test.go", -1, 2)
	if err != nil {
		t.Fatalf("GetStructure failed: %v", err)
	}
	// Root may have unnamed children (newlines), but should be capped at 2
	if len(info.Children) > 2 {
		t.Errorf("maxChildren=2: got %d children", len(info.Children))
	}
}

// ─── GetNode ───

func TestGetNodeByByte(t *testing.T) {
	source := "package main\n"
	ed := openScratchDoc(t, source, "go")
	info, err := ed.GetNode("scratch:///test.go", 0)
	if err != nil {
		t.Fatalf("GetNode failed: %v", err)
	}
	// At position 0, the smallest node might be the package keyword or source_file
	if info.Type == "" {
		t.Errorf("node type should not be empty")
	}
}

func TestGetNodeInPackageName(t *testing.T) {
	source := "package main\n"
	ed := openScratchDoc(t, source, "go")
	// Position 8 is inside "main" (package is 7 chars: "package")
	info, err := ed.GetNode("scratch:///test.go", 8)
	if err != nil {
		t.Fatalf("GetNode failed: %v", err)
	}
	// Should find an identifier or similar named node
	if info.Type == "" {
		t.Errorf("node type should not be empty")
	}
}

func TestGetNodeNotFound(t *testing.T) {
	source := "package main\n"
	ed := openScratchDoc(t, source, "go")
	// Position past end of source
	_, err := ed.GetNode("scratch:///test.go", 1000)
	if err != ErrNodeNotFound && err != ErrInvalidPosition {
		t.Errorf("expected error for out-of-range position, got %v", err)
	}
}

// ─── GetNodeAtPoint ───

func TestGetNodeAtPoint(t *testing.T) {
	source := "package main\n"
	ed := openScratchDoc(t, source, "go")
	info, err := ed.GetNodeAtPoint("scratch:///test.go", 0, 0)
	if err != nil {
		t.Fatalf("GetNodeAtPoint failed: %v", err)
	}
	if info.Type == "" {
		t.Errorf("node type should not be empty")
	}
}

func TestGetNodeAtPointMidLine(t *testing.T) {
	source := "package main\n"
	ed := openScratchDoc(t, source, "go")
	info, err := ed.GetNodeAtPoint("scratch:///test.go", 0, 8)
	if err != nil {
		t.Fatalf("GetNodeAtPoint failed: %v", err)
	}
	// At row 0, col 8, we're in "main" — should find identifier
	if info.Type == "" {
		t.Errorf("node type should not be empty")
	}
}

func TestGetNodeAtPointOutOfRange(t *testing.T) {
	source := "package main\n"
	ed := openScratchDoc(t, source, "go")
	_, err := ed.GetNodeAtPoint("scratch:///test.go", 999, 0)
	if err != ErrNodeNotFound {
		t.Errorf("expected ErrNodeNotFound, got %v", err)
	}
}

// ─── GetNodeAtRange ───

func TestGetNodeAtRange(t *testing.T) {
	source := "package main\n"
	ed := openScratchDoc(t, source, "go")
	info, err := ed.GetNodeAtRange("scratch:///test.go", 0, 1)
	if err != nil {
		t.Fatalf("GetNodeAtRange failed: %v", err)
	}
	if info.Type == "" {
		t.Errorf("node type should not be empty")
	}
}

// ─── GetDescendantsAt ───

func TestGetDescendantsAt(t *testing.T) {
	source := "package main\n"
	ed := openScratchDoc(t, source, "go")
	nodes, err := ed.GetDescendantsAt("scratch:///test.go", 0)
	if err != nil {
		t.Fatalf("GetDescendantsAt failed: %v", err)
	}
	if len(nodes) == 0 {
		t.Fatal("expected at least one ancestor node")
	}
	// The outermost node should be source_file
	if nodes[len(nodes)-1].Type != "source_file" {
		t.Errorf("outermost ancestor: got %q, want %q",
			nodes[len(nodes)-1].Type, "source_file")
	}
}

func TestGetDescendantsAtInnerToOuter(t *testing.T) {
	source := "package main\n"
	ed := openScratchDoc(t, source, "go")
	nodes, err := ed.GetDescendantsAt("scratch:///test.go", 8)
	if err != nil {
		t.Fatalf("GetDescendantsAt failed: %v", err)
	}
	if len(nodes) == 0 {
		t.Fatal("expected at least one node")
	}
	// First node should be the innermost (smallest range, most specific)
	// Last node should be the outermost (largest range, source_file)
	inner := nodes[0]
	outer := nodes[len(nodes)-1]
	// Inner must have StartByte >= outer StartByte (inner is a descendant)
	if inner.StartByte < outer.StartByte {
		t.Errorf("inner starts before outer: inner.StartByte=%d < outer.StartByte=%d",
			inner.StartByte, outer.StartByte)
	}
	// Inner must have EndByte <= outer EndByte
	if inner.EndByte > outer.EndByte {
		t.Errorf("inner ends after outer: inner.EndByte=%d > outer.EndByte=%d",
			inner.EndByte, outer.EndByte)
	}
	// Outermost must be source_file
	if outer.Type != "source_file" {
		t.Errorf("outermost: got %q, want %q", outer.Type, "source_file")
	}
}

func TestGetDescendantsAtNotFound(t *testing.T) {
	source := "package main\n"
	ed := openScratchDoc(t, source, "go")
	_, err := ed.GetDescendantsAt("scratch:///test.go", 1000)
	if err != ErrNodeNotFound && err != ErrInvalidPosition {
		t.Errorf("expected error for out-of-range, got %v", err)
	}
}

// ─── Query ───

func TestQueryFunctionNames(t *testing.T) {
	source := "package main\n\nfunc hello() {}\nfunc world() {}\n"
	ed := openScratchDoc(t, source, "go")
	results, err := ed.Query("scratch:///test.go",
		"(function_declaration name: (identifier) @fn)")
	if err != nil {
		t.Fatalf("Query failed: %v", err)
	}
	if len(results) == 0 {
		t.Fatal("expected query matches")
	}
	// Check captures
	fns := results[0].Captures["fn"]
	if len(fns) == 0 {
		t.Fatal("expected fn captures")
	}
}

func TestQueryNoMatch(t *testing.T) {
	source := "package main\n"
	ed := openScratchDoc(t, source, "go")
	// Query for something that doesn't exist
	results, err := ed.Query("scratch:///test.go",
		"(function_declaration name: (identifier) @fn)")
	if err != nil {
		t.Fatalf("Query failed: %v", err)
	}
	if len(results) != 0 {
		t.Errorf("expected 0 matches, got %d", len(results))
	}
}

func TestQueryInvalidPattern(t *testing.T) {
	source := "package main\n"
	ed := openScratchDoc(t, source, "go")
	_, err := ed.Query("scratch:///test.go", "(invalid !!! pattern")
	if err == nil {
		t.Fatal("expected error for invalid query pattern")
	}
}

// ─── GetText ───

func TestGetText(t *testing.T) {
	source := "package main\n"
	ed := openScratchDoc(t, source, "go")
	text, err := ed.GetText("scratch:///test.go", 0, uint32(len(source)))
	if err != nil {
		t.Fatalf("GetText failed: %v", err)
	}
	if text != source {
		t.Errorf("GetText: got %q, want %q", text, source)
	}
}

func TestGetTextSubstring(t *testing.T) {
	source := "package main\n"
	ed := openScratchDoc(t, source, "go")
	text, err := ed.GetText("scratch:///test.go", 0, 7)
	if err != nil {
		t.Fatalf("GetText failed: %v", err)
	}
	if text != "package" {
		t.Errorf("GetText(0,7): got %q, want %q", text, "package")
	}
}

func TestGetTextEmptyRange(t *testing.T) {
	source := "package main\n"
	ed := openScratchDoc(t, source, "go")
	text, err := ed.GetText("scratch:///test.go", 0, 0)
	if err != nil {
		t.Fatalf("GetText empty range failed: %v", err)
	}
	if text != "" {
		t.Errorf("empty range: got %q, want empty", text)
	}
}

func TestGetTextInvalidRange(t *testing.T) {
	source := "package main\n"
	ed := openScratchDoc(t, source, "go")
	_, err := ed.GetText("scratch:///test.go", 10, 5)
	if err != ErrInvalidPosition {
		t.Errorf("start > end: expected ErrInvalidPosition, got %v", err)
	}
}

func TestGetTextPastEnd(t *testing.T) {
	source := "package main\n"
	ed := openScratchDoc(t, source, "go")
	_, err := ed.GetText("scratch:///test.go", 0, 100)
	if err != ErrInvalidPosition {
		t.Errorf("past end: expected ErrInvalidPosition, got %v", err)
	}
}

// ─── GetLine ───

func TestGetLineFirst(t *testing.T) {
	source := "line one\nline two\nline three\n"
	ed := openScratchDoc(t, source, "go")
	line, err := ed.GetLine("scratch:///test.go", 0)
	if err != nil {
		t.Fatalf("GetLine failed: %v", err)
	}
	if line != "line one" {
		t.Errorf("line 0: got %q, want %q", line, "line one")
	}
}

func TestGetLineSecond(t *testing.T) {
	source := "line one\nline two\nline three\n"
	ed := openScratchDoc(t, source, "go")
	line, err := ed.GetLine("scratch:///test.go", 1)
	if err != nil {
		t.Fatalf("GetLine failed: %v", err)
	}
	if line != "line two" {
		t.Errorf("line 1: got %q, want %q", line, "line two")
	}
}

func TestGetLineLastNoTrailingNewline(t *testing.T) {
	source := "line one\nline two"
	ed := openScratchDoc(t, source, "go")
	line, err := ed.GetLine("scratch:///test.go", 1)
	if err != nil {
		t.Fatalf("GetLine last line failed: %v", err)
	}
	if line != "line two" {
		t.Errorf("last line: got %q, want %q", line, "line two")
	}
}

func TestGetLineEmptyFile(t *testing.T) {
	ed := openScratchDoc(t, "", "go")
	line, err := ed.GetLine("scratch:///test.go", 0)
	if err != nil {
		t.Fatalf("GetLine empty file failed: %v", err)
	}
	if line != "" {
		t.Errorf("empty file line 0: got %q, want empty", line)
	}
}

func TestGetLineOutOfRange(t *testing.T) {
	source := "line one\n"
	ed := openScratchDoc(t, source, "go")
	_, err := ed.GetLine("scratch:///test.go", 999)
	if err != ErrInvalidPosition {
		t.Errorf("out of range: expected ErrInvalidPosition, got %v", err)
	}
}

func TestGetLineCRLF(t *testing.T) {
	source := "line one\r\nline two\r\n"
	ed := openScratchDoc(t, source, "go")
	line, err := ed.GetLine("scratch:///test.go", 0)
	if err != nil {
		t.Fatalf("GetLine CRLF failed: %v", err)
	}
	if line != "line one" {
		t.Errorf("CRLF line 0: got %q, want %q", line, "line one")
	}
	line, err = ed.GetLine("scratch:///test.go", 1)
	if err != nil {
		t.Fatalf("GetLine CRLF line 1 failed: %v", err)
	}
	if line != "line two" {
		t.Errorf("CRLF line 1: got %q, want %q", line, "line two")
	}
}

func TestGetLineStripNewline(t *testing.T) {
	source := "hello\nworld\n"
	ed := openScratchDoc(t, source, "go")
	line, err := ed.GetLine("scratch:///test.go", 0)
	if err != nil {
		t.Fatalf("GetLine failed: %v", err)
	}
	// Should not include trailing \n
	if line != "hello" {
		t.Errorf("line 0 without newline: got %q, want %q", line, "hello")
	}
}

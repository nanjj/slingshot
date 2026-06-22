package editor

import (
	"testing"
)

func TestResolveNodeByPos(t *testing.T) {
	ed := openScratchDoc(t, "package main\n\nfunc main() {}\n", "go")
	doc, _ := ed.GetDocument("scratch:///test.go")
	pos := uint32(0)
	sel := NodeSelector{Pos: &pos}
	node, err := doc.resolveNode(sel)
	if err != nil {
		t.Fatalf("resolveNode by Pos failed: %v", err)
	}
	if node == nil {
		t.Fatal("resolveNode returned nil")
	}
}

func TestResolveNodeByPoint(t *testing.T) {
	ed := openScratchDoc(t, "package main\n\nfunc main() {}\n", "go")
	doc, _ := ed.GetDocument("scratch:///test.go")
	point := [2]uint32{0, 0}
	sel := NodeSelector{Point: &point}
	node, err := doc.resolveNode(sel)
	if err != nil {
		t.Fatalf("resolveNode by Point failed: %v", err)
	}
	if node == nil {
		t.Fatal("resolveNode returned nil")
	}
}

func TestResolveNodeByRange(t *testing.T) {
	ed := openScratchDoc(t, "package main\n\nfunc main() {}\n", "go")
	doc, _ := ed.GetDocument("scratch:///test.go")
	rng := [2]uint32{0, 1}
	sel := NodeSelector{Range: &rng}
	node, err := doc.resolveNode(sel)
	if err != nil {
		t.Fatalf("resolveNode by Range failed: %v", err)
	}
	if node == nil {
		t.Fatal("resolveNode returned nil")
	}
}

func TestResolveNodeByPath(t *testing.T) {
	ed := openScratchDoc(t, "package main\n\nfunc main() {}\n", "go")
	doc, _ := ed.GetDocument("scratch:///test.go")
	sel := NodeSelector{Path: []PathStep{
		{Type: "function_declaration", NamedOnly: true},
	}}
	node, err := doc.resolveNode(sel)
	if err != nil {
		t.Fatalf("resolveNode by Path failed: %v", err)
	}
	if node == nil {
		t.Fatal("resolveNode returned nil")
	}
	if node.Type(doc.language) != "function_declaration" {
		t.Errorf("resolved node type: got %q, want %q",
			node.Type(doc.language), "function_declaration")
	}
}

func TestResolveNodeEmptySelector(t *testing.T) {
	ed := openScratchDoc(t, "package main\n", "go")
	doc, _ := ed.GetDocument("scratch:///test.go")
	sel := NodeSelector{}
	_, err := doc.resolveNode(sel)
	if err == nil {
		t.Fatal("expected error for empty selector")
	}
}

func TestResolveNodeNonexistentPos(t *testing.T) {
	ed := openScratchDoc(t, "package main\n", "go")
	doc, _ := ed.GetDocument("scratch:///test.go")
	pos := uint32(1000)
	sel := NodeSelector{Pos: &pos}
	_, err := doc.resolveNode(sel)
	if err != ErrNodeNotFound {
		t.Errorf("expected ErrNodeNotFound, got %v", err)
	}
}

func TestResolvePathDeep(t *testing.T) {
	ed := openScratchDoc(t, "package main\n\nfunc main() {\n\treturn\n}\n", "go")
	doc, _ := ed.GetDocument("scratch:///test.go")
	// Go grammar wraps statements in statement_list inside block
	sel := NodeSelector{Path: []PathStep{
		{Type: "function_declaration", NamedOnly: true},
		{Type: "block", NamedOnly: true},
		{Type: "statement_list", NamedOnly: true},
		{Type: "return_statement", NamedOnly: true},
	}}
	node, err := doc.resolveNode(sel)
	if err != nil {
		t.Fatalf("resolvePath deep failed: %v", err)
	}
	if node == nil {
		t.Fatal("resolvePath returned nil")
	}
	if node.Type(doc.language) != "return_statement" {
		t.Errorf("deep path type: got %q, want %q",
			node.Type(doc.language), "return_statement")
	}
}

func TestResolvePathNotFound(t *testing.T) {
	ed := openScratchDoc(t, "package main\n", "go")
	doc, _ := ed.GetDocument("scratch:///test.go")
	sel := NodeSelector{Path: []PathStep{
		{Type: "nonexistent_type"},
	}}
	_, err := doc.resolveNode(sel)
	if err == nil {
		t.Fatal("expected error for nonexistent path")
	}
}

func TestResolvePathWithFieldName(t *testing.T) {
	ed := openScratchDoc(t, "package main\n\nfunc main() {\n}\n", "go")
	doc, _ := ed.GetDocument("scratch:///test.go")
	// Navigate to function declaration, then to body (block) via field name
	sel := NodeSelector{Path: []PathStep{
		{Type: "function_declaration", NamedOnly: true},
		{Field: "body"},
	}}
	node, err := doc.resolveNode(sel)
	if err != nil {
		t.Fatalf("resolvePath with field failed: %v", err)
	}
	if node == nil {
		t.Fatal("resolvePath returned nil")
	}
	// The body field should be a block node
	typ := node.Type(doc.language)
	if typ == "" {
		t.Errorf("resolved node type is empty")
	}
}

func TestResolvePathWithChildIndex(t *testing.T) {
	ed := openScratchDoc(t, "package main\n\nfunc a() {}\nfunc b() {}\nfunc c() {}\n", "go")
	doc, _ := ed.GetDocument("scratch:///test.go")
	// ChildIndex counts across ALL named children of source_file:
	//   NamedChild[0]=package_clause, [1]=function_declaration(a), [2]=function_declaration(b), [3]=function_declaration(c)
	// ChildIndex=2 → NamedChild[2] → function_declaration "func b"
	sel := NodeSelector{Path: []PathStep{
		{Type: "function_declaration", NamedOnly: true, ChildIndex: 2},
	}}
	node, err := doc.resolveNode(sel)
	if err != nil {
		t.Fatalf("resolvePath with ChildIndex failed: %v", err)
	}
	if node == nil {
		t.Fatal("resolvePath returned nil")
	}
	text := node.Text(doc.source)
	if !contains(text, "func b") {
		t.Errorf("expected 'func b', got %q", text)
	}
}

// Helper function: substring check without importing strings
func contains(s, substr string) bool {
	if len(substr) == 0 {
		return true
	}
	if len(substr) > len(s) {
		return false
	}
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

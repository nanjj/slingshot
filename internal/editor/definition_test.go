package editor

import (
	"testing"
)

func TestGetDefs_Go(t *testing.T) {
	ed := NewEditor("")
	src := []byte(`package main

import "fmt"

type Point struct {
	X, Y int
}

func (p Point) String() string {
	return fmt.Sprintf("(%d,%d)", p.X, p.Y)
}

func main() {
	var p Point
	p.String()
}
`)

	err := ed.OpenDocument("scratch:///test.go", src, "go")
	if err != nil {
		t.Fatalf("OpenDocument: %v", err)
	}

	tags, err := ed.GetDefs("scratch:///test.go")
	if err != nil {
		t.Fatalf("GetDefs: %v", err)
	}

	if len(tags) == 0 {
		t.Fatal("expected at least one definition tag, got none")
	}

	found := map[string]bool{}
	for _, tag := range tags {
		found[tag.Name] = true
		t.Logf("  tag: kind=%s name=%s bytes=[%d,%d)", tag.Kind, tag.Name, tag.StartByte, tag.EndByte)
	}
	// Go's inferred tags query covers functions and methods but not types.
	// "Point" won't appear because the Go LangEntry has no explicit TagsQuery.
	names := []string{"String", "main"}
	for _, name := range names {
		if !found[name] {
			t.Errorf("expected definition %q not found", name)
		}
	}
}

func TestGetDefs_Python(t *testing.T) {
	ed := NewEditor("")
	src := []byte(`import os
import sys

class Greeter:
    """A simple greeter."""

    def __init__(self, name):
        self.name = name

    def greet(self):
        print(f"Hello, {self.name}!")

def create_greeter(name):
    return Greeter(name)
`)

	err := ed.OpenDocument("scratch:///test.py", src, "python")
	if err != nil {
		t.Fatalf("OpenDocument: %v", err)
	}

	tags, err := ed.GetDefs("scratch:///test.py")
	if err != nil {
		t.Fatalf("GetDefs: %v", err)
	}

	if len(tags) == 0 {
		t.Fatal("expected at least one definition tag, got none")
	}

	found := map[string]string{}
	for _, tag := range tags {
		found[tag.Name] = tag.Kind
		t.Logf("  tag: kind=%s name=%s", tag.Kind, tag.Name)
	}

	// Python tags query tags all functions/methods as "definition.function"
	// because Python grammar doesn't distinguish methods from functions.
	expect := map[string]string{
		"Greeter":        "definition.class",
		"__init__":       "definition.function",
		"greet":          "definition.function",
		"create_greeter": "definition.function",
	}
	for name, kind := range expect {
		actual, ok := found[name]
		if !ok {
			t.Errorf("expected definition %q not found", name)
			continue
		}
		if actual != kind {
			t.Errorf("definition %q: expected kind %q, got %q", name, kind, actual)
		}
	}
}

func TestGetDefs_JavaScript(t *testing.T) {
	ed := NewEditor("")
	src := []byte(`class Counter {
    constructor(initial) {
        this.count = initial;
    }

    increment() {
        this.count++;
    }
}

function formatCount(counter) {
    return counter.count.toString();
}
`)

	err := ed.OpenDocument("scratch:///test.js", src, "javascript")
	if err != nil {
		t.Fatalf("OpenDocument: %v", err)
	}

	tags, err := ed.GetDefs("scratch:///test.js")
	if err != nil {
		t.Fatalf("GetDefs: %v", err)
	}

	if len(tags) == 0 {
		t.Fatal("expected at least one definition tag, got none")
	}

	found := map[string]string{}
	for _, tag := range tags {
		found[tag.Name] = tag.Kind
		t.Logf("  tag: kind=%s name=%s", tag.Kind, tag.Name)
	}

	expect := map[string]string{
		"Counter":     "definition.class",
		"constructor": "definition.method",
		"increment":   "definition.method",
		"formatCount": "definition.function",
	}
	for name, kind := range expect {
		actual, ok := found[name]
		if !ok {
			t.Errorf("expected definition %q not found", name)
			continue
		}
		if actual != kind {
			t.Errorf("definition %q: expected kind %q, got %q", name, kind, actual)
		}
	}
}

func TestGetDefs_Rust(t *testing.T) {
	ed := NewEditor("")
	src := []byte(`struct Point {
    x: i32,
    y: i32,
}

impl Point {
    fn new(x: i32, y: i32) -> Self {
        Point { x, y }
    }
}

fn main() {
    let _p = Point::new(1, 2);
}
`)

	err := ed.OpenDocument("scratch:///test.rs", src, "rust")
	if err != nil {
		t.Fatalf("OpenDocument: %v", err)
	}

	tags, err := ed.GetDefs("scratch:///test.rs")
	if err != nil {
		t.Fatalf("GetDefs: %v", err)
	}

	if len(tags) == 0 {
		t.Fatal("expected at least one definition tag, got none")
	}

	found := map[string]string{}
	for _, tag := range tags {
		found[tag.Name] = tag.Kind
		t.Logf("  tag: kind=%s name=%s", tag.Kind, tag.Name)
	}

	expect := []string{"Point", "new", "main"}
	for _, name := range expect {
		if _, ok := found[name]; !ok {
			t.Errorf("expected definition %q not found", name)
		}
	}
}

func TestGetDefs_EmptyFile(t *testing.T) {
	ed := NewEditor("")
	src := []byte{}

	err := ed.OpenDocument("scratch:///empty.go", src, "go")
	if err != nil {
		t.Fatalf("OpenDocument: %v", err)
	}

	tags, err := ed.GetDefs("scratch:///empty.go")
	if err != nil {
		t.Fatalf("GetDefs: %v", err)
	}

	if len(tags) != 0 {
		t.Errorf("expected 0 tags for empty file, got %d", len(tags))
	}
}

func TestGetDefs_UnsupportedLanguage(t *testing.T) {
	ed := NewEditor("")
	src := []byte("some text")

	err := ed.OpenDocument("scratch:///test.xyz", src, "")
	if err == nil {
		t.Fatal("expected error for unsupported language")
	}
}

func TestLocateDef_Go(t *testing.T) {
	ed := NewEditor("")
	src := []byte(`package main

func foo() {}
func bar() {}
func baz() {}
`)

	err := ed.OpenDocument("scratch:///test.go", src, "go")
	if err != nil {
		t.Fatalf("OpenDocument: %v", err)
	}

	tag, err := ed.LocateDef("scratch:///test.go", "function:bar")
	if err != nil {
		t.Fatalf("LocateDef: %v", err)
	}

	if tag.Name != "bar" {
		t.Errorf("expected name 'bar', got %q", tag.Name)
	}
	if tag.Kind != "definition.function" {
		t.Errorf("expected kind 'definition.function', got %q", tag.Kind)
	}
}

func TestLocateDef_NotFound(t *testing.T) {
	ed := NewEditor("")
	src := []byte(`package main
func foo() {}`)

	err := ed.OpenDocument("scratch:///test.go", src, "go")
	if err != nil {
		t.Fatalf("OpenDocument: %v", err)
	}

	_, err = ed.LocateDef("scratch:///test.go", "function:nonexistent")
	if err == nil {
		t.Fatal("expected error for nonexistent definition")
	}
}

func TestLocateDef_LinesSelector(t *testing.T) {
	ed := NewEditor("")
	src := []byte(`package main

func foo() {}
`)

	err := ed.OpenDocument("scratch:///test.go", src, "go")
	if err != nil {
		t.Fatalf("OpenDocument: %v", err)
	}

	_, err = ed.LocateDef("scratch:///test.go", "lines:2-4")
	if err == nil {
		t.Fatal("expected error for lines selector")
	}
}

func TestParseSelector(t *testing.T) {
	tests := []struct {
		input    string
		wantKind string
		wantName string
		wantErr  bool
	}{
		{"function:main", "function", "main", false},
		{"class:User", "class", "User", false},
		{"method:GetName", "method", "GetName", false},
		{"struct:Point", "struct", "Point", false},
		{"interface:Reader", "interface", "Reader", false},
		{"lines:10-20", "lines", "10-20", false},
		{"invalid", "", "", true},
		{":empty", "", "", true},
		{"empty:", "", "", true},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			kind, name, err := parseSelector(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Errorf("expected error, got kind=%q name=%q", kind, name)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if kind != tt.wantKind {
				t.Errorf("kind: got %q, want %q", kind, tt.wantKind)
			}
			if name != tt.wantName {
				t.Errorf("name: got %q, want %q", name, tt.wantName)
			}
		})
	}
}

func TestGetStructureSummary_Go(t *testing.T) {
	ed := NewEditor("")
	src := []byte(`package main

type Point struct {
	X, Y int
}

type Reader interface {
	Read([]byte) (int, error)
}

func (p Point) String() string { return "" }

func main() {}
`)

	err := ed.OpenDocument("scratch:///test.go", src, "go")
	if err != nil {
		t.Fatalf("OpenDocument: %v", err)
	}

	summary, err := ed.GetStructureSummary("scratch:///test.go")
	if err != nil {
		t.Fatalf("GetStructureSummary: %v", err)
	}

	t.Logf("language: %s", summary.Language)
	t.Logf("functions: %d", len(summary.Functions))
	t.Logf("methods: %d", len(summary.Methods))
	t.Logf("structs: %d", len(summary.Structs))
	t.Logf("interfaces: %d", len(summary.Interfaces))

	if len(summary.Functions) < 1 {
		t.Error("expected at least 1 function")
	}
	if summary.Language == "" {
		t.Error("expected non-empty language")
	}
}

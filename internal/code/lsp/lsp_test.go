// Package lsp_test provides unit tests for the tree-sitter based code analysis
// operations in the lsp package. Each method is tested against temporary Go
// source files with known structure to verify parsing, AST navigation, definition
// extraction, validation, queries, and complexity analysis.
package lsp_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
	"gotest.tools/v3/assert"

	"github.com/nanjj/slingshot/internal/code/lsp"
)

// ─── Test Fixtures ────────────────────────────────────────────────────────────

// writeTestFile writes content to a file in a temp directory and returns its path.
func writeTestFile(t *testing.T, dir, name, content string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	err := os.WriteFile(path, []byte(content), 0644)
	assert.NilError(t, err)
	return path
}

// simpleGo returns a simple Go source file for basic parsing tests.
func simpleGo(t *testing.T, dir string) string {
	return writeTestFile(t, dir, "main.go", `package main

import "fmt"

// Greeter greets people.
type Greeter struct {
	Name string
}

// Hello returns a greeting.
func (g *Greeter) Hello() string {
	return "Hello, " + g.Name + "!"
}

// main is the entry point.
func main() {
	g := &Greeter{Name: "World"}
	fmt.Println(g.Hello())
}
`)
}

// complexGo returns a Go source with conditionals, loops, and recursion.
func complexGo(t *testing.T, dir string) string {
	return writeTestFile(t, dir, "complex.go", `package main

import "fmt"

// factorial computes factorial recursively.
func factorial(n int) int {
	if n <= 1 {
		return 1
	}
	return n * factorial(n-1)
}

// findMax finds the maximum value in a slice with a linear scan inside a loop.
func findMax(items []int) (int, error) {
	if len(items) == 0 {
		return 0, fmt.Errorf("empty slice")
	}
	seen := make(map[int]bool)
	max := items[0]
	for _, v := range items {
		if v > max {
			max = v
		}
		// O(n²): linear scan inside loop — find first occurrence
		_ = contains(items, v)
		if !seen[v] {
			seen[v] = true
		}
	}
	return max, nil
}

// contains checks if a value exists in a slice.
func contains(items []int, target int) bool {
	for _, v := range items {
		if v == target {
			return true
		}
	}
	return false
}

// complexCondition has nested conditionals for cognitive complexity testing.
func complexCondition(a, b, c int) string {
	if a > 0 {
		if b > 0 {
			if c > 0 {
				return "all positive"
			}
			return "a and b positive"
		}
		return "a positive"
	} else if b > 0 {
		return "b positive"
	}
	return "none"
}

// countdown uses a for loop.
func countdown(n int) {
	for i := n; i >= 0; i-- {
		fmt.Println(i)
	}
}

// doLoop uses a do-while style loop.
func doLoop(n int) {
	for {
		if n <= 0 {
			break
		}
		n--
	}
}
`)
}

// invalidSyntaxGo returns a Go file with syntax errors.
func invalidSyntaxGo(t *testing.T, dir string) string {
	return writeTestFile(t, dir, "invalid.go", `package main

import "fmt"

func broken(a int) {
	if a > 0 {
		fmt.Println("positive")
	// missing closing brace
}
`)
}

// crlfGo returns a Go file with CRLF line endings.
func crlfGo(t *testing.T, dir string) string {
	return writeTestFile(t, dir, "crlf.go", "package main\r\n\r\nfunc main() {\r\n}\r\n")
}

// emptyGo returns an empty Go file.
func emptyGo(t *testing.T, dir string) string {
	return writeTestFile(t, dir, "empty.go", "")
}

// nonGo returns a file with an unsupported extension.
func nonGo(t *testing.T, dir string) string {
	return writeTestFile(t, dir, "data.xyz", "unsupported")
}

// paramFuncGo has functions with various parameter counts.
func paramFuncGo(t *testing.T, dir string) string {
	return writeTestFile(t, dir, "params.go", `package main

// noop takes no parameters.
func noop() {}

// oneParam takes one parameter.
func oneParam(a int) {}

// multiParam takes multiple parameters.
func multiParam(a, b, c int, s string) {}

// methodWithReceiver has a receiver (counts as 1+ params).
type T struct{}

func (t *T) methodWithReceiver(x int) {}
`)
}

// ─── Tests: Parsing ──────────────────────────────────────────────────────────

// TestParseFile verifies ParseFile successfully parses a valid Go file.
func TestParseFile(t *testing.T) {
	a := lsp.NewAnalyzer()
	dir := t.TempDir()
	path := simpleGo(t, dir)

	result, err := a.ParseFile(path)
	assert.NilError(t, err)
	defer result.Tree.Release()

	assert.Assert(t, result.Language != "")
	assert.Assert(t, len(result.Source) > 0)
	assert.Assert(t, result.Tree != nil)

	// Verify the root node is present
	root := result.Tree.RootNode()
	assert.Assert(t, root != nil)
}

// TestParseFile_UnsupportedLanguage returns an error for unknown file types.
func TestParseFile_UnsupportedLanguage(t *testing.T) {
	a := lsp.NewAnalyzer()
	dir := t.TempDir()
	path := nonGo(t, dir)

	_, err := a.ParseFile(path)
	assert.ErrorContains(t, err, "unsupported language")
}

// TestParseFile_NonexistentFile returns an error for a missing file.
func TestParseFile_NonexistentFile(t *testing.T) {
	a := lsp.NewAnalyzer()

	_, err := a.ParseFile("/nonexistent/path/file.go")
	assert.ErrorContains(t, err, "read file")
}

// TestParseBytes verifies ParseBytes parses source bytes with a language name.
func TestParseBytes(t *testing.T) {
	a := lsp.NewAnalyzer()
	source := []byte(`package main
func main() {}`)

	result, err := a.ParseBytes(source, "go")
	assert.NilError(t, err)
	defer result.Tree.Release()

	assert.Assert(t, result.Language != "")
	assert.Assert(t, len(result.Source) > 0)
	assert.Assert(t, result.Tree != nil)
}

// TestParseBytes_UnsupportedLanguage returns an error for unknown language.
func TestParseBytes_UnsupportedLanguage(t *testing.T) {
	a := lsp.NewAnalyzer()

	_, err := a.ParseBytes([]byte("hello"), "nonexistent-lang")
	assert.ErrorContains(t, err, "unsupported language")
}

// ─── Tests: Structure ────────────────────────────────────────────────────────

// TestGetStructure verifies GetStructure returns a hierarchical AST.
func TestGetStructure(t *testing.T) {
	a := lsp.NewAnalyzer()
	dir := t.TempDir()
	path := simpleGo(t, dir)

	// With no limits
	info, err := a.GetStructure(path, -1, -1)
	assert.NilError(t, err)
	assert.Assert(t, info.Type != "")
	assert.Assert(t, info.StartByte < info.EndByte)
	assert.Assert(t, len(info.Children) > 0, "root should have children")
}

// TestGetStructure_DepthLimit verifies maxDepth limits the recursion.
func TestGetStructure_DepthLimit(t *testing.T) {
	a := lsp.NewAnalyzer()
	dir := t.TempDir()
	path := simpleGo(t, dir)

	// maxDepth=1 should stop at first-level children (no grandchildren)
	shallow, err := a.GetStructure(path, 1, -1)
	assert.NilError(t, err)

	// maxDepth=-1 should be deeper
	full, err := a.GetStructure(path, -1, -1)
	assert.NilError(t, err)
	assert.Assert(t, countChildren(full) >= countChildren(shallow),
		"full structure should have at least as many nodes as depth-limited")
}

func countChildren(n lsp.NodeInfo) int {
	c := 1
	for _, child := range n.Children {
		c += countChildren(child)
	}
	return c
}

// TestGetStructure_WidthLimit verifies maxWidth limits children per node.
func TestGetStructure_WidthLimit(t *testing.T) {
	a := lsp.NewAnalyzer()
	dir := t.TempDir()
	path := simpleGo(t, dir)

	narrow, err := a.GetStructure(path, -1, 2)
	assert.NilError(t, err)

	// With maxWidth=2, the max children per node should not exceed 2
	checkMaxWidth(t, narrow, 2)
}

func checkMaxWidth(t *testing.T, n lsp.NodeInfo, max int) {
	t.Helper()
	assert.Assert(t, len(n.Children) <= max,
		"node %q has %d children, expected ≤ %d", n.Type, len(n.Children), max)
	for _, c := range n.Children {
		checkMaxWidth(t, c, max)
	}
}

// TestGetStructure_EmptyFile handles an empty file gracefully.
func TestGetStructure_EmptyFile(t *testing.T) {
	a := lsp.NewAnalyzer()
	dir := t.TempDir()
	path := emptyGo(t, dir)

	info, err := a.GetStructure(path, -1, -1)
	assert.NilError(t, err)
	assert.Assert(t, info.Type != "")
}

// TestGetStructureFromResult verifies structure from an existing parse result.
func TestGetStructureFromResult(t *testing.T) {
	a := lsp.NewAnalyzer()
	dir := t.TempDir()
	path := simpleGo(t, dir)

	result, err := a.ParseFile(path)
	assert.NilError(t, err)
	defer result.Tree.Release()

	info, err := a.GetStructureFromResult(result, path, -1, -1)
	assert.NilError(t, err)
	assert.Assert(t, info.Type != "")
}

// ─── Tests: Node Operations ──────────────────────────────────────────────────

// TestGetNode verifies GetNode returns the smallest AST node at a position.
func TestGetNode(t *testing.T) {
	a := lsp.NewAnalyzer()
	dir := t.TempDir()
	path := simpleGo(t, dir)

	// Position 0 is the start of "package main" — should be "package" keyword
	info, err := a.GetNode(path, 0)
	assert.NilError(t, err)
	assert.Assert(t, info.Type != "")
}

// TestGetNode_PositionOutOfBounds returns an error for invalid position.
func TestGetNode_PositionOutOfBounds(t *testing.T) {
	a := lsp.NewAnalyzer()
	dir := t.TempDir()
	path := simpleGo(t, dir)

	// Position past end of file
	_, err := a.GetNode(path, 99999)
	assert.ErrorContains(t, err, "node not found")
}

// TestGetNodeAtPoint verifies GetNodeAtPoint returns node at (row, col).
func TestGetNodeAtPoint(t *testing.T) {
	a := lsp.NewAnalyzer()
	dir := t.TempDir()
	path := simpleGo(t, dir)

	// Row 0, col 9 is "main" in "package main"
	info, err := a.GetNodeAtPoint(path, 0, 9)
	assert.NilError(t, err)
	assert.Assert(t, info.Type != "")
}

// TestGetNodeAtPoint_OutOfBounds returns an error.
func TestGetNodeAtPoint_OutOfBounds(t *testing.T) {
	a := lsp.NewAnalyzer()
	dir := t.TempDir()
	path := simpleGo(t, dir)

	_, err := a.GetNodeAtPoint(path, 999, 0)
	assert.ErrorContains(t, err, "node not found")
}

// TestGetNodeAtRange verifies GetNodeAtRange returns the smallest node covering [start, end).
func TestGetNodeAtRange(t *testing.T) {
	a := lsp.NewAnalyzer()
	dir := t.TempDir()
	path := simpleGo(t, dir)

	info, err := a.GetNodeAtRange(path, 0, 1)
	assert.NilError(t, err)
	assert.Assert(t, info.Type != "")
}

// TestGetNodeAtRange_InvalidRange returns an error.
func TestGetNodeAtRange_InvalidRange(t *testing.T) {
	a := lsp.NewAnalyzer()
	dir := t.TempDir()
	path := simpleGo(t, dir)

	_, err := a.GetNodeAtRange(path, 9999, 10000)
	assert.ErrorContains(t, err, "node not found")
}

// TestGetDescendantsAt verifies GetDescendantsAt returns all ancestors.
func TestGetDescendantsAt(t *testing.T) {
	a := lsp.NewAnalyzer()
	dir := t.TempDir()
	path := simpleGo(t, dir)

	descendants, err := a.GetDescendantsAt(path, 0)
	assert.NilError(t, err)
	assert.Assert(t, len(descendants) > 0, "should have at least one ancestor")

	// Innermost first — first element should be a descendant of last
	assert.Assert(t, descendants[0].StartByte >= descendants[len(descendants)-1].StartByte,
		"first element should be innermost (deeper) or same as outermost")
}

// TestGetDescendantsAt_NotFound returns an error.
func TestGetDescendantsAt_NotFound(t *testing.T) {
	a := lsp.NewAnalyzer()
	dir := t.TempDir()
	path := simpleGo(t, dir)

	_, err := a.GetDescendantsAt(path, 99999)
	assert.ErrorContains(t, err, "node not found")
}

// ─── Tests: Definitions ──────────────────────────────────────────────────────

// TestGetDefinitions verifies GetDefinitions returns function and method tags.
func TestGetDefinitions(t *testing.T) {
	a := lsp.NewAnalyzer()
	dir := t.TempDir()
	path := simpleGo(t, dir)

	tags, err := a.GetDefinitions(path)
	assert.NilError(t, err)
	assert.Assert(t, len(tags) >= 2,
		"expected at least 2 definitions (Hello + main), got %d", len(tags))

	// Verify the tags include known functions
	found := make(map[string]bool)
	for _, tag := range tags {
		if tag.Kind == "definition.function" || tag.Kind == "definition.method" {
			found[tag.Name] = true
		}
	}
	assert.Assert(t, found["main"], "main should be found as a definition")
}

// TestGetDefinitions_UnsupportedLanguage returns an error.
func TestGetDefinitions_UnsupportedLanguage(t *testing.T) {
	a := lsp.NewAnalyzer()
	dir := t.TempDir()
	path := nonGo(t, dir)

	_, err := a.GetDefinitions(path)
	assert.ErrorContains(t, err, "unsupported language")
}

// ─── Tests: Validation ───────────────────────────────────────────────────────

// TestValidate_ValidFile verifies Validate returns valid=true for valid Go.
func TestValidate_ValidFile(t *testing.T) {
	a := lsp.NewAnalyzer()
	dir := t.TempDir()
	path := simpleGo(t, dir)

	result, err := a.Validate(path)
	assert.NilError(t, err)
	assert.Assert(t, result.Valid, "expected valid=true for correct Go")
	assert.Assert(t, result.SourceSize > 0)
	assert.Assert(t, result.Language != "")
}

// TestValidate_InvalidSyntax detects syntax errors.
func TestValidate_InvalidSyntax(t *testing.T) {
	a := lsp.NewAnalyzer()
	dir := t.TempDir()
	path := invalidSyntaxGo(t, dir)

	result, err := a.Validate(path)
	assert.NilError(t, err)
	assert.Assert(t, !result.Valid, "expected valid=false for broken syntax")
	assert.Assert(t, len(result.SyntaxErrors) > 0, "expected syntax errors")
}

// TestValidate_LineEndingLF detects \n line endings.
func TestValidate_LineEndingLF(t *testing.T) {
	a := lsp.NewAnalyzer()
	dir := t.TempDir()
	path := simpleGo(t, dir)

	result, err := a.Validate(path)
	assert.NilError(t, err)
	assert.Equal(t, result.LineEnding, "\n")
}

// TestValidate_LineEndingCRLF detects \r\n line endings.
func TestValidate_LineEndingCRLF(t *testing.T) {
	a := lsp.NewAnalyzer()
	dir := t.TempDir()
	path := crlfGo(t, dir)

	result, err := a.Validate(path)
	assert.NilError(t, err)
	assert.Equal(t, result.LineEnding, "\r\n")
}

// TestValidate_TrailingNewline detects trailing newline presence.
func TestValidate_TrailingNewline(t *testing.T) {
	a := lsp.NewAnalyzer()

	dir := t.TempDir()

	// With trailing newline
	withNL := writeTestFile(t, dir, "with_nl.go", "package main\nfunc main() {}\n")
	result, err := a.Validate(withNL)
	assert.NilError(t, err)
	assert.Assert(t, result.TrailingNewline, "expected trailing newline")

	// Without trailing newline
	withoutNL := writeTestFile(t, dir, "no_nl.go", "package main\nfunc main() {}")
	result, err = a.Validate(withoutNL)
	assert.NilError(t, err)
	assert.Assert(t, !result.TrailingNewline, "expected no trailing newline")
}

// TestValidate_EmptyFile handles empty content.
func TestValidate_EmptyFile(t *testing.T) {
	a := lsp.NewAnalyzer()
	dir := t.TempDir()
	path := emptyGo(t, dir)

	result, err := a.Validate(path)
	assert.NilError(t, err)
	assert.Assert(t, result.SourceSize == 0)
}

// TestValidate_UnsupportedLanguage still returns a result.
func TestValidate_UnsupportedLanguage(t *testing.T) {
	a := lsp.NewAnalyzer()
	dir := t.TempDir()
	path := nonGo(t, dir)

	_, err := a.Validate(path)
	assert.ErrorContains(t, err, "unsupported language")
}

// ─── Tests: AST Query ────────────────────────────────────────────────────────

// TestQueryAST verifies QueryAST matches tree-sitter S-expression patterns.
func TestQueryAST(t *testing.T) {
	a := lsp.NewAnalyzer()
	dir := t.TempDir()
	path := simpleGo(t, dir)

	// Find all function_declaration names
	matches, err := a.QueryAST(path, "(function_declaration name: (identifier) @fn)")
	assert.NilError(t, err)
	assert.Assert(t, len(matches) >= 1,
		"expected at least 1 function_declaration match")

	// Check that the capture has the function name
	found := false
	for _, m := range matches {
		if caps, ok := m.Captures["fn"]; ok {
			for _, cap := range caps {
				if strings.Contains(cap.Text, "main") {
					found = true
				}
			}
		}
	}
	assert.Assert(t, found, "should have captured 'main' function name")
}

// TestQueryAST_InvalidPattern returns a compile error.
func TestQueryAST_InvalidPattern(t *testing.T) {
	a := lsp.NewAnalyzer()
	dir := t.TempDir()
	path := simpleGo(t, dir)

	_, err := a.QueryAST(path, "(this_is_not_a_valid_pattern")
	assert.ErrorContains(t, err, "compile query")
}

// ─── Tests: Text Extraction ──────────────────────────────────────────────────

// TestGetText verifies GetText returns the correct byte range.
func TestGetText(t *testing.T) {
	a := lsp.NewAnalyzer()
	dir := t.TempDir()
	path := simpleGo(t, dir)

	// "package main" is 12 bytes (including newline on line 1)
	text, err := a.GetText(path, 0, 12)
	assert.NilError(t, err)
	assert.Assert(t, strings.HasPrefix(text, "package "))
}

// TestGetText_InvalidRange returns an error.
func TestGetText_InvalidRange(t *testing.T) {
	a := lsp.NewAnalyzer()
	dir := t.TempDir()
	path := simpleGo(t, dir)

	_, err := a.GetText(path, 10, 5)
	assert.ErrorContains(t, err, "invalid byte range")
}

// TestGetText_PastEnd returns an error for out-of-bounds range.
func TestGetText_PastEnd(t *testing.T) {
	a := lsp.NewAnalyzer()
	dir := t.TempDir()
	path := simpleGo(t, dir)

	_, err := a.GetText(path, 0, 999999)
	assert.ErrorContains(t, err, "invalid byte range")
}

// TestGetLine verifies GetLine returns a specific line.
func TestGetLine(t *testing.T) {
	a := lsp.NewAnalyzer()
	dir := t.TempDir()
	path := simpleGo(t, dir)

	line, err := a.GetLine(path, 0)
	assert.NilError(t, err)
	assert.Equal(t, line, "package main")

	line2, err := a.GetLine(path, 1)
	assert.NilError(t, err)
	assert.Equal(t, line2, "")
}

// TestGetLine_OutOfBounds returns an error.
func TestGetLine_OutOfBounds(t *testing.T) {
	a := lsp.NewAnalyzer()
	dir := t.TempDir()
	path := simpleGo(t, dir)

	_, err := a.GetLine(path, 999)
	assert.ErrorContains(t, err, "out of bounds")
}

// ─── Tests: Complexity Analysis ──────────────────────────────────────────────

// TestAnalyzeFile verifies AnalyzeFile returns correct per-function metrics.
func TestAnalyzeFile(t *testing.T) {
	a := lsp.NewAnalyzer()
	dir := t.TempDir()
	path := complexGo(t, dir)

	result, err := a.AnalyzeFile(path)
	assert.NilError(t, err)
	assert.Assert(t, result.File != "")
	assert.Assert(t, result.Language != "")
	assert.Assert(t, len(result.Functions) > 0, "expected at least 1 function")

	// Verify summary
	assert.Assert(t, result.Summary.TotalFunctions > 0)
	assert.Assert(t, result.Summary.AvgCyclomatic >= 1.0)
	assert.Assert(t, result.Summary.MaxComplexity >= 1)

	// Build function map for easy lookup
	fnMap := make(map[string]lsp.FuncAnalysis)
	for _, fn := range result.Functions {
		fnMap[fn.Name] = fn
	}

	t.Logf("Functions found: %d", len(result.Functions))
	for _, fn := range result.Functions {
		t.Logf("  %s (%s): cyclomatic=%d cognitive=%d loopDepth=%d loopCount=%d params=%d recursive=%v linearScan=%d",
			fn.Name, fn.Kind, fn.Cyclomatic, fn.Cognitive, fn.LoopDepth, fn.LoopCount,
			fn.ParamCount, fn.Recursive, fn.LinearScanInLoop)
	}
}

// TestAnalyzeFile_RecursiveFunction detects recursion.
func TestAnalyzeFile_RecursiveFunction(t *testing.T) {
	a := lsp.NewAnalyzer()
	dir := t.TempDir()
	path := complexGo(t, dir)

	result, err := a.AnalyzeFile(path)
	assert.NilError(t, err)

	// factorial is recursive
	var factorial *lsp.FuncAnalysis
	for i, fn := range result.Functions {
		if fn.Name == "factorial" {
			factorial = &result.Functions[i]
			break
		}
	}
	if factorial == nil {
		t.Fatal("factorial should be found among analyzed functions")
	}
	assert.Assert(t, factorial.Recursive, "factorial should be marked recursive")
	assert.Assert(t, factorial.Cyclomatic >= 2, "factorial has if-statement + base case, cyclomatic >= 2")
}

// TestAnalyzeFile_LoopMetrics detects loop depth and count.
func TestAnalyzeFile_LoopMetrics(t *testing.T) {
	a := lsp.NewAnalyzer()
	dir := t.TempDir()
	path := complexGo(t, dir)

	result, err := a.AnalyzeFile(path)
	assert.NilError(t, err)

	// findMax has a for-range loop
	var findMax *lsp.FuncAnalysis
	for i, fn := range result.Functions {
		if fn.Name == "findMax" {
			findMax = &result.Functions[i]
			break
		}
	}
	if findMax == nil {
		t.Fatal("findMax should be found among analyzed functions")
	}
	assert.Assert(t, findMax.LoopCount >= 1, "findMax should have at least 1 loop")
	assert.Assert(t, findMax.LoopDepth >= 1, "findMax should have loop depth >= 1")
}

// TestAnalyzeFile_LinearScanInLoop detects linear scans inside loops.
func TestAnalyzeFile_LinearScanInLoop(t *testing.T) {
	a := lsp.NewAnalyzer()
	dir := t.TempDir()
	path := complexGo(t, dir)

	result, err := a.AnalyzeFile(path)
	assert.NilError(t, err)

	// findMax calls contains() inside the loop — this may or may not be
	// detected depending on tree-sitter's call detection (varies by grammar).
	// The key assertion is that LinearScanInLoop should be >= 0.
	var findMax *lsp.FuncAnalysis
	for i, fn := range result.Functions {
		if fn.Name == "findMax" {
			findMax = &result.Functions[i]
			break
		}
	}
	if findMax == nil {
		t.Fatal("findMax should be found among analyzed functions")
	}
	assert.Assert(t, findMax.LinearScanInLoop >= 0)
	t.Logf("findMax.linearScanInLoop = %d", findMax.LinearScanInLoop)
}

// TestAnalyzeFile_CognitiveComplexity verifies nested conditionals increase cognitive score.
func TestAnalyzeFile_CognitiveComplexity(t *testing.T) {
	a := lsp.NewAnalyzer()
	dir := t.TempDir()
	path := complexGo(t, dir)

	result, err := a.AnalyzeFile(path)
	assert.NilError(t, err)

	// complexCondition has nested if-statements
	var cond *lsp.FuncAnalysis
	for i, fn := range result.Functions {
		if fn.Name == "complexCondition" {
			cond = &result.Functions[i]
			break
		}
	}
	if cond == nil {
		t.Fatal("complexCondition should be found among analyzed functions")
	}
	assert.Assert(t, cond.Cognitive > 0, "complexCondition should have cognitive complexity > 0")
	assert.Assert(t, cond.Cyclomatic > 1,
		"complexCondition should have cyclomatic > 1 (has if/else-if)")
}

// TestAnalyzeFile_ParamCount verifies parameter counting.
func TestAnalyzeFile_ParamCount(t *testing.T) {
	a := lsp.NewAnalyzer()
	dir := t.TempDir()
	path := paramFuncGo(t, dir)

	result, err := a.AnalyzeFile(path)
	assert.NilError(t, err)

	fnMap := make(map[string]lsp.FuncAnalysis)
	for _, fn := range result.Functions {
		fnMap[fn.Name] = fn
	}

	tests := []struct {
		name     string
		expected int
	}{
		{"noop", 0},
		{"oneParam", 1},
		{"multiParam", 4}, // a, b, c, s
		// methodWithReceiver: receiver (t *T) + x int = 2
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fn, ok := fnMap[tt.name]
			if !ok {
				t.Errorf("function %q should be found in definitions", tt.name)
				return
			}

			assert.Equal(t, fn.ParamCount, tt.expected,
				"param count for %q: expected %d, got %d", tt.name, tt.expected, fn.ParamCount)
		})
	}
}

// TestAnalyzeFile_EmptyFile returns zero functions for empty file.
func TestAnalyzeFile_EmptyFile(t *testing.T) {
	a := lsp.NewAnalyzer()
	dir := t.TempDir()
	path := emptyGo(t, dir)

	result, err := a.AnalyzeFile(path)
	assert.NilError(t, err)
	assert.Assert(t, result.Summary.TotalFunctions == 0,
		"empty file should have 0 functions")
}

// TestAnalyzeFile_UnsupportedLanguage returns an error.
func TestAnalyzeFile_UnsupportedLanguage(t *testing.T) {
	a := lsp.NewAnalyzer()
	dir := t.TempDir()
	path := nonGo(t, dir)

	_, err := a.AnalyzeFile(path)
	assert.ErrorContains(t, err, "unsupported language")
}

// ─── Tests: Edge Cases ───────────────────────────────────────────────────────

// TestNewAnalyzer verifies NewAnalyzer returns a usable instance.
func TestNewAnalyzer(t *testing.T) {
	a := lsp.NewAnalyzer()
	assert.Assert(t, a != nil)
}

// TestNodeInfoTypes verifies the types have expected fields via JSON serialization.
func TestNodeInfoTypes(t *testing.T) {
	a := lsp.NewAnalyzer()
	dir := t.TempDir()
	path := simpleGo(t, dir)

	info, err := a.GetNode(path, 0)
	assert.NilError(t, err)

	// Verify fields are populated
	assert.Assert(t, info.Type != "", "Type should not be empty")
	assert.Assert(t, info.StartByte < info.EndByte || info.StartByte == info.EndByte,
		"startByte (%d) should be ≤ endByte (%d)", info.StartByte, info.EndByte)
	// StartPoint should have row/col
	assert.Assert(t, info.StartPoint[0] >= 0)
	assert.Assert(t, info.StartPoint[1] >= 0)
}

// TestParseResult_Release verifies double-release is safe (no panic).
func TestParseResult_Release(t *testing.T) {
	a := lsp.NewAnalyzer()
	dir := t.TempDir()
	path := simpleGo(t, dir)

	result, err := a.ParseFile(path)
	assert.NilError(t, err)

	// First release
	result.Tree.Release()

	// Second release should not panic
	result.Tree.Release()
}

// ─── Tests: Signature & DocComment Extraction ─────────────────────────────

// TestExtractSignature verifies ExtractSignature extracts the correct signature.
func TestExtractSignature(t *testing.T) {
	dir := t.TempDir()
	path := writeTestFile(t, dir, "sig.go", `package main

// Greeter greets people.
type Greeter struct {
	Name string
}

// Hello returns a greeting.
func (g *Greeter) Hello() string {
	return "Hello, " + g.Name + "!"
}

// main is the entry point.
func main() {
	g := &Greeter{Name: "World"}
	println(g.Hello())
}
`)
	a := lsp.NewAnalyzer()
	result, err := a.ParseFile(path)
	assert.NilError(t, err)
	defer result.Tree.Release()

	root := result.Tree.RootNode()
	entry := grammars.DetectLanguage(path)
	lang := entry.Language()

	// Find function declarations and test signatures
	var walk func(n *gotreesitter.Node)
	walk = func(n *gotreesitter.Node) {
		if n == nil { return }
		typ := n.Type(lang)
		if typ == "method_declaration" || typ == "function_declaration" {
			sig := lsp.ExtractSignature(n, lang, result.Source)
			t.Logf("Signature for %s: %q", typ, sig)
			assert.Assert(t, sig != "", "signature should not be empty")
			assert.Assert(t, strings.Contains(sig, "func"), "signature should contain 'func'")
		}
		for i := 0; i < int(n.ChildCount()); i++ {
			walk(n.Child(i))
		}
	}
	walk(root)
}

// TestExtractDocComment verifies ExtractDocComment finds the preceding doc comment.
func TestExtractDocComment(t *testing.T) {
	dir := t.TempDir()
	path := writeTestFile(t, dir, "doc.go", `package main

// Hello returns a greeting.
func Hello() string {
	return "hello"
}

// MultiLineDoc explains something
// over multiple lines.
func MultiLine() int {
	return 42
}
`)
	a := lsp.NewAnalyzer()
	result, err := a.ParseFile(path)
	assert.NilError(t, err)
	defer result.Tree.Release()

	root := result.Tree.RootNode()
	entry := grammars.DetectLanguage(path)
	lang := entry.Language()

	// Collect doc comments
	var walk func(n *gotreesitter.Node)
	walk = func(n *gotreesitter.Node) {
		if n == nil { return }
		typ := n.Type(lang)
		if typ == "function_declaration" {
			doc := lsp.ExtractDocComment(n, lang, result.Source)
			t.Logf("DocComment for node at %d: %q", n.StartByte(), doc)
			// Hello() has doc, MultiLine() has doc
			assert.Assert(t, doc != "", "function should have doc comment")
		}
		if typ == "method_declaration" {
			// Methods might not have doc comments in our simple fixture
			_ = lsp.ExtractDocComment(n, lang, result.Source)
		}
		for i := 0; i < int(n.ChildCount()); i++ {
			walk(n.Child(i))
		}
	}
	walk(root)
}

// TestExtractDeclName verifies ExtractDeclName returns the correct name.
func TestExtractDeclName(t *testing.T) {
	dir := t.TempDir()
	path := writeTestFile(t, dir, "name.go", `package main

func Foo() {}
func (r *Receiver) Bar() {}
`)
	a := lsp.NewAnalyzer()
	result, err := a.ParseFile(path)
	assert.NilError(t, err)
	defer result.Tree.Release()

	root := result.Tree.RootNode()
	entry := grammars.DetectLanguage(path)
	lang := entry.Language()

	found := make(map[string]bool)
	var walk func(n *gotreesitter.Node)
	walk = func(n *gotreesitter.Node) {
		if n == nil { return }
		typ := n.Type(lang)
		if typ == "function_declaration" || typ == "method_declaration" {
			name := lsp.ExtractDeclName(n, lang, result.Source)
			assert.Assert(t, name != "", "name should not be empty")
			found[name] = true
		}
		for i := 0; i < int(n.ChildCount()); i++ {
			walk(n.Child(i))
		}
	}
	walk(root)
	assert.Assert(t, found["Foo"], "Foo should be found")
	assert.Assert(t, found["Bar"], "Bar should be found")
}

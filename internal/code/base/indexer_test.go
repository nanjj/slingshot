// Package base provides tests for the tree-sitter indexer — symbol extraction,
// complexity analysis, call detection, and full project indexing.
//
// These are white-box tests (package base) that exercise the unexported
// complexity and extraction functions directly, plus a full IndexProject test.
package base

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
	"gotest.tools/v3/assert"
)

// ─── Helpers ─────────────────────────────────────────────────────────────────

// parseGo parses Go source and returns tree, language, and source bytes.
// Cleans up the tree with t.Cleanup.
func parseGo(t *testing.T, source string) (*gotreesitter.Tree, *gotreesitter.Language, []byte) {
	t.Helper()
	src := []byte(source)
	entry := grammars.DetectLanguageByName("go")
	assert.Assert(t, entry != nil, "go grammar not found")
	lang := entry.Language()
	assert.Assert(t, lang != nil, "go language object nil")
	parser := gotreesitter.NewParser(lang)
	tree, err := parser.Parse(src)
	assert.NilError(t, err)
	t.Cleanup(tree.Release)
	return tree, lang, src
}

// findFirstBody uses the Tagger API (same as the real indexer) to find the first
// function/method body node, along with the language, source bytes, and function name.
func findFirstBody(t *testing.T, source string) (*gotreesitter.Node, *gotreesitter.Language, []byte, string) {
	t.Helper()
	tree, lang, src := parseGo(t, source)
	root := tree.RootNode()

	entry := grammars.DetectLanguageByName("go")
	tagsQuery := grammars.ResolveTagsQuery(*entry)
	tagger, err := gotreesitter.NewTagger(lang, tagsQuery)
	assert.NilError(t, err)

	tags := tagger.TagTree(tree)
	for _, tag := range tags {
		if !strings.HasPrefix(tag.Kind, "definition.") {
			continue
		}
		body := findFunctionBody(root, tag.Range, lang)
		if body != nil {
			return body, lang, src, tag.Name
		}
	}
	t.Fatal("no function body found via Tagger API")
	return nil, nil, nil, ""
}

// ─── kindFromTag ─────────────────────────────────────────────────────────────

func TestKindFromTag(t *testing.T) {
	tests := []struct {
		tagKind string
		want    string
	}{
		{"definition.function", "function"},
		{"definition.method", "method"},
		{"definition.class", "class"},
		{"definition.struct", "struct"},
		{"definition.interface", "interface"},
		{"definition.module", "module"},
		{"definition.type", "type"},
		{"definition.constant", "constant"},
		{"definition.variable", "variable"},
	}
	for _, tt := range tests {
		got := kindFromTag(tt.tagKind)
		assert.Equal(t, got, tt.want, "kindFromTag(%q)", tt.tagKind)
	}
}

func TestKindFromTagUnknown(t *testing.T) {
	// unknown prefix after "definition." returns the suffix
	assert.Equal(t, kindFromTag("definition.foo"), "foo")
	// non-definition prefix returns as-is
	assert.Equal(t, kindFromTag("reference.function"), "reference.function")
}

// ─── extractVarConstDefs ─────────────────────────────────────────────────────

func TestExtractVarConstDefs_GoVar(t *testing.T) {
	tree, lang, src := parseGo(t, `package main
var x = 5
func main() { _ = x }`)
	root := tree.RootNode()
	nodes := extractVarConstDefs(root, lang, src, 1, "test.go")
	assert.Equal(t, len(nodes), 1, "expected 1 var node")
	if len(nodes) > 0 {
		assert.Equal(t, nodes[0].Kind, "variable")
		assert.Equal(t, nodes[0].Name, "x")
		assert.Equal(t, nodes[0].QualifiedName, "variable:x")
	}
}

func TestExtractVarConstDefs_GoConst(t *testing.T) {
	tree, lang, src := parseGo(t, `package main
const MAX_SIZE = 100
func main() { _ = MAX_SIZE }`)
	root := tree.RootNode()
	nodes := extractVarConstDefs(root, lang, src, 1, "test.go")
	assert.Equal(t, len(nodes), 1, "expected 1 const node")
	if len(nodes) > 0 {
		assert.Equal(t, nodes[0].Kind, "constant")
		assert.Equal(t, nodes[0].Name, "MAX_SIZE")
		assert.Equal(t, nodes[0].QualifiedName, "constant:MAX_SIZE")
	}
}

func TestExtractVarConstDefs_GoShortVar(t *testing.T) {
	tree, lang, src := parseGo(t, `package main
func f() {
	x := 5
	_ = x
}`)
	root := tree.RootNode()
	nodes := extractVarConstDefs(root, lang, src, 1, "test.go")
	// short_var_declaration inside a function body should NOT be extracted
	// because we skip function bodies to avoid local variables
	assert.Equal(t, len(nodes), 0, "expected 0 variable nodes from local scope")
}

func TestExtractVarConstDefs_GoMultiVar(t *testing.T) {
	tree, lang, src := parseGo(t, `package main
var a, b = 1, 2
func main() { _, _ = a, b }`)
	root := tree.RootNode()
	nodes := extractVarConstDefs(root, lang, src, 1, "test.go")
	assert.Equal(t, len(nodes), 2, "expected 2 var nodes for a, b")
	names := make(map[string]bool)
	for _, n := range nodes {
		names[n.Name] = true
	}
	assert.Assert(t, names["a"], "expected variable 'a'")
	assert.Assert(t, names["b"], "expected variable 'b'")
}

func TestExtractVarConstDefs_PackageLevel(t *testing.T) {
	tree, lang, src := parseGo(t, `package main
var debug = false
const AppName = "test"
func main() {
	msg := "hello"
	_ = msg
}`)
	root := tree.RootNode()
	nodes := extractVarConstDefs(root, lang, src, 1, "test.go")
	assert.Equal(t, len(nodes), 2, "expected 2 nodes (var debug + const AppName)")
	got := make(map[string]string)
	for _, n := range nodes {
		got[n.Name] = n.Kind
	}
	assert.Equal(t, got["debug"], "variable")
	assert.Equal(t, got["AppName"], "constant")
}

// ─── extractVarRefs ──────────────────────────────────────────────────────────

func TestExtractVarRefs_Simple(t *testing.T) {
	body, lang, src, name := findFirstBody(t, `package main
var count = 0
func use() {
	println(count)
}`)
	varNameMap := map[string]string{"count": "variable:count"}
	edges := extractVarRefs(body, lang, src, name, varNameMap, 1)
	assert.Assert(t, len(edges) >= 1, "expected at least 1 REFERENCES edge for count")
	if len(edges) > 0 {
		assert.Equal(t, edges[0].EdgeType, "REFERENCES")
		assert.Equal(t, edges[0].SourceQN, "use")
		assert.Equal(t, edges[0].TargetQN, "variable:count")
	}
}

func TestExtractVarRefs_NoMatch(t *testing.T) {
	body, lang, src, name := findFirstBody(t, `package main
func use() {
	println("hello")
}`)
	varNameMap := map[string]string{"count": "variable:count"}
	edges := extractVarRefs(body, lang, src, name, varNameMap, 1)
	assert.Equal(t, len(edges), 0, "expected 0 REFERENCES edges, 'count' not referenced")
}

func TestExtractVarRefs_MultipleVars(t *testing.T) {
	body, lang, src, name := findFirstBody(t, `package main
var x, y = 1, 2
func use() {
	println(x + y)
}`)
	varNameMap := map[string]string{"x": "variable:x", "y": "variable:y"}
	edges := extractVarRefs(body, lang, src, name, varNameMap, 1)
	assert.Assert(t, len(edges) >= 2, "expected ≥2 REFERENCES edges for x and y")
	found := make(map[string]bool)
	for _, e := range edges {
		found[e.TargetQN] = true
	}
	assert.Assert(t, found["variable:x"], "expected reference to x")
	assert.Assert(t, found["variable:y"], "expected reference to y")
}

func TestExtractVarRefs_CallArgs(t *testing.T) {
	body, lang, src, name := findFirstBody(t, `package main
var msg = "hello"
func use() {
	fmt.Println(msg)
}`)
	varNameMap := map[string]string{"msg": "variable:msg"}
	edges := extractVarRefs(body, lang, src, name, varNameMap, 1)
	// msg is a call argument, should still be detected as a reference
	assert.Assert(t, len(edges) >= 1, "expected REFERENCE to msg in call args")
}

func TestExtractVarRefs_NotCallTarget(t *testing.T) {
	body, lang, src, name := findFirstBody(t, `package main
var println = "shadow"
func use() {
	println("hello")  // this 'println' is a call target, not a var ref
}`)
	varNameMap := map[string]string{"println": "variable:println"}
	edges := extractVarRefs(body, lang, src, name, varNameMap, 1)
	// The 'println' in call position should NOT be treated as a var reference
	assert.Equal(t, len(edges), 0, "expected 0 REFERENCES edges — call target is not a var ref")
}

// ─── buildVarNameMap ─────────────────────────────────────────────────────────

func TestBuildVarNameMap(t *testing.T) {
	nodes := []Node{
		{Kind: "variable", Name: "x", QualifiedName: "variable:x"},
		{Kind: "constant", Name: "MAX", QualifiedName: "constant:MAX"},
		{Kind: "function", Name: "main", QualifiedName: "main"},
	}
	m := buildVarNameMap(nodes)
	assert.Equal(t, len(m), 2, "expected 2 entries (variable + constant)")
	assert.Equal(t, m["x"], "variable:x")
	assert.Equal(t, m["MAX"], "constant:MAX")
	_, ok := m["main"]
	assert.Assert(t, !ok, "function names should not be in var name map")
}

// ─── Integration: var/const in IndexProject ──────────────────────────────────

func TestIndexProject_GoFileWithVarConst(t *testing.T) {
	dir := t.TempDir()
	src := `package main

import "fmt"

var debug = true

const AppName = "test-app"

func main() {
	if debug {
		fmt.Println("debug:", AppName)
	}
}
`
	err := os.WriteFile(filepath.Join(dir, "main.go"), []byte(src), 0644)
	assert.NilError(t, err)

	dbPath := filepath.Join(t.TempDir(), "test_vc.db")
	store, err := OpenStore(dbPath)
	assert.NilError(t, err)
	defer store.Close()

	result, err := store.IndexProject(dir, "test-vc-project", IndexModeFull)
	assert.NilError(t, err)
	assert.Assert(t, result.FilesParsed >= 1)
	assert.Equal(t, result.Errors, 0)

	// Should have: main (function), debug (variable), AppName (constant) — at least 3
	assert.Assert(t, result.NodesStored >= 3,
		"expected ≥3 nodes (main + debug + AppName), got %d", result.NodesStored)

	// Verify variable node was stored
	debugNode, err := store.GetNodeByQN("variable:debug", "test-vc-project")
	assert.NilError(t, err)
	assert.Equal(t, debugNode.Kind, "variable")
	assert.Equal(t, debugNode.Name, "debug")

	// Verify constant node was stored
	constNode, err := store.GetNodeByQN("constant:AppName", "test-vc-project")
	assert.NilError(t, err)
	assert.Equal(t, constNode.Kind, "constant")
	assert.Equal(t, constNode.Name, "AppName")

	// Verify REFERENCES edges exist: main → debug, main → AppName
	refs, err := store.GetReferences("variable:debug", "test-vc-project", "inbound", 1)
	assert.NilError(t, err)
	assert.Assert(t, len(refs) > 0, "expected inbound REFERENCES to variable:debug")

	refs, err = store.GetReferences("constant:AppName", "test-vc-project", "inbound", 1)
	assert.NilError(t, err)
	assert.Assert(t, len(refs) > 0, "expected inbound REFERENCES to constant:AppName")

	// Verify CALLS still work
	refs, err = store.GetReferences("fmt.Println", "test-vc-project", "inbound", 1)
	assert.NilError(t, err)
	assert.Assert(t, len(refs) > 0, "expected inbound CALLS to fmt.Println")
}

func TestIndexProject_GoFileVarRefs(t *testing.T) {
	dir := t.TempDir()
	src := `package main

var counter = 0

func increment() {
	counter++
}

func main() {
	increment()
	println(counter)
}
`
	err := os.WriteFile(filepath.Join(dir, "main.go"), []byte(src), 0644)
	assert.NilError(t, err)

	dbPath := filepath.Join(t.TempDir(), "test_refs.db")
	store, err := OpenStore(dbPath)
	assert.NilError(t, err)
	defer store.Close()

	result, err := store.IndexProject(dir, "test-refs-project", IndexModeFull)
	assert.NilError(t, err)
	assert.Equal(t, result.Errors, 0)

	// Verify variable node
	_, err = store.GetNodeByQN("variable:counter", "test-refs-project")
	assert.NilError(t, err)

	// Verify REFERENCES edges: increment → counter, main → counter
	refs, err := store.GetReferences("variable:counter", "test-refs-project", "inbound", 1)
	assert.NilError(t, err)
	assert.Assert(t, len(refs) >= 2,
		"expected ≥2 REFERENCES to counter (from increment + main), got %d", len(refs))
}


// ─── isBuiltinOrKeyword ──────────────────────────────────────────────────────

func TestIsBuiltinOrKeyword(t *testing.T) {
	builtins := []string{"if", "for", "while", "switch", "return", "throw",
		"new", "delete", "typeof", "instanceof", "void", "sizeof", "assert"}
	for _, b := range builtins {
		assert.Assert(t, isBuiltinOrKeyword(b), "expected builtin: %q", b)
	}

	nonBuiltins := []string{"fmt", "Println", "Hello", "main", "Greeter", "foo"}
	for _, nb := range nonBuiltins {
		assert.Assert(t, !isBuiltinOrKeyword(nb), "expected non-builtin: %q", nb)
	}
}

// ─── cyclomaticComplexity ────────────────────────────────────────────────────

func cyclomaticSrc(t *testing.T, source string) (*gotreesitter.Node, *gotreesitter.Language, []byte) {
	t.Helper()
	body, lang, src, _ := findFirstBody(t, source)
	return body, lang, src
}

func TestCyclomaticComplexity_Simple(t *testing.T) {
	body, lang, src := cyclomaticSrc(t, `package main
func simple() { x := 1 }`)
	c := cyclomaticComplexity(body, lang, src)
	assert.Equal(t, c, 1, "simple function should have base complexity 1")
}

func TestCyclomaticComplexity_If(t *testing.T) {
	body, lang, src := cyclomaticSrc(t, `package main
func branch(a int) int {
	if a > 0 { return a }
	return 0
}`)
	c := cyclomaticComplexity(body, lang, src)
	assert.Equal(t, c, 2) // 1 base + 1 if_statement
}

func TestCyclomaticComplexity_IfElse(t *testing.T) {
	body, lang, src := cyclomaticSrc(t, `package main
func branch(a int) int {
	if a > 0 { return a } else { return -1 }
}`)
	c := cyclomaticComplexity(body, lang, src)
	assert.Assert(t, c >= 2, "expected ≥2 (base + if), got %d", c)
}

func TestCyclomaticComplexity_ForLoop(t *testing.T) {
	body, lang, src := cyclomaticSrc(t, `package main
func sum(n int) int {
	s := 0
	for i := 0; i < n; i++ { s += i }
	return s
}`)
	c := cyclomaticComplexity(body, lang, src)
	assert.Equal(t, c, 2) // 1 base + 1 for_statement
}

func TestCyclomaticComplexity_Switch(t *testing.T) {
	body, lang, src := cyclomaticSrc(t, `package main
func kind(x int) string {
	switch x {
	case 0: return "zero"
	case 1: return "one"
	default: return "other"
	}
}`)
	c := cyclomaticComplexity(body, lang, src)
	// 1 base + 1 switch + 2 cases (case_statement x2)
	assert.Assert(t, c >= 3)
}

func TestCyclomaticComplexity_AndOperator(t *testing.T) {
	body, lang, src := cyclomaticSrc(t, `package main
func check(a, b int) bool {
	return a > 0 && b > 0
}`)
	c := cyclomaticComplexity(body, lang, src)
	assert.Equal(t, c, 2) // 1 base + 1 for &&
}

func TestCyclomaticComplexity_OrOperator(t *testing.T) {
	body, lang, src := cyclomaticSrc(t, `package main
func check(a, b int) bool {
	return a > 0 || b > 0
}`)
	c := cyclomaticComplexity(body, lang, src)
	assert.Equal(t, c, 2) // 1 base + 1 for ||
}

func TestCyclomaticComplexity_Mixed(t *testing.T) {
	body, lang, src := cyclomaticSrc(t, `package main
func complex(a, b, c int) int {
	if a > 0 {
		for i := 0; i < b; i++ {
			if c > 0 && i > 0 { return i }
		}
	}
	return 0
}`)
	c := cyclomaticComplexity(body, lang, src)
	// 1 base + 1 if(a) + 1 for + 1 if(c) + 1 && = 5
	assert.Equal(t, c, 5)
}

// findBodyWithLang returns body and language (discarding src and name).
func findBodyWithLang(t *testing.T, source string) (*gotreesitter.Node, *gotreesitter.Language) {
	t.Helper()
	body, lang, _, _ := findFirstBody(t, source)
	return body, lang
}

func TestCognitiveComplexity_Simple(t *testing.T) {
	body, lang := findBodyWithLang(t, `package main
func simple() { x := 1 }`)
	c := cognitiveComplexity(body, lang, 0)
	assert.Equal(t, c, 0)
}

func TestCognitiveComplexity_Flat(t *testing.T) {
	body, lang, _, _ := findFirstBody(t, `package main
func flat(a, b int) int {
	if a > 0 { return a }
	if b > 0 { return b }
	return 0
}`)
	c := cognitiveComplexity(body, lang, 0)
	// if(a): 1+0=1, if(b): 1+0=1, total=2
	assert.Equal(t, c, 2)
}

func TestCognitiveComplexity_Nested(t *testing.T) {
	body, lang, _, _ := findFirstBody(t, `package main
func nested(a, b, c int) {
	if a > 0 {
		if b > 0 {
			if c > 0 { _ = a + b + c }
		}
	}
}`)
	c := cognitiveComplexity(body, lang, 0)
	// if(a): nesting=0, add 1
	//   if(b): nesting=1, add 1+1=2
	//     if(c): nesting=2, add 1+2=3
	// Total: 1+2+3=6
	assert.Equal(t, c, 6)
}

func TestCognitiveComplexity_ForLoop(t *testing.T) {
	body, lang, _, _ := findFirstBody(t, `package main
func loop(n int) {
	for i := 0; i < n; i++ {
		if i%2 == 0 { _ = i }
	}
}`)
	c := cognitiveComplexity(body, lang, 0)
	// for: nesting=0, add 1+0=1
	//   if: nesting=1, add 1+1=2
	// Total: 1+2=3
	assert.Equal(t, c, 3)
}

// ─── maxLoopDepth ────────────────────────────────────────────────────────────

func TestMaxLoopDepth_NoLoops(t *testing.T) {
	body, lang, _, _ := findFirstBody(t, `package main
func simple() { x := 1 }`)
	d := maxLoopDepth(body, lang, 0)
	assert.Equal(t, d, 0)
}

func TestMaxLoopDepth_Single(t *testing.T) {
	body, lang, _, _ := findFirstBody(t, `package main
func sum(n int) int {
	s := 0
	for i := 0; i < n; i++ { s += i }
	return s
}`)
	d := maxLoopDepth(body, lang, 0)
	assert.Equal(t, d, 1)
}

func TestMaxLoopDepth_Nested(t *testing.T) {
	body, lang, _, _ := findFirstBody(t, `package main
func nested(n int) {
	for i := 0; i < n; i++ {
		for j := 0; j < n; j++ {
			for k := 0; k < n; k++ { _ = i + j + k }
		}
	}
}`)
	d := maxLoopDepth(body, lang, 0)
	assert.Equal(t, d, 3)
}

// ─── countLoops ──────────────────────────────────────────────────────────────

func TestCountLoops_NoLoops(t *testing.T) {
	body, lang, _, _ := findFirstBody(t, `package main
func simple() { x := 1 }`)
	c := countLoops(body, lang)
	assert.Equal(t, c, 0)
}

func TestCountLoops_Multiple(t *testing.T) {
	body, lang, _, _ := findFirstBody(t, `package main
func multi(n int) {
	for i := 0; i < n; i++ { _ = i }
	for j := 0; j < n; j++ { _ = j }
}`)
	c := countLoops(body, lang)
	assert.Equal(t, c, 2)
}

// ─── isRecursive ─────────────────────────────────────────────────────────────

func TestIsRecursive_Direct(t *testing.T) {
	body, lang, src, name := findFirstBody(t, `package main
func fact(n int) int {
	if n <= 1 { return 1 }
	return n * fact(n-1)
}`)
	assert.Equal(t, name, "fact")
	r := isRecursive(body, name, lang, src)
	assert.Assert(t, r, "fact should be recursive")
}

func TestIsRecursive_NonRecursive(t *testing.T) {
	body, lang, src, name := findFirstBody(t, `package main
func add(a, b int) int {
	return a + b
}`)
	r := isRecursive(body, name, lang, src)
	assert.Assert(t, !r, "add should not be recursive")
}

func TestIsRecursive_MethodCall(t *testing.T) {
	body, lang, src, name := findFirstBody(t, `package main
type R struct{}
func (r *R) self() string {
	return r.self()
}`)
	r := isRecursive(body, name, lang, src)
	assert.Assert(t, r, "self should be recursive (method call)")
}

// ─── extractCalls ────────────────────────────────────────────────────────────

func TestExtractCalls_NoCalls(t *testing.T) {
	body, lang, src, _ := findFirstBody(t, `package main
func simple() { x := 1 }`)
	calls := extractCalls(body, lang, src)
	assert.Assert(t, len(calls) == 0, "expected no calls, got %v", calls)
}

func TestExtractCalls_FunctionCalls(t *testing.T) {
	body, lang, src, _ := findFirstBody(t, `package main
import "fmt"
func greet(name string) {
	fmt.Println("Hello", name)
	greet("World")
}`)
	calls := extractCalls(body, lang, src)
	// Should find: fmt.Println, greet (self-call)
	assert.Assert(t, len(calls) >= 2, "expected at least 2 calls, got %v", calls)
	hasFmt := false
	hasGreet := false
	for _, c := range calls {
		if c == "fmt.Println" {
			hasFmt = true
		}
		if c == "greet" {
			hasGreet = true
		}
	}
	assert.Assert(t, hasFmt, "expected fmt.Println in calls")
	assert.Assert(t, hasGreet, "expected greet in calls")
}

func TestExtractCalls_FiltersBuiltins(t *testing.T) {
	body, lang, src, _ := findFirstBody(t, `package main
func use() {
	if true { _ = 1 }
	for i := 0; i < 10; i++ { _ = i }
}`)
	calls := extractCalls(body, lang, src)
	// "if" and "for" are builtins, should be filtered
	assert.Assert(t, len(calls) == 0, "expected no builtin calls, got %v", calls)
}

// ─── nodeText ────────────────────────────────────────────────────────────────

func TestNodeText(t *testing.T) {
	tree, lang, src := parseGo(t, `package main
func foo() {}`)
	root := tree.RootNode()
	// Find the identifier "foo" inside the function_declaration
	var found bool
	var find func(n *gotreesitter.Node)
	find = func(n *gotreesitter.Node) {
		if found || n == nil {
			return
		}
		if n.Type(lang) == "identifier" {
			text := nodeText(n, lang, src)
			assert.Equal(t, text, "foo")
			found = true
			return
		}
		for i := 0; i < int(n.ChildCount()); i++ {
			find(n.Child(i))
		}
	}
	find(root)
	assert.Assert(t, found, "expected to find identifier 'foo'")
}

func TestNodeText_NilSource(t *testing.T) {
	tree, lang, _ := parseGo(t, `package main
func foo() {}`)
	root := tree.RootNode()
	var found bool
	var find func(n *gotreesitter.Node)
	find = func(n *gotreesitter.Node) {
		if found || n == nil {
			return
		}
		if n.Type(lang) == "identifier" {
			text := nodeText(n, lang, nil)
			assert.Equal(t, text, "", "nodeText with nil source should return empty")
			found = true
			return
		}
		for i := 0; i < int(n.ChildCount()); i++ {
			find(n.Child(i))
		}
	}
	find(root)
	assert.Assert(t, found)
}

// ─── findBodyRecursive / isBodyNode ──────────────────────────────────────────

func TestIsBodyNode(t *testing.T) {
	bodyTypes := []string{"block", "block_statement", "compound_statement",
		"statement_block", "body", "declaration_list"}
	for _, bt := range bodyTypes {
		assert.Assert(t, isBodyNode(bt), "expected %q to be a body node", bt)
	}
	assert.Assert(t, !isBodyNode("if_statement"))
	assert.Assert(t, !isBodyNode("source_file"))
}

func TestFindFunctionBody(t *testing.T) {
	tree, lang, _ := parseGo(t, `package main
func foo() { x := 1 }`)
	root := tree.RootNode()
	entry := grammars.DetectLanguageByName("go")
	tagsQuery := grammars.ResolveTagsQuery(*entry)
	tagger, err := gotreesitter.NewTagger(lang, tagsQuery)
	assert.NilError(t, err)
	tags := tagger.TagTree(tree)

	var body *gotreesitter.Node
	for _, tag := range tags {
		if strings.HasPrefix(tag.Kind, "definition.") && tag.Name == "foo" {
			body = findFunctionBody(root, tag.Range, lang)
			break
		}
	}
	assert.Assert(t, body != nil, "expected to find function body for 'foo'")
	assert.Equal(t, body.Type(lang), "block")
}

// ─── countParams ─────────────────────────────────────────────────────────────

func TestCountParams(t *testing.T) {
	tree, lang, _ := parseGo(t, `package main
func add(a, b int) int { return a + b }`)
	root := tree.RootNode()
	entry := grammars.DetectLanguageByName("go")
	tagsQuery := grammars.ResolveTagsQuery(*entry)
	tagger, err := gotreesitter.NewTagger(lang, tagsQuery)
	assert.NilError(t, err)
	tags := tagger.TagTree(tree)

	for _, tag := range tags {
		if strings.HasPrefix(tag.Kind, "definition.") && tag.Name == "add" {
			body := findFunctionBody(root, tag.Range, lang)
			assert.Assert(t, body != nil)
			n := countParams(body, tag.Kind, lang)
			assert.Equal(t, n, 2)
			return
		}
	}
	t.Fatal("tag for 'add' not found")
}

// ─── IndexProject (full pipeline) ────────────────────────────────────────────

func TestIndexProject_GoFile(t *testing.T) {
	// Create a temp directory with a Go source file
	dir := t.TempDir()
	src := `package main

import "fmt"

// Greeter greets people.
type Greeter struct {
	Name string
}

// Hello returns a greeting message.
func (g *Greeter) Hello() string {
	return "Hello, " + g.Name + "!"
}

// main is the entry point.
func main() {
	g := &Greeter{Name: "World"}
	fmt.Println(g.Hello())
}
`
	err := os.WriteFile(filepath.Join(dir, "main.go"), []byte(src), 0644)
	assert.NilError(t, err)

	// Open a temp database
	dbPath := filepath.Join(t.TempDir(), "test.db")
	store, err := OpenStore(dbPath)
	assert.NilError(t, err)
	defer store.Close()

	// Index the project
	result, err := store.IndexProject(dir, "test-project", IndexModeFull)
	assert.NilError(t, err)
	assert.Assert(t, result.FilesParsed >= 1)
	// Go tags query: only functions and methods are tagged, not structs.
	assert.Assert(t, result.NodesStored >= 2,
		"expected ≥2 nodes (Hello + main), got %d", result.NodesStored)
	assert.Assert(t, result.EdgesStored >= 1,
		"expected ≥1 edge (fmt.Println, g.Hello), got %d", result.EdgesStored)
	assert.Equal(t, result.Errors, 0)

	// Verify the project was marked ready
	info, err := store.ProjectStatus("test-project")
	assert.NilError(t, err)
	assert.Equal(t, info.Status, "ready")

	// Verify specific nodes were indexed — now package-qualified
	hello, err := store.GetNodeByQN("main.Hello", "test-project")
	assert.NilError(t, err)
	assert.Equal(t, hello.Kind, "method")
	assert.Equal(t, hello.Name, "Hello")

	mainfn, err := store.GetNodeByQN("main.main", "test-project")
	assert.NilError(t, err)
	assert.Equal(t, mainfn.Kind, "function")
	assert.Assert(t, mainfn.Complexity >= 1, "main should have complexity ≥1")

	// Verify edges (calls) were extracted
	refs, err := store.GetReferences("fmt.Println", "test-project", "inbound", 1)
	assert.NilError(t, err)
	assert.Assert(t, len(refs) > 0, "expected inbound refs to fmt.Println")
}

func TestIndexProject_MultiFile(t *testing.T) {
	dir := t.TempDir()

	// main.go
	err := os.WriteFile(filepath.Join(dir, "main.go"), []byte(`package main

import "fmt"

func main() {
	fmt.Println(helper())
}

func helper() string {
	return "hello"
}
`), 0644)
	assert.NilError(t, err)

	// utils.go
	err = os.WriteFile(filepath.Join(dir, "utils.go"), []byte(`package main

func add(a, b int) int {
	return a + b
}
`), 0644)
	assert.NilError(t, err)

	dbPath := filepath.Join(t.TempDir(), "multi.db")
	store, err := OpenStore(dbPath)
	assert.NilError(t, err)
	defer store.Close()

	result, err := store.IndexProject(dir, "multi-project", IndexModeFull)
	assert.NilError(t, err)
	assert.Equal(t, result.FilesParsed, 2)
	assert.Assert(t, result.NodesStored >= 3) // main, helper, add
	assert.Equal(t, result.Errors, 0)

	// Verify cross-file call: helper called from main
	refs, err := store.GetReferences("main.helper", "multi-project", "inbound", 1)
	assert.NilError(t, err)
	assert.Assert(t, len(refs) > 0, "helper should have inbound calls from main")
}

func TestIndexProject_UnsupportedFile(t *testing.T) {
	dir := t.TempDir()
	// Write a file with unsupported extension
	err := os.WriteFile(filepath.Join(dir, "foo.xyz"), []byte("some content"), 0644)
	assert.NilError(t, err)

	dbPath := filepath.Join(t.TempDir(), "unsup.db")
	store, err := OpenStore(dbPath)
	assert.NilError(t, err)
	defer store.Close()

	result, err := store.IndexProject(dir, "unsupported", IndexModeFull)
	assert.NilError(t, err)
	assert.Equal(t, result.FilesParsed, 0) // no supported files
	assert.Equal(t, result.Errors, 0)
}

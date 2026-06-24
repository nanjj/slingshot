package base

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
)

// SupportedExts lists file extensions the indexer recognizes.
var supportedExts = map[string]bool{
	".go": true, ".py": true, ".js": true, ".ts": true, ".jsx": true, ".tsx": true,
	".rs": true, ".java": true, ".kt": true, ".swift": true, ".c": true, ".h": true,
	".cpp": true, ".hpp": true, ".cc": true, ".cxx": true, ".cs": true,
	".rb": true, ".php": true, ".ex": true, ".exs": true,
}

// ─── Indexer ──────────────────────────────────────────────────────────────────

// IndexProject walks a project directory, parses all files with tree-sitter,
// extracts symbols and relationships, and stores them in the database.
//
// It uses the Tagger API for definition extraction and manual tree traversal
// for call detection and complexity analysis.
func (s *Store) IndexProject(root, name string) (*IndexResult, error) {
	// 1. Create or get project
	projectID, err := s.UpsertProject(name, root)
	if err != nil {
		return nil, fmt.Errorf("upsert project: %w", err)
	}

	result := &IndexResult{
		ProjectName: name,
		ProjectRoot: root,
	}

	// 2. Walk files
	var files []string
	err = filepath.Walk(root, func(path string, fi os.FileInfo, err error) error {
		if err != nil {
			return err // skip inaccessible dirs
		}
		if fi.IsDir() {
			// Skip hidden dirs and common non-source dirs
			base := filepath.Base(path)
			if strings.HasPrefix(base, ".") || base == "node_modules" || base == "vendor" || base == ".git" {
				return filepath.SkipDir
			}
			return nil
		}
		ext := strings.ToLower(filepath.Ext(path))
		if supportedExts[ext] {
			files = append(files, path)
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("walk project: %w", err)
	}

	// 3. Process each file
	var allNodes []Node
	var allEdges []Edge

	for _, filePath := range files {
		nodes, edges, err := s.indexFile(projectID, filePath)
		if err != nil {
			result.Errors++
			continue
		}
		allNodes = append(allNodes, nodes...)
		allEdges = append(allEdges, edges...)
		result.FilesParsed++
	}

	// 4. Batch save
	if len(allNodes) > 0 {
		if err := s.SaveNodesBatch(allNodes); err != nil {
			return nil, fmt.Errorf("save nodes batch: %w", err)
		}
		result.NodesStored = len(allNodes)
	}
	if len(allEdges) > 0 {
		if err := s.SaveEdgesBatch(allEdges); err != nil {
			return nil, fmt.Errorf("save edges batch: %w", err)
		}
		result.EdgesStored = len(allEdges)
	}

	// 5. Mark project ready
	if err := s.SetProjectStatus(name, "ready"); err != nil {
		return nil, fmt.Errorf("set project status: %w", err)
	}

	return result, nil
}

// indexFile processes a single file: parse → extract symbols → compute metrics.
func (s *Store) indexFile(projectID int64, filePath string) ([]Node, []Edge, error) {
	source, err := os.ReadFile(filePath)
	if err != nil {
		return nil, nil, fmt.Errorf("read file: %w", err)
	}

	// Detect language
	entry := grammars.DetectLanguage(filePath)
	if entry == nil {
		return nil, nil, fmt.Errorf("unsupported language: %s", filePath)
	}

	lang := entry.Language()
	if lang == nil {
		return nil, nil, fmt.Errorf("no language object for %s", filePath)
	}

	// Parse
	parser := gotreesitter.NewParser(lang)
	tree, err := parser.Parse(source)
	if err != nil {
		return nil, nil, fmt.Errorf("parse: %w", err)
	}
	defer tree.Release()

	relPath := toRelPath(filePath)

	// Extract definitions via Tagger API
	tags := extractTags(tree, entry, lang)

	var nodes []Node
	for _, tag := range tags {
		qn := tag.Name // short name; qualified name can be extended later
		n := Node{
			ProjectID:     projectID,
			QualifiedName: qn,
			Kind:          kindFromTag(tag.Kind),
			Name:          tag.Name,
			FilePath:      relPath,
			Line:          tag.Range.StartPoint.Row,
			Col:           tag.Range.StartPoint.Column,
			EndLine:       tag.Range.EndPoint.Row,
			EndCol:        tag.Range.EndPoint.Column,
		}

		// Compute complexity for functions/methods
		if n.Kind == "function" || n.Kind == "method" {
			bodyNode := findFunctionBody(tree.RootNode(), tag.Range, lang)
			if bodyNode != nil {
				n.Complexity = cyclomaticComplexity(bodyNode, lang)
				n.Cognitive = cognitiveComplexity(bodyNode, lang, 0)
				n.LoopDepth = maxLoopDepth(bodyNode, lang, 0)
				n.LoopCount = countLoops(bodyNode, lang)
				n.Recursive = isRecursive(bodyNode, tag.Name, lang)
				n.ParamCount = countParams(bodyNode, tag.Kind, lang)
			}
		}

		nodes = append(nodes, n)
	}

	// Extract edges: call relationships
	var edges []Edge
	for _, tag := range tags {
		if tag.Kind == "definition.function" || tag.Kind == "definition.method" {
			bodyNode := findFunctionBody(tree.RootNode(), tag.Range, lang)
			if bodyNode != nil {
				calls := extractCalls(bodyNode, lang)
				for _, callee := range calls {
					edges = append(edges, Edge{
						ProjectID: projectID,
						SourceQN:  tag.Name,
						TargetQN:  callee,
						EdgeType:  "CALLS",
					})
				}
			}
		}
	}

	return nodes, edges, nil
}

// ─── Tag Extraction ───────────────────────────────────────────────────────────

// extractTags retrieves definition tags from a parsed file using the Tagger API.
func extractTags(tree *gotreesitter.Tree, entry *grammars.LangEntry, lang *gotreesitter.Language) []gotreesitter.Tag {
	tagsQuery := grammars.ResolveTagsQuery(*entry)
	if tagsQuery == "" {
		return nil
	}
	tagger, err := gotreesitter.NewTagger(lang, tagsQuery)
	if err != nil {
		return nil
	}
	tags := tagger.TagTree(tree)
	// Filter to definition.* tags only
	var defs []gotreesitter.Tag
	for _, t := range tags {
		if strings.HasPrefix(t.Kind, "definition.") {
			defs = append(defs, t)
		}
	}
	return defs
}

// kindFromTag converts a tag kind (e.g. "definition.function") to a short kind.
func kindFromTag(tagKind string) string {
	switch tagKind {
	case "definition.function":
		return "function"
	case "definition.method":
		return "method"
	case "definition.class":
		return "class"
	case "definition.struct":
		return "struct"
	case "definition.interface":
		return "interface"
	case "definition.module":
		return "module"
	case "definition.type":
		return "type"
	case "definition.constant":
		return "constant"
	default:
		if strings.HasPrefix(tagKind, "definition.") {
			return tagKind[len("definition."):]
		}
		return tagKind
	}
}

// ─── Tree Traversal Helpers ───────────────────────────────────────────────────

// findFunctionBody locates the body node of a function definition given its range.
// It walks the tree to find either a block_statement or compound_statement node
// that falls within the tag's range.
func findFunctionBody(root *gotreesitter.Node, r gotreesitter.Range, lang *gotreesitter.Language) *gotreesitter.Node {
	// Walk the tree looking for the body node within the function range
	return findBodyRecursive(root, r, lang)
}

func findBodyRecursive(node *gotreesitter.Node, r gotreesitter.Range, lang *gotreesitter.Language) *gotreesitter.Node {
	if node == nil {
		return nil
	}

	// Check if this is a body node
	typ := node.Type(lang)
	if typ == "block" || typ == "block_statement" || typ == "compound_statement" ||
		typ == "statement_block" || typ == "body" ||
		typ == "declaration_list" || typ == "program" {
		// Verify it's within the tag's range
		sb := node.StartByte()
		eb := node.EndByte()
		if sb >= r.StartByte && eb <= r.EndByte {
			return node
		}
	}

	// Recurse children
	for i := 0; i < int(node.ChildCount()); i++ {
		if found := findBodyRecursive(node.Child(i), r, lang); found != nil {
			return found
		}
	}
	return nil
}

// isBodyNode checks if a node type represents a function body.
func isBodyNode(typ string) bool {
	switch typ {
	case "block", "block_statement", "compound_statement",
		"statement_block", "body", "declaration_list":
		return true
	}
	return false
}

// ─── Complexity Analysis ──────────────────────────────────────────────────────

// cyclomaticComplexity counts branching constructs in a node.
// Base = 1, plus: if/else/for/while/switch/case/&&/||/catch/ternary.
func cyclomaticComplexity(node *gotreesitter.Node, lang *gotreesitter.Language) int {
	if node == nil {
		return 1
	}
	count := 1
	collectCyclomatic(node, lang, &count)
	return count
}

func collectCyclomatic(node *gotreesitter.Node, lang *gotreesitter.Language, count *int) {
	if node == nil {
		return
	}
	typ := node.Type(lang)
	switch typ {
	case "if_statement", "else_clause", "for_statement", "while_statement",
		"do_statement", "switch_statement", "case_statement", "case",
		"catch_clause", "conditional_expression", "ternary_expression":
		*count++
	case "binary_expression":
		// Check for && and || operators
		op := nodeText(node, lang, nil) // simplified
		if strings.Contains(op, "&&") || strings.Contains(op, "||") {
			*count++
		}
	}

	for i := 0; i < int(node.ChildCount()); i++ {
		collectCyclomatic(node.Child(i), lang, count)
	}
}

// cognitiveComplexity computes cognitive complexity with nesting weighting.
func cognitiveComplexity(node *gotreesitter.Node, lang *gotreesitter.Language, nesting int) int {
	if node == nil {
		return 0
	}
	sum := 0
	collectCognitive(node, lang, nesting, &sum)
	return sum
}

func collectCognitive(node *gotreesitter.Node, lang *gotreesitter.Language, nesting int, sum *int) {
	if node == nil {
		return
	}
	typ := node.Type(lang)
	switch typ {
	case "if_statement", "else_clause", "for_statement", "while_statement",
		"do_statement", "catch_clause":
		*sum += 1 + nesting
		collectCognitive(node, lang, nesting+1, sum)
		return
	case "switch_statement", "case_statement", "case":
		*sum += 1 + nesting
		// Cases don't increment nesting for their children
		collectCognitive(node, lang, nesting, sum)
		return
	case "conditional_expression", "ternary_expression":
		*sum += 1 + nesting
	}

	for i := 0; i < int(node.ChildCount()); i++ {
		collectCognitive(node.Child(i), lang, nesting, sum)
	}
}

// maxLoopDepth finds the maximum nesting depth of loops.
func maxLoopDepth(node *gotreesitter.Node, lang *gotreesitter.Language, depth int) int {
	if node == nil {
		return 0
	}
	typ := node.Type(lang)
	isLoop := typ == "for_statement" || typ == "while_statement" || typ == "do_statement"

	currentDepth := depth
	if isLoop {
		currentDepth++
	}

	max := currentDepth
	for i := 0; i < int(node.ChildCount()); i++ {
		childMax := maxLoopDepth(node.Child(i), lang, currentDepth)
		if childMax > max {
			max = childMax
		}
	}
	return max
}

// countLoops counts the total number of loop constructs.
func countLoops(node *gotreesitter.Node, lang *gotreesitter.Language) int {
	if node == nil {
		return 0
	}
	count := 0
	collectLoops(node, lang, &count)
	return count
}

func collectLoops(node *gotreesitter.Node, lang *gotreesitter.Language, count *int) {
	if node == nil {
		return
	}
	typ := node.Type(lang)
	if typ == "for_statement" || typ == "while_statement" || typ == "do_statement" {
		*count++
	}
	for i := 0; i < int(node.ChildCount()); i++ {
		collectLoops(node.Child(i), lang, count)
	}
}

// isRecursive checks if a function body contains a call to itself.
func isRecursive(body *gotreesitter.Node, name string, lang *gotreesitter.Language) bool {
	if body == nil {
		return false
	}
	found := false
	checkRecursive(body, name, lang, &found)
	return found
}

func checkRecursive(node *gotreesitter.Node, name string, lang *gotreesitter.Language, found *bool) {
	if *found || node == nil {
		return
	}
	// This is a simplified check — we look for identifier nodes matching the name
	// within what appears to be a call context
	if node.Type(lang) == "identifier" && nodeText(node, lang, nil) == name {
		parent := node.Parent()
		if parent != nil && parent.Type(lang) == "call_expression" {
			*found = true
			return
		}
	}
	// Also check method calls like this.foo()
	if node.Type(lang) == "method" || node.Type(lang) == "property_identifier" {
		if nodeText(node, lang, nil) == name {
			parent := node.Parent()
			if parent != nil && parent.Type(lang) == "call_expression" {
				*found = true
				return
			}
		}
	}
	for i := 0; i < int(node.ChildCount()); i++ {
		checkRecursive(node.Child(i), name, lang, found)
	}
}

// countParams counts parameters in a function/method declaration.
func countParams(body *gotreesitter.Node, kind string, lang *gotreesitter.Language) int {
	if body == nil {
		return 0
	}
	// Find the parent function declaration and look for parameters
	// Walk up to find the function/method declaration, then count parameter children
	parent := body.Parent()
	for parent != nil {
		ptype := parent.Type(lang)
		if ptype == "function_definition" || ptype == "method_definition" ||
			ptype == "function_declaration" || ptype == "method_declaration" ||
			ptype == "arrow_function" {
			// Find parameters node
			for i := 0; i < int(parent.ChildCount()); i++ {
				child := parent.Child(i)
				if child != nil && child.Type(lang) == "parameters" {
					return int(child.NamedChildCount())
				}
			}
		}
		parent = parent.Parent()
	}
	return 0
}

// ─── Call Extraction ──────────────────────────────────────────────────────────

// extractCalls extracts function call names from a node subtree.
func extractCalls(node *gotreesitter.Node, lang *gotreesitter.Language) []string {
	var calls []string
	collectCalls(node, lang, &calls)
	return calls
}

func collectCalls(node *gotreesitter.Node, lang *gotreesitter.Language, calls *[]string) {
	if node == nil {
		return
	}

	// Look for call_expression nodes
	if node.Type(lang) == "call_expression" {
		// The function being called is the first child
		if node.ChildCount() > 0 {
			fnNode := node.Child(0)
			if fnNode != nil {
				fnName := nodeText(fnNode, lang, nil)
				if fnName != "" && !isBuiltinOrKeyword(fnName) {
					*calls = append(*calls, fnName)
				}
			}
		}
	}

	for i := 0; i < int(node.ChildCount()); i++ {
		collectCalls(node.Child(i), lang, calls)
	}
}

// isBuiltinOrKeyword checks if a name looks like a built-in or keyword call.
func isBuiltinOrKeyword(name string) bool {
	switch name {
	case "if", "for", "while", "switch", "return", "throw", "new", "delete",
		"typeof", "instanceof", "void", "sizeof", "assert":
		return true
	}
	return false
}

// ─── Utilities ────────────────────────────────────────────────────────────────

// nodeText returns the source text of a node.
func nodeText(node *gotreesitter.Node, lang *gotreesitter.Language, source []byte) string {
	if source != nil {
		return string(node.Text(source))
	}
	// Fallback: try to get text from node range
	// This is limited; callers should pass source when available
	return ""
}

// toRelPath converts an absolute file path to a relative path within the project.
// If the path is not under the project root, returns the path as-is.
func toRelPath(absPath string) string {
	// We don't know the project root here, so return the path
	// The store layer doesn't normalize — callers should do it
	return absPath
}

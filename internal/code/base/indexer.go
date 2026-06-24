// Package base implements SQLite-backed code graph storage and project indexing.
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
func (s *Store) IndexProject(root, name string, mode IndexMode) (*IndexResult, error) {
	// 1. Create or get project
	projectID, err := s.UpsertProject(name, root)
	if err != nil {
		return nil, fmt.Errorf("upsert project: %w", err)
	}
	result := &IndexResult{
		ProjectName: name,
		ProjectRoot: root,
		Mode:        mode,
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
		if !supportedExts[ext] {
			return nil
		}

		// Mode-based filtering
		if mode != IndexModeFull {
			// Skip large files (>1MB) in non-full modes
			if fi.Size() > 1<<20 {
				return nil
			}
			// Skip generated files in non-full modes
			if isGeneratedFile(path) {
				return nil
			}
			// Skip test files in fast mode
			if mode == IndexModeFast && isTestFile(path) {
				return nil
			}
		}

		files = append(files, path)
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

// indexFile processes a single file: parse → extract metadata → build graph.
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
	root := tree.RootNode()
	baseName := filepath.Base(relPath)

	// ── Extract file-level metadata ──
	meta := extractFileMeta(root, lang, source)

	// ── Build nodes ──
	var nodes []Node

	// 1. File node
	fileQN := relPath
	nodes = append(nodes, Node{
		ProjectID:     projectID,
		QualifiedName: fileQN,
		Kind:          "file",
		Name:          baseName,
		FilePath:      relPath,
	})

	// 2. Package / Module node (if package name detected)
	var packageQN string
	if meta.packageName != "" {
		packageQN = "package:" + meta.packageName
		nodes = append(nodes, Node{
			ProjectID:     projectID,
			QualifiedName: packageQN,
			Kind:          "package",
			Name:          meta.packageName,
			FilePath:      relPath,
		})
	}

	// 3. Extract definitions via Tagger API
	tags := extractTags(tree, entry, lang)
	var defQNs []string
	for _, tag := range tags {
		qn := tag.Name
		defQNs = append(defQNs, qn)
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
			bodyNode := findFunctionBody(root, tag.Range, lang)
			if bodyNode != nil {
				n.Complexity = cyclomaticComplexity(bodyNode, lang, source)
				n.Cognitive = cognitiveComplexity(bodyNode, lang, 0)
				n.LoopDepth = maxLoopDepth(bodyNode, lang, 0)
				n.LoopCount = countLoops(bodyNode, lang)
				n.Recursive = isRecursive(bodyNode, tag.Name, lang, source)
				n.ParamCount = countParams(bodyNode, tag.Kind, lang)
				n.LinearScanInLoop = linearScanInLoop(bodyNode, lang, source, false)
			}
		}

		nodes = append(nodes, n)
	}

	// 4. Extract variable/constant definitions via manual AST walk
	//    The Tagger API (go-specific override) doesn't emit definition.variable
	//    or definition.constant tags, so we handle them here.
	varConstNodes := extractVarConstDefs(root, lang, source, projectID, relPath)
	for _, vc := range varConstNodes {
		defQNs = append(defQNs, vc.QualifiedName)
	}
	nodes = append(nodes, varConstNodes...)
	varNameMap := buildVarNameMap(varConstNodes)

	// ── Build edges ──
	var edges []Edge

	// 1. DEFINES edges: file → each definition in this file
	for _, defQN := range defQNs {
		edges = append(edges, Edge{
			ProjectID: projectID,
			SourceQN:  fileQN,
			TargetQN:  defQN,
			EdgeType:  "DEFINES",
		})
	}

	// 2. CONTAINS edges: package → file
	if packageQN != "" {
		edges = append(edges, Edge{
			ProjectID: projectID,
			SourceQN:  packageQN,
			TargetQN:  fileQN,
			EdgeType:  "CONTAINS",
		})
	}

	// 3. IMPORTS edges: file → imported package (using import path as QN)
	for _, imp := range meta.importPaths {
		impQN := "import:" + imp
		edges = append(edges, Edge{
			ProjectID: projectID,
			SourceQN:  fileQN,
			TargetQN:  impQN,
			EdgeType:  "IMPORTS",
		})
	}

	// 4. INHERITS edges: struct → embedded type, class → superclass
	for childType, parentTypes := range meta.inherits {
		for _, parentType := range parentTypes {
			edges = append(edges, Edge{
				ProjectID: projectID,
				SourceQN:  childType,
				TargetQN:  parentType,
				EdgeType:  "IMPLEMENTS",
			})
		}
	}

	// 5. CONTAINS edges from struct/class → method
	for methodName, parentStruct := range meta.methodParents {
		edges = append(edges, Edge{
			ProjectID: projectID,
			SourceQN:  parentStruct,
			TargetQN:  methodName,
			EdgeType:  "CONTAINS",
		})
	}

	// 6. CALLS edges: extracted from function/method bodies
	for _, tag := range tags {
		if tag.Kind == "definition.function" || tag.Kind == "definition.method" {
			bodyNode := findFunctionBody(root, tag.Range, lang)
			if bodyNode != nil {
				calls := extractCalls(bodyNode, lang, source)
				for _, callee := range calls {
					edges = append(edges, Edge{
						ProjectID: projectID,
						SourceQN:  tag.Name,
						TargetQN:  callee,
						EdgeType:  "CALLS",
					})
				}
				// Also extract variable/constant REFERENCES from this function body
				if len(varNameMap) > 0 {
					refs := extractVarRefs(bodyNode, lang, source, tag.Name, varNameMap, projectID)
					edges = append(edges, refs...)
				}
			}
		}
	}

	return nodes, edges, nil
}

// ─── File Metadata Extraction ─────────────────────────────────────────────────

// fileMeta holds per-file metadata extracted from the AST.
type fileMeta struct {
	packageName   string              // package/module name (e.g. "main", "fmt")
	importPaths   []string            // import paths (e.g. "fmt", "strings")
	methodParents map[string]string   // method QN → parent struct QN
	inherits      map[string][]string // type QN → list of embedded/extends QNs
}

// extractFileMeta extracts package name, imports, and structural relationships.
func extractFileMeta(root *gotreesitter.Node, lang *gotreesitter.Language, source []byte) fileMeta {
	meta := fileMeta{
		methodParents: make(map[string]string),
		inherits:      make(map[string][]string),
	}

	var walk func(node *gotreesitter.Node)
	walk = func(node *gotreesitter.Node) {
		if node == nil {
			return
		}
		typ := node.Type(lang)

		switch {
		case typ == "package_clause":
			// Go: package <identifier>
			for i := 0; i < int(node.ChildCount()); i++ {
				child := node.Child(i)
				if child != nil && (child.Type(lang) == "identifier" || child.Type(lang) == "package_identifier") {
					meta.packageName = nodeText(child, lang, source)
				}
			}

		case typ == "module_declaration" || typ == "module":
			// Python/Ruby/Elixir module declaration
			for i := 0; i < int(node.ChildCount()); i++ {
				child := node.Child(i)
				if child != nil && (child.Type(lang) == "identifier" || child.Type(lang) == "constant") {
					meta.packageName = nodeText(child, lang, source)
				}
			}

		case typ == "package_declaration":
			// Java/Kotlin package
			for i := 0; i < int(node.ChildCount()); i++ {
				child := node.Child(i)
				if child == nil {
					continue
				}
				ct := child.Type(lang)
				if ct == "identifier" && meta.packageName == "" {
					meta.packageName = nodeText(child, lang, source)
				}
				if ct == "scoped_identifier" {
					meta.packageName = nodeText(child, lang, source)
				}
			}

		case typ == "import_declaration":
			// Go: import ( "fmt" "strings" )
			for i := 0; i < int(node.ChildCount()); i++ {
				child := node.Child(i)
				if child != nil && child.Type(lang) == "import_spec" {
					extractImportSpec(child, lang, source, &meta)
				}
			}

		case typ == "import_statement":
			// JS/TS/Python/Java import statement
			for i := 0; i < int(node.ChildCount()); i++ {
				child := node.Child(i)
				if child != nil {
					extractImportPaths(child, lang, source, &meta)
				}
			}

		case typ == "method_declaration":
			// Go: func (r *Receiver) Name(...) { ... }
			extractGoMethodParent(node, lang, source, &meta)

		case typ == "type_declaration":
			// Go: type Foo struct { ... }
			extractGoTypeInfo(node, lang, source, &meta)

		case typ == "class_declaration" || typ == "class_definition":
			// Java/TypeScript/Python class
			extractClassParent(node, lang, source, &meta)
		}
		for i := 0; i < int(node.ChildCount()); i++ {
			walk(node.Child(i))
		}
	}
	walk(root)
	return meta
}

// extractImportSpec extracts import path from a Go import_spec.
func extractImportSpec(node *gotreesitter.Node, lang *gotreesitter.Language, source []byte, meta *fileMeta) {
	for i := 0; i < int(node.ChildCount()); i++ {
		child := node.Child(i)
		if child == nil {
			continue
		}
		ct := child.Type(lang)
		if ct == "interpreted_string_literal" || ct == "string" {
			path := nodeText(child, lang, source)
			path = strings.Trim(path, "\"'")
			if path != "" {
				meta.importPaths = append(meta.importPaths, path)
			}
		}
		// Aliased import: import alias "path" — the string is a sibling
		if ct == "identifier" {
			for j := i + 1; j < int(node.ChildCount()); j++ {
				sib := node.Child(j)
				if sib != nil && (sib.Type(lang) == "interpreted_string_literal" || sib.Type(lang) == "string") {
					path := nodeText(sib, lang, source)
					path = strings.Trim(path, "\"'")
					if path != "" {
						meta.importPaths = append(meta.importPaths, path)
					}
				}
			}
		}
	}
}

// extractImportPaths attempts to find import path strings from generic nodes.
func extractImportPaths(node *gotreesitter.Node, lang *gotreesitter.Language, source []byte, meta *fileMeta) {
	typ := node.Type(lang)
	switch typ {
	case "string", "interpreted_string_literal", "string_literal":
		path := nodeText(node, lang, source)
		path = strings.Trim(path, "\"'")
		if path != "" {
			meta.importPaths = append(meta.importPaths, path)
		}
	case "import_spec", "import_clause":
		for i := 0; i < int(node.ChildCount()); i++ {
			extractImportPaths(node.Child(i), lang, source, meta)
		}
	}
}

// extractGoMethodParent resolves the parent struct for a Go method.
func extractGoMethodParent(node *gotreesitter.Node, lang *gotreesitter.Language, source []byte, meta *fileMeta) {
	var methodName, receiverType string
	for i := 0; i < int(node.ChildCount()); i++ {
		child := node.Child(i)
		if child == nil {
			continue
		}
		ct := child.Type(lang)
		if ct == "receiver" {
			receiverType = findReceiverType(child, lang, source)
		}
		if ct == "identifier" && methodName == "" {
			methodName = nodeText(child, lang, source)
		}
		if ct == "field_identifier" && methodName == "" {
			methodName = nodeText(child, lang, source)
		}
	}
	if methodName != "" && receiverType != "" {
		meta.methodParents[methodName] = receiverType
	}
}

// findReceiverType walks a receiver node to find the type name.
func findReceiverType(node *gotreesitter.Node, lang *gotreesitter.Language, source []byte) string {
	for i := 0; i < int(node.ChildCount()); i++ {
		child := node.Child(i)
		if child == nil {
			continue
		}
		ct := child.Type(lang)
		switch ct {
		case "type_identifier", "identifier":
			return nodeText(child, lang, source)
		case "parameter_list", "parameter_declaration":
			return findReceiverType(child, lang, source)
		case "pointer_type":
			for j := 0; j < int(child.ChildCount()); j++ {
				gc := child.Child(j)
				if gc != nil && (gc.Type(lang) == "type_identifier" || gc.Type(lang) == "identifier") {
					return nodeText(gc, lang, source)
				}
			}
		}
	}
	return ""
}

// extractGoTypeInfo extracts struct embedding and interface info from Go type declarations.
func extractGoTypeInfo(node *gotreesitter.Node, lang *gotreesitter.Language, source []byte, meta *fileMeta) {
	for i := 0; i < int(node.ChildCount()); i++ {
		child := node.Child(i)
		if child == nil || child.Type(lang) != "type_spec" {
			continue
		}
		var typeName string
		var embedded []string
		for j := 0; j < int(child.ChildCount()); j++ {
			inner := child.Child(j)
			if inner == nil {
				continue
			}
			it := inner.Type(lang)
			if it == "type_identifier" && typeName == "" {
				typeName = nodeText(inner, lang, source)
			}
			if it == "struct_type" {
				embedded = append(embedded, findEmbeddedTypes(inner, lang, source)...)
			}
		}
		if typeName != "" && len(embedded) > 0 {
			meta.inherits[typeName] = embedded
		}
	}
}

// findEmbeddedTypes walks a struct body to find embedded field type names.
func findEmbeddedTypes(node *gotreesitter.Node, lang *gotreesitter.Language, source []byte) []string {
	var types []string
	var walk func(n *gotreesitter.Node)
	walk = func(n *gotreesitter.Node) {
		if n == nil {
			return
		}
		nt := n.Type(lang)
		if nt == "field_declaration" {
			hasFieldName := false
			var typeID string
			for i := 0; i < int(n.ChildCount()); i++ {
				c := n.Child(i)
				if c == nil {
					continue
				}
				ct := c.Type(lang)
				if ct == "identifier" || ct == "field_identifier" {
					hasFieldName = true
				}
				if ct == "type_identifier" {
					typeID = nodeText(c, lang, source)
				}
			}
			if !hasFieldName && typeID != "" {
				types = append(types, typeID)
			}
			return
		}
		if nt == "extends_clause" || nt == "implements_clause" {
			for i := 0; i < int(n.ChildCount()); i++ {
				c := n.Child(i)
				if c != nil && (c.Type(lang) == "type_identifier" || c.Type(lang) == "identifier" || c.Type(lang) == "scoped_type_identifier") {
					types = append(types, nodeText(c, lang, source))
				}
			}
			return
		}
		for i := 0; i < int(n.ChildCount()); i++ {
			walk(n.Child(i))
		}
	}
	walk(node)
	return types
}

// extractClassParent extracts parent class/interface from class declarations.
func extractClassParent(node *gotreesitter.Node, lang *gotreesitter.Language, source []byte, meta *fileMeta) {
	var className string
	var inheritsFrom []string
	for i := 0; i < int(node.ChildCount()); i++ {
		child := node.Child(i)
		if child == nil {
			continue
		}
		ct := child.Type(lang)
		if ct == "identifier" && className == "" {
			className = nodeText(child, lang, source)
		}
		if ct == "name" {
			for j := 0; j < int(child.ChildCount()); j++ {
				nc := child.Child(j)
				if nc != nil && nc.Type(lang) == "identifier" && className == "" {
					className = nodeText(nc, lang, source)
				}
			}
		}
		if ct == "extends_clause" || ct == "implements_clause" || ct == "superclass" || ct == "base_class" {
			inheritsFrom = append(inheritsFrom, findClassRefs(child, lang, source)...)
		}
	}
	if className != "" && len(inheritsFrom) > 0 {
		meta.inherits[className] = inheritsFrom
	}
}

// findClassRefs extracts type references from extends/implements clauses.
func findClassRefs(node *gotreesitter.Node, lang *gotreesitter.Language, source []byte) []string {
	var refs []string
	for i := 0; i < int(node.ChildCount()); i++ {
		c := node.Child(i)
		if c == nil {
			continue
		}
		ct := c.Type(lang)
		if ct == "identifier" || ct == "type_identifier" || ct == "scoped_identifier" || ct == "scoped_type_identifier" {
			refs = append(refs, nodeText(c, lang, source))
		}
	}
	return refs
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

// ─── Variable/Constant Definition Extraction ─────────────────────────────────
//
// extractVarConstDefs walks the AST to find variable and constant declarations.
// The Tagger API only emits definition.function/method for Go, so we need
// manual traversal for var/const/short_var_declaration patterns.
//
// Returns a list of Nodes with kind "variable" or "constant".
// Qualified names use "variable:<name>" or "constant:<name>" prefix to avoid
// collision with function/method nodes of the same name.
func extractVarConstDefs(root *gotreesitter.Node, lang *gotreesitter.Language, source []byte, projectID int64, relPath string) []Node {
	var nodes []Node

	var walk func(node *gotreesitter.Node)
	walk = func(node *gotreesitter.Node) {
		if node == nil {
			return
		}
		typ := node.Type(lang)

		switch {
		case typ == "var_spec" || typ == "const_spec":
			kind := "variable"
			if typ == "const_spec" {
				kind = "constant"
			}
			// var_spec: var x int = 5  → children: identifier(s), type, value
			// const_spec: const x = 5  → children: identifier(s), value
			for i := 0; i < int(node.ChildCount()); i++ {
				child := node.Child(i)
				if child == nil {
					continue
				}
				ct := child.Type(lang)
				if ct == "identifier" || ct == "field_identifier" {
					name := nodeText(child, lang, source)
					if name == "" {
						continue
					}
					qn := kind + ":" + name
					nodes = append(nodes, Node{
						ProjectID:     projectID,
						QualifiedName: qn,
						Kind:          kind,
						Name:          name,
						FilePath:      relPath,
						Line:          child.StartPoint().Row,
						Col:           child.StartPoint().Column,
						EndLine:       child.EndPoint().Row,
						EndCol:        child.EndPoint().Column,
					})
				}
				// Handles "var a, b int" — multiple identifiers in identifier_list
				if ct == "identifier_list" {
					for j := 0; j < int(child.ChildCount()); j++ {
						gc := child.Child(j)
						if gc != nil && gc.Type(lang) == "identifier" {
							name := nodeText(gc, lang, source)
							if name == "" {
								continue
							}
							qn := kind + ":" + name
							nodes = append(nodes, Node{
								ProjectID:     projectID,
								QualifiedName: qn,
								Kind:          kind,
								Name:          name,
								FilePath:      relPath,
								Line:          gc.StartPoint().Row,
								Col:           gc.StartPoint().Column,
								EndLine:       gc.EndPoint().Row,
								EndCol:        gc.EndPoint().Column,
							})
						}
					}
				}
			}

		case typ == "short_var_declaration":
			// x := 5  → children: identifier(s), ":=", expression
			for i := 0; i < int(node.ChildCount()); i++ {
				child := node.Child(i)
				if child == nil {
					continue
				}
				ct := child.Type(lang)
				if ct == "identifier" || ct == "field_identifier" {
					name := nodeText(child, lang, source)
					if name == "" {
						continue
					}
					qn := "variable:" + name
					nodes = append(nodes, Node{
						ProjectID:     projectID,
						QualifiedName: qn,
						Kind:          "variable",
						Name:          name,
						FilePath:      relPath,
						Line:          child.StartPoint().Row,
						Col:           child.StartPoint().Column,
						EndLine:       child.EndPoint().Row,
						EndCol:        child.EndPoint().Column,
					})
				}
			}
		}

		// Recurse children — skip function/method bodies to avoid detecting
		// local variables inside functions.
		if typ != "block" && typ != "block_statement" && typ != "compound_statement" &&
			typ != "statement_block" && typ != "body" {
			for i := 0; i < int(node.ChildCount()); i++ {
				walk(node.Child(i))
			}
		}
	}
	walk(root)
	return nodes
}

// ─── Variable/Constant Reference Extraction ──────────────────────────────────
//
// extractVarRefs walks a function/method body looking for identifier references
// that match known variable/constant names. Returns REFERENCES edges from the
// containing function to the referenced variable/constant.
//
// Heuristic approach (no type resolution):
//   - Identifiers that are call targets are skipped (those produce CALLS edges)
//   - Only identifiers matching known var/const names in the same file match
//   - Duplicate references to the same variable from the same function are
//     collapsed into a single REFERENCES edge
func extractVarRefs(body *gotreesitter.Node, lang *gotreesitter.Language, source []byte, funcQN string, varNames map[string]string, projectID int64) []Edge {
	var edges []Edge
	seen := make(map[string]bool)

	var walk func(node *gotreesitter.Node)
	walk = func(node *gotreesitter.Node) {
		if node == nil {
			return
		}
		typ := node.Type(lang)

		// Skip call expressions — the function name is a CALLS target, not REFERENCES
		if typ == "call_expression" {
			// Recurse into arguments (which may reference variables)
			for i := 1; i < int(node.ChildCount()); i++ {
				walk(node.Child(i))
			}
			return
		}

		// In selector_expression (e.g., obj.field), only the object may be a var ref
		if typ == "selector_expression" {
			if node.ChildCount() > 0 {
				walk(node.Child(0))
			}
			return
		}

		if typ == "identifier" || typ == "field_identifier" {
			name := nodeText(node, lang, source)
			if name == "" {
				return
			}
			if targetQN, ok := varNames[name]; ok && !seen[targetQN] {
				seen[targetQN] = true
				edges = append(edges, Edge{
					ProjectID: projectID,
					SourceQN:  funcQN,
					TargetQN:  targetQN,
					EdgeType:  "REFERENCES",
				})
			}
			return
		}

		for i := 0; i < int(node.ChildCount()); i++ {
			walk(node.Child(i))
		}
	}
	walk(body)
	return edges
}

// buildVarNameMap builds a map from short name → qualified name for all
// variable and constant nodes in a list.
func buildVarNameMap(nodes []Node) map[string]string {
	m := make(map[string]string)
	for _, n := range nodes {
		if n.Kind == "variable" || n.Kind == "constant" {
			if _, ok := m[n.Name]; !ok {
				m[n.Name] = n.QualifiedName
			}
		}
	}
	return m
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
func cyclomaticComplexity(node *gotreesitter.Node, lang *gotreesitter.Language, source []byte) int {
	if node == nil {
		return 1
	}
	count := 1
	collectCyclomatic(node, lang, source, &count)
	return count
}

func collectCyclomatic(node *gotreesitter.Node, lang *gotreesitter.Language, source []byte, count *int) {
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
		op := nodeText(node, lang, source) // simplified
		if strings.Contains(op, "&&") || strings.Contains(op, "||") {
			*count++
		}
	}

	for i := 0; i < int(node.ChildCount()); i++ {
		collectCyclomatic(node.Child(i), lang, source, count)
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
		for i := 0; i < int(node.ChildCount()); i++ {
			collectCognitive(node.Child(i), lang, nesting+1, sum)
		}
		return
	case "switch_statement", "case_statement", "case":
		*sum += 1 + nesting
		// Cases don't increment nesting for their children
		for i := 0; i < int(node.ChildCount()); i++ {
			collectCognitive(node.Child(i), lang, nesting, sum)
		}
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
func isRecursive(body *gotreesitter.Node, name string, lang *gotreesitter.Language, source []byte) bool {
	if body == nil {
		return false
	}
	found := false
	checkRecursive(body, name, lang, source, &found)
	return found
}

func checkRecursive(node *gotreesitter.Node, name string, lang *gotreesitter.Language, source []byte, found *bool) {
	if *found || node == nil {
		return
	}
	// This is a simplified check — we look for identifier nodes matching the name
	// within what appears to be a call context
	if node.Type(lang) == "identifier" && nodeText(node, lang, source) == name {
		parent := node.Parent()
		if parent != nil && parent.Type(lang) == "call_expression" {
			*found = true
			return
		}
	}
	// Also check method calls like this.foo()
	if node.Type(lang) == "method" || node.Type(lang) == "property_identifier" || node.Type(lang) == "field_identifier" {
		if nodeText(node, lang, source) == name {
			p := node.Parent()
			// In Go, r.self() has selector_expression between field_identifier and call_expression
			if p != nil && p.Type(lang) == "selector_expression" {
				p = p.Parent()
			}
			if p != nil && p.Type(lang) == "call_expression" {
				*found = true
				return
			}
		}
	}
	for i := 0; i < int(node.ChildCount()); i++ {
		checkRecursive(node.Child(i), name, lang, source, found)
	}
}

// countParams counts parameters in a function/method declaration.
func countParams(body *gotreesitter.Node, kind string, lang *gotreesitter.Language) int {
	if body == nil {
		return 0
	}
	// Find the parent function declaration and look for parameters
	parent := body.Parent()
	for parent != nil {
		ptype := parent.Type(lang)
		if ptype == "function_definition" || ptype == "method_definition" ||
			ptype == "function_declaration" || ptype == "method_declaration" ||
			ptype == "arrow_function" {
			// Find parameters node
			for i := 0; i < int(parent.ChildCount()); i++ {
				child := parent.Child(i)
				if child != nil && (child.Type(lang) == "parameters" || child.Type(lang) == "parameter_list") {
					// Count parameter_declaration children; each may contain multiple
					// identifiers (e.g. "a, b int" in Go is a single declaration).
					params := 0
					for j := 0; j < int(child.ChildCount()); j++ {
						gc := child.Child(j)
						if gc != nil && gc.Type(lang) == "parameter_declaration" {
							idCount := 0
							for k := 0; k < int(gc.ChildCount()); k++ {
								if gc.Child(k) != nil && gc.Child(k).Type(lang) == "identifier" {
									idCount++
								}
							}
							if idCount > 0 {
								params += idCount
							} else {
								params++ // at least one parameter
							}
						}
					}
					return params
				}
			}
		}
		parent = parent.Parent()
	}
	return 0
}

// ─── Call Extraction ──────────────────────────────────────────────────────────

// extractCalls extracts function call names from a node subtree.
func extractCalls(node *gotreesitter.Node, lang *gotreesitter.Language, source []byte) []string {
	var calls []string
	collectCalls(node, lang, source, &calls)
	return calls
}

func collectCalls(node *gotreesitter.Node, lang *gotreesitter.Language, source []byte, calls *[]string) {
	if node == nil {
		return
	}

	// Look for call_expression nodes
	if node.Type(lang) == "call_expression" {
		// The function being called is the first child
		if node.ChildCount() > 0 {
			fnNode := node.Child(0)
			if fnNode != nil {
				fnName := nodeText(fnNode, lang, source)
				if fnName != "" && !isBuiltinOrKeyword(fnName) {
					*calls = append(*calls, fnName)
				}
			}
		}
	}

	for i := 0; i < int(node.ChildCount()); i++ {
		collectCalls(node.Child(i), lang, source, calls)
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

// linearScanInLoop counts linear scan operations (find, contains, indexOf, etc.)
// that occur inside loop bodies — these are hidden O(n^2) signals.
// Walks the body node looking for call expressions inside loop constructs.
func linearScanInLoop(node *gotreesitter.Node, lang *gotreesitter.Language, source []byte, inLoop bool) int {
	if node == nil {
		return 0
	}
	typ := node.Type(lang)
	isLoop := typ == "for_statement" || typ == "while_statement" || typ == "do_statement"

	currentInLoop := inLoop || isLoop

	count := 0
	if currentInLoop && typ == "call_expression" {
		// Get function name
		if node.ChildCount() > 0 {
			fnNode := node.Child(0)
			if fnNode != nil {
				name := nodeText(fnNode, lang, source)
				if isLinearScanName(name) {
					count++
				}
			}
		}
	}

	for i := 0; i < int(node.ChildCount()); i++ {
		count += linearScanInLoop(node.Child(i), lang, source, currentInLoop)
	}
	return count
}

var linearScanNames = map[string]bool{
	"find": true, "findFirst": true, "findLast": true, "findAny": true,
	"contains": true, "indexOf": true, "lastIndexOf": true,
	"includes": true, "search": true, "searchRange": true,
	"match": true, "matchAll": true,
	"Count": true, "Any": true, "All": true,
	"Filter": true, "Where": true, "First": true, "FirstOrDefault": true,
	"Single": true, "SingleOrDefault": true,
}

func isLinearScanName(name string) bool {
	// Handle method calls like .Find, .Contains, strings.Contains
	parts := strings.Split(name, ".")
	if len(parts) > 0 {
		name = parts[len(parts)-1]
	}
	return linearScanNames[name]
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

// isGeneratedFile returns true if the file appears to be auto-generated.
func isGeneratedFile(path string) bool {
	name := strings.ToLower(filepath.Base(path))
	if strings.HasSuffix(name, ".pb.go") || strings.HasSuffix(name, ".pb.swift") || strings.HasSuffix(name, "_pb.js") {
		return true
	}
	if strings.Contains(name, "_generated.") || strings.Contains(name, ".generated.") {
		return true
	}
	if strings.HasSuffix(name, "_mock.go") || strings.HasSuffix(name, "_testify.go") {
		return true
	}
	return false
}

// isTestFile returns true if the file appears to be a test file.
func isTestFile(path string) bool {
	name := filepath.Base(path)
	ext := strings.ToLower(filepath.Ext(name))
	baseName := strings.TrimSuffix(name, ext)
	switch ext {
	case ".go":
		return strings.HasSuffix(baseName, "_test")
	case ".js", ".ts", ".jsx", ".tsx":
		return strings.HasSuffix(baseName, ".test") || strings.HasSuffix(baseName, ".spec")
	case ".py":
		return strings.HasPrefix(baseName, "test_") || strings.HasSuffix(baseName, "_test")
	case ".rs":
		return strings.HasSuffix(baseName, "_test")
	case ".java":
		return strings.HasSuffix(baseName, "Test") || strings.HasSuffix(baseName, "Tests")
	case ".kt":
		return strings.HasSuffix(baseName, "Test") || strings.HasSuffix(baseName, "Spec")
	default:
		return false
	}
}

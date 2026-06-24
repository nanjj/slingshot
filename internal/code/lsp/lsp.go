package lsp

import (
	"slices"
	"fmt"
	"os"
	"strings"

	"github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
)

// ─── Parsing ──────────────────────────────────────────────────────────────────

// ParseFile parses a source file and returns the result.
// The caller must call result.Tree.Release() when done.
func (a *Analyzer) ParseFile(filePath string) (*ParseResult, error) {
	source, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("read file: %w", err)
	}

	// Detect language from file path
	entry := grammars.DetectLanguage(filePath)
	if entry == nil {
		return nil, fmt.Errorf("unsupported language: %s", filePath)
	}

	lang := entry.Language()
	if lang == nil {
		return nil, fmt.Errorf("no language object for %s", filePath)
	}

	parser := gotreesitter.NewParser(lang)
	tree, err := parser.Parse(source)
	if err != nil {
		return nil, fmt.Errorf("parse: %w", err)
	}

	return &ParseResult{
		Language: entry.Name,
		Source:   source,
		Tree:     tree,
	}, nil
}

// ParseBytes parses source bytes with a given language name.
// The caller must call result.Tree.Release() when done.
// Useful for scratch snippets or in-memory analysis.
func (a *Analyzer) ParseBytes(source []byte, languageName string) (*ParseResult, error) {
	entry := grammars.DetectLanguageByName(languageName)
	if entry == nil {
		return nil, fmt.Errorf("unsupported language: %s", languageName)
	}

	lang := entry.Language()
	parser := gotreesitter.NewParser(lang)
	tree, err := parser.Parse(source)
	if err != nil {
		return nil, fmt.Errorf("parse: %w", err)
	}

	return &ParseResult{
		Language: entry.Name,
		Source:   source,
		Tree:     tree,
	}, nil
}

// ─── Structure ────────────────────────────────────────────────────────────────

// GetStructure returns the hierarchical AST structure of a file.
// maxDepth=-1 means recursive; maxWidth=-1 means unlimited children per node.
func (a *Analyzer) GetStructure(filePath string, maxDepth, maxWidth int) (NodeInfo, error) {
	result, err := a.ParseFile(filePath)
	if err != nil {
		return NodeInfo{}, err
	}
	defer result.Tree.Release()

	root := result.Tree.RootNode()
	if root == nil {
		return NodeInfo{}, fmt.Errorf("empty tree")
	}

	// Use language from the parse result
	entry := grammars.DetectLanguage(filePath)
	lang := getLanguage(entry)

	return buildStructure(root, lang, result.Source, 0, maxDepth, maxWidth), nil
}

// GetStructureFromResult returns the hierarchical AST from an existing ParseResult.
func (a *Analyzer) GetStructureFromResult(result *ParseResult, filePath string, maxDepth, maxWidth int) (NodeInfo, error) {
	root := result.Tree.RootNode()
	if root == nil {
		return NodeInfo{}, fmt.Errorf("empty tree")
	}

	entry := grammars.DetectLanguage(filePath)
	lang := getLanguage(entry)

	return buildStructure(root, lang, result.Source, 0, maxDepth, maxWidth), nil
}

// ─── Node Operations ──────────────────────────────────────────────────────────

// GetNode returns the smallest AST node at the given byte position.
func (a *Analyzer) GetNode(filePath string, pos uint32) (NodeInfo, error) {
	result, err := a.ParseFile(filePath)
	if err != nil {
		return NodeInfo{}, err
	}
	defer result.Tree.Release()

	root := result.Tree.RootNode()
	node := root.DescendantForByteRange(pos, pos)
	if node == nil {
		return NodeInfo{}, fmt.Errorf("node not found at position %d", pos)
	}

	lang := getLanguage(grammars.DetectLanguage(filePath))
	return nodeToInfo(node, lang, result.Source), nil
}

// GetNodeAtPoint returns the smallest AST node at the given (row, col) position.
func (a *Analyzer) GetNodeAtPoint(filePath string, row, col uint32) (NodeInfo, error) {
	result, err := a.ParseFile(filePath)
	if err != nil {
		return NodeInfo{}, err
	}
	defer result.Tree.Release()

	p := gotreesitter.Point{Row: row, Column: col}
	root := result.Tree.RootNode()
	node := root.DescendantForPointRange(p, p)
	if node == nil {
		return NodeInfo{}, fmt.Errorf("node not found at %d:%d", row, col)
	}

	lang := getLanguage(grammars.DetectLanguage(filePath))
	return nodeToInfo(node, lang, result.Source), nil
}

// GetNodeAtRange returns the smallest AST node covering [startByte, endByte).
func (a *Analyzer) GetNodeAtRange(filePath string, startByte, endByte uint32) (NodeInfo, error) {
	result, err := a.ParseFile(filePath)
	if err != nil {
		return NodeInfo{}, err
	}
	defer result.Tree.Release()

	root := result.Tree.RootNode()
	node := root.DescendantForByteRange(startByte, endByte)
	if node == nil {
		return NodeInfo{}, fmt.Errorf("node not found at range [%d, %d)", startByte, endByte)
	}

	lang := getLanguage(grammars.DetectLanguage(filePath))
	return nodeToInfo(node, lang, result.Source), nil
}

// GetDescendantsAt returns all ancestor nodes at a byte position, from innermost to root.
func (a *Analyzer) GetDescendantsAt(filePath string, pos uint32) ([]NodeInfo, error) {
	result, err := a.ParseFile(filePath)
	if err != nil {
		return nil, err
	}
	defer result.Tree.Release()

	root := result.Tree.RootNode()
	leaf := root.DescendantForByteRange(pos, pos)
	if leaf == nil {
		return nil, fmt.Errorf("node not found at position %d", pos)
	}

	lang := getLanguage(grammars.DetectLanguage(filePath))
	var infos []NodeInfo
	for n := leaf; n != nil; n = n.Parent() {
		infos = append(infos, nodeToInfo(n, lang, result.Source))
	}
	return infos, nil
}

// ─── Definitions ──────────────────────────────────────────────────────────────

// GetDefinitions returns all definition tags from a file using the Tagger API.
func (a *Analyzer) GetDefinitions(filePath string) ([]Tag, error) {
	result, err := a.ParseFile(filePath)
	if err != nil {
		return nil, err
	}
	defer result.Tree.Release()

	entry := grammars.DetectLanguage(filePath)
	if entry == nil {
		return nil, fmt.Errorf("unsupported language: %s", filePath)
	}

	tagsQuery := grammars.ResolveTagsQuery(*entry)
	if tagsQuery == "" {
		return nil, fmt.Errorf("no tags query for %s", filePath)
	}

	lang := getLanguage(entry)
	tagger, err := gotreesitter.NewTagger(lang, tagsQuery)
	if err != nil {
		return nil, fmt.Errorf("create tagger: %w", err)
	}

	tags := tagger.TagTree(result.Tree)
	return toTags(tags), nil
}

// ─── Validation ───────────────────────────────────────────────────────────────

// Validate checks a file for syntax errors, line ending style, and trailing newline.
func (a *Analyzer) Validate(filePath string) (*ValidationResult, error) {
	result, err := a.ParseFile(filePath)
	if err != nil {
		return nil, err
	}
	defer result.Tree.Release()

	vr := &ValidationResult{
		SourceSize: len(result.Source),
		Language:   result.Language,
	}

	// Line ending detection
	vr.LineEnding = detectLineEnding(result.Source)

	// Trailing newline
	if len(result.Source) > 0 && result.Source[len(result.Source)-1] == '\n' {
		vr.TrailingNewline = true
	}

	// Syntax errors
	entry := grammars.DetectLanguage(filePath)
	lang := getLanguage(entry)
	root := result.Tree.RootNode()
	if root != nil && root.HasError() {
		vr.SyntaxErrors = collectSyntaxErrors(root, lang)
		vr.Valid = false
	} else {
		vr.Valid = true
	}

	return vr, nil
}

// ─── AST Query ────────────────────────────────────────────────────────────────

// QueryAST executes a tree-sitter S-expression query against a file.
func (a *Analyzer) QueryAST(filePath string, pattern string) ([]QueryMatch, error) {
	result, err := a.ParseFile(filePath)
	if err != nil {
		return nil, err
	}
	defer result.Tree.Release()

	entry := grammars.DetectLanguage(filePath)
	lang := getLanguage(entry)

	q, err := gotreesitter.NewQuery(pattern, lang)
	if err != nil {
		return nil, fmt.Errorf("compile query: %w", err)
	}

	matches := q.Execute(result.Tree)
	queryMatches := make([]QueryMatch, 0, len(matches))

	for _, match := range matches {
		qm := QueryMatch{
			Pattern:  match.PatternIndex,
			Captures: make(map[string][]NodeInfo),
		}
		for _, cap := range match.Captures {
			qm.Captures[cap.Name] = append(qm.Captures[cap.Name], nodeToInfo(cap.Node, lang, result.Source))
		}
		queryMatches = append(queryMatches, qm)
	}

	return queryMatches, nil
}

// ─── Text Extraction ─────────────────────────────────────────────────────────

// GetText returns the source text in the given byte range.
func (a *Analyzer) GetText(filePath string, startByte, endByte uint32) (string, error) {
	source, err := os.ReadFile(filePath)
	if err != nil {
		return "", fmt.Errorf("read file: %w", err)
	}

	if startByte > endByte || endByte > uint32(len(source)) {
		return "", fmt.Errorf("invalid byte range [%d, %d), file size %d", startByte, endByte, len(source))
	}

	return string(source[startByte:endByte]), nil
}

// GetLine returns the content of a specific line (0-indexed, trailing newline stripped).
func (a *Analyzer) GetLine(filePath string, line uint32) (string, error) {
	source, err := os.ReadFile(filePath)
	if err != nil {
		return "", fmt.Errorf("read file: %w", err)
	}

	// Build line index simply
	offsets := []uint32{0}
	for i, b := range source {
		if b == '\n' {
			offsets = append(offsets, uint32(i+1))
		}
	}

	if int(line) >= len(offsets) {
		return "", fmt.Errorf("line %d out of bounds, file has %d lines", line, len(offsets))
	}

	start := offsets[line]
	var end uint32
	if int(line)+1 < len(offsets) {
		end = offsets[line+1]
	} else {
		end = uint32(len(source))
	}

	lineBytes := source[start:end]
	// Strip trailing newline
	if len(lineBytes) > 0 && lineBytes[len(lineBytes)-1] == '\n' {
		lineBytes = lineBytes[:len(lineBytes)-1]
		if len(lineBytes) > 0 && lineBytes[len(lineBytes)-1] == '\r' {
			lineBytes = lineBytes[:len(lineBytes)-1]
		}
	}

	return string(lineBytes), nil
}

// ─── Code Analysis ─────────────────────────────────────────────────────────

// AnalyzeFile performs complexity analysis on a file using tree-sitter.
// Returns per-function metrics and a summary.
func (a *Analyzer) AnalyzeFile(filePath string) (*AnalysisResult, error) {
	result, err := a.ParseFile(filePath)
	if err != nil {
		return nil, err
	}
	defer result.Tree.Release()

	entry := grammars.DetectLanguage(filePath)
	if entry == nil {
		return nil, fmt.Errorf("unsupported language: %s", filePath)
	}
	lang := getLanguage(entry)
	root := result.Tree.RootNode()

	tags, err := a.GetDefinitions(filePath)
	if err != nil {
		return nil, fmt.Errorf("get definitions: %w", err)
	}

	var functions []FuncAnalysis
	maxComplexity := 0
	totalCyclomatic := 0
	totalCognitive := 0

	for _, tag := range tags {
		if tag.Kind != "definition.function" && tag.Kind != "definition.method" {
			continue
		}
		bodyNode := findFuncBody(root, tag, lang)
		fa := FuncAnalysis{
			Name:      tag.Name,
			Kind:      strings.TrimPrefix(tag.Kind, "definition."),
			StartLine: tag.StartLine,
			EndLine:   tag.EndLine,
		}
		if bodyNode != nil {
			fa.Cyclomatic = cyclomaticComplex(bodyNode, lang, result.Source)
			fa.Cognitive = cognitiveComplex(bodyNode, lang, 0)
			fa.LoopDepth = maxLoopDepth(bodyNode, lang, 0)
			fa.LoopCount = countLoopsInNode(bodyNode, lang)
			fa.Recursive = checkRecursiveCall(bodyNode, tag.Name, lang, result.Source)
			fa.ParamCount = countFuncParams(bodyNode, tag.Kind, lang)
			fa.LinearScanInLoop = linearScanCount(bodyNode, lang, result.Source, false)
		}
		if fa.Cyclomatic > maxComplexity {
			maxComplexity = fa.Cyclomatic
		}
		totalCyclomatic += fa.Cyclomatic
		totalCognitive += fa.Cognitive
		functions = append(functions, fa)
	}

	n := len(functions)
	avgCyc := 0.0
	if n > 0 {
		avgCyc = float64(totalCyclomatic) / float64(n)
	}

	return &AnalysisResult{
		File:      filePath,
		Language:  result.Language,
		Functions: functions,
		Summary: AnalysisSummary{
			TotalFunctions: n,
			AvgCyclomatic:  avgCyc,
			MaxComplexity:  maxComplexity,
			TotalCognitive: totalCognitive,
		},
	}, nil
}

// ─── Complexity Helpers ─────────────────────────────────────────────────────

func findFuncBody(root *gotreesitter.Node, tag Tag, lang *gotreesitter.Language) *gotreesitter.Node {
	return searchBody(root, tag, lang)
}

func searchBody(node *gotreesitter.Node, tag Tag, lang *gotreesitter.Language) *gotreesitter.Node {
	if node == nil {
		return nil
	}
	typ := node.Type(lang)
	if isBodyBlock(typ) {
		sb, eb := node.StartByte(), node.EndByte()
		if sb >= tag.StartByte && eb <= tag.EndByte {
			return node
		}
	}
	for i := 0; i < int(node.ChildCount()); i++ {
		if found := searchBody(node.Child(i), tag, lang); found != nil {
			return found
		}
	}
	return nil
}

func isBodyBlock(typ string) bool {
	switch typ {
	case "block", "block_statement", "compound_statement",
		"statement_block", "body", "declaration_list":
		return true
	}
	return false
}

func cyclomaticComplex(node *gotreesitter.Node, lang *gotreesitter.Language, source []byte) int {
	if node == nil {
		return 1
	}
	c := 1
	walkCyclomatic(node, lang, source, &c)
	return c
}

func walkCyclomatic(node *gotreesitter.Node, lang *gotreesitter.Language, source []byte, c *int) {
	if node == nil {
		return
	}
	switch node.Type(lang) {
	case "if_statement", "else_clause", "for_statement", "while_statement",
		"do_statement", "switch_statement", "case_statement", "case",
		"catch_clause", "conditional_expression", "ternary_expression":
		*c++
	case "binary_expression":
		t := nodeText(node, lang, source)
		if strings.Contains(t, "&&") || strings.Contains(t, "||") {
			*c++
		}
	}
	for i := 0; i < int(node.ChildCount()); i++ {
		walkCyclomatic(node.Child(i), lang, source, c)
	}
}

func cognitiveComplex(node *gotreesitter.Node, lang *gotreesitter.Language, nesting int) int {
	if node == nil {
		return 0
	}
	s := 0
	walkCognitive(node, lang, nesting, &s)
	return s
}

func walkCognitive(node *gotreesitter.Node, lang *gotreesitter.Language, nesting int, s *int) {
	if node == nil {
		return
	}
	switch node.Type(lang) {
	case "if_statement", "else_clause", "for_statement", "while_statement",
		"do_statement", "catch_clause":
		*s += 1 + nesting
		for i := 0; i < int(node.ChildCount()); i++ {
			walkCognitive(node.Child(i), lang, nesting+1, s)
		}
		return
	case "switch_statement", "case_statement", "case":
		*s += 1 + nesting
		for i := 0; i < int(node.ChildCount()); i++ {
			walkCognitive(node.Child(i), lang, nesting, s)
		}
		return
	case "conditional_expression", "ternary_expression":
		*s += 1 + nesting
	}
	for i := 0; i < int(node.ChildCount()); i++ {
		walkCognitive(node.Child(i), lang, nesting, s)
	}
}

func maxLoopDepth(node *gotreesitter.Node, lang *gotreesitter.Language, depth int) int {
	if node == nil {
		return 0
	}
	typ := node.Type(lang)
	isLoop := typ == "for_statement" || typ == "while_statement" || typ == "do_statement"
	cur := depth
	if isLoop {
		cur++
	}
	m := cur
	for i := 0; i < int(node.ChildCount()); i++ {
		if cd := maxLoopDepth(node.Child(i), lang, cur); cd > m {
			m = cd
		}
	}
	return m
}

func countLoopsInNode(node *gotreesitter.Node, lang *gotreesitter.Language) int {
	if node == nil {
		return 0
	}
	c := 0
	walkLoops(node, lang, &c)
	return c
}

func walkLoops(node *gotreesitter.Node, lang *gotreesitter.Language, c *int) {
	if node == nil {
		return
	}
	switch node.Type(lang) {
	case "for_statement", "while_statement", "do_statement":
		*c++
	}
	for i := 0; i < int(node.ChildCount()); i++ {
		walkLoops(node.Child(i), lang, c)
	}
}

func checkRecursiveCall(body *gotreesitter.Node, name string, lang *gotreesitter.Language, source []byte) bool {
	if body == nil {
		return false
	}
	found := false
	walkRecursive(body, name, lang, source, &found)
	return found
}

func walkRecursive(node *gotreesitter.Node, name string, lang *gotreesitter.Language, source []byte, found *bool) {
	if *found || node == nil {
		return
	}
	if node.Type(lang) == "identifier" && nodeText(node, lang, source) == name {
		if p := node.Parent(); p != nil && p.Type(lang) == "call_expression" {
			*found = true
			return
		}
	}
	if node.Type(lang) == "field_identifier" || node.Type(lang) == "property_identifier" {
		if nodeText(node, lang, source) == name {
			p := node.Parent()
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
		walkRecursive(node.Child(i), name, lang, source, found)
	}
}

func countFuncParams(body *gotreesitter.Node, kind string, lang *gotreesitter.Language) int {
	if body == nil {
		return 0
	}
	parent := body.Parent()
	for parent != nil {
		switch parent.Type(lang) {
		case "function_definition", "method_definition",
			"function_declaration", "method_declaration", "arrow_function":
			for i := 0; i < int(parent.ChildCount()); i++ {
				child := parent.Child(i)
				if child != nil && (child.Type(lang) == "parameters" || child.Type(lang) == "parameter_list") {
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
								params++
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

func linearScanCount(node *gotreesitter.Node, lang *gotreesitter.Language, source []byte, inLoop bool) int {
	if node == nil {
		return 0
	}
	typ := node.Type(lang)
	isLoop := typ == "for_statement" || typ == "while_statement" || typ == "do_statement"
	curInLoop := inLoop || isLoop
	count := 0
	if curInLoop && typ == "call_expression" && node.ChildCount() > 0 {
		if fnNode := node.Child(0); fnNode != nil {
			if isScanName(nodeText(fnNode, lang, source)) {
				count++
			}
		}
	}
	for i := 0; i < int(node.ChildCount()); i++ {
		count += linearScanCount(node.Child(i), lang, source, curInLoop)
	}
	return count
}

var scanNames = map[string]bool{
	"find": true, "findFirst": true, "findLast": true, "findAny": true,
	"contains": true, "indexOf": true, "lastIndexOf": true,
	"includes": true, "search": true, "searchRange": true,
	"match": true, "matchAll": true,
	"Count": true, "Any": true, "All": true,
	"Filter": true, "Where": true, "First": true, "FirstOrDefault": true,
	"Single": true, "SingleOrDefault": true,
}

func isScanName(name string) bool {
	parts := strings.Split(name, ".")
	if len(parts) > 0 {
		name = parts[len(parts)-1]
	}
	return scanNames[name]
}

// nodeText returns the source text of a tree-sitter node.
func nodeText(node *gotreesitter.Node, lang *gotreesitter.Language, source []byte) string {
	if source != nil {
		return string(node.Text(source))
	}
	return ""
}


// buildStructure recursively builds a NodeInfo tree.
func buildStructure(node *gotreesitter.Node, lang *gotreesitter.Language, source []byte, depth, maxDepth, maxWidth int) NodeInfo {
	if node == nil {
		return NodeInfo{}
	}

	sp := node.StartPoint()
	ep := node.EndPoint()

	info := NodeInfo{
		Type:       node.Type(lang),
		StartByte:  node.StartByte(),
		EndByte:    node.EndByte(),
		StartPoint: [2]uint32{sp.Row, sp.Column},
		EndPoint:   [2]uint32{ep.Row, ep.Column},
		IsNamed:    node.IsNamed(),
		IsError:    node.HasError(),
		IsMissing:  node.IsMissing(),
	}

	childCount := int(node.ChildCount())
	if childCount == 0 || (maxDepth != -1 && depth >= maxDepth) {
		if node.IsNamed() || info.Type == "comment" {
			info.Text = node.Text(source)
			if len(info.Text) > 200 {
				info.Text = info.Text[:200] + "..."
			}
		}
		return info
	}

	if maxWidth != -1 && childCount > maxWidth {
		childCount = maxWidth
	}

	info.Children = make([]NodeInfo, 0, childCount)
	for i := 0; i < childCount; i++ {
		child := node.Child(i)
		if child == nil {
			continue
		}
		childInfo := buildStructure(child, lang, source, depth+1, maxDepth, maxWidth)
		info.Children = append(info.Children, childInfo)
	}

	return info
}

// nodeToInfo converts a gotreesitter.Node to NodeInfo.
func nodeToInfo(node *gotreesitter.Node, lang *gotreesitter.Language, source []byte) NodeInfo {
	sp := node.StartPoint()
	ep := node.EndPoint()

	return NodeInfo{
		Type:       node.Type(lang),
		StartByte:  node.StartByte(),
		EndByte:    node.EndByte(),
		StartPoint: [2]uint32{sp.Row, sp.Column},
		EndPoint:   [2]uint32{ep.Row, ep.Column},
		Text:       node.Text(source),
		IsNamed:    node.IsNamed(),
		IsError:    node.HasError(),
		IsMissing:  node.IsMissing(),
	}
}

// collectSyntaxErrors recursively collects ERROR and MISSING nodes.
func collectSyntaxErrors(node *gotreesitter.Node, lang *gotreesitter.Language) []SyntaxError {
	var errs []SyntaxError
	collectSyntaxErrorsRecursive(node, lang, &errs)
	return errs
}

func collectSyntaxErrorsRecursive(node *gotreesitter.Node, lang *gotreesitter.Language, errs *[]SyntaxError) {
	if node == nil {
		return
	}

	if node.Type(lang) == "ERROR" || node.IsMissing() {
		sp := node.StartPoint()
		ep := node.EndPoint()
		typ := "error"
		if node.IsMissing() {
			typ = "missing"
		}
		*errs = append(*errs, SyntaxError{
			Type:     typ,
			StartRow: sp.Row,
			StartCol: sp.Column,
			EndRow:   ep.Row,
			EndCol:   ep.Column,
		})
	}

	for i := 0; i < int(node.ChildCount()); i++ {
		collectSyntaxErrorsRecursive(node.Child(i), lang, errs)
	}
}

// toTags converts gotreesitter.Tag slices to our Tag format, filtering to definition.*.
func toTags(gtags []gotreesitter.Tag) []Tag {
	if len(gtags) == 0 {
		return nil
	}
	result := make([]Tag, 0, len(gtags))
	for _, t := range gtags {
		if !strings.HasPrefix(t.Kind, "definition.") {
			continue
		}
		result = append(result, Tag{
			Kind:      t.Kind,
			Name:      t.Name,
			StartByte: t.Range.StartByte,
			EndByte:   t.Range.EndByte,
			StartLine: t.Range.StartPoint.Row,
			EndLine:   t.Range.EndPoint.Row,
			StartCol:  t.Range.StartPoint.Column,
			EndCol:    t.Range.EndPoint.Column,
		})
	}
	return result
}

// detectLineEnding determines the line ending style of source bytes.
func detectLineEnding(source []byte) string {
	if len(source) == 0 {
		return "\n"
	}
	// Check for \r\n
	if slices.Contains(source, '\r') {
			return "\r\n"
		}
	return "\n"
}

// getLanguage retrieves the Language from a LangEntry.
func getLanguage(entry *grammars.LangEntry) *gotreesitter.Language {
	if entry == nil {
		return nil
	}
	return entry.Language()
}

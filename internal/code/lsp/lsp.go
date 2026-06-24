package lsp

import (
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

// ─── Internal Helpers ─────────────────────────────────────────────────────────

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
	for _, b := range source {
		if b == '\r' {
			return "\r\n"
		}
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

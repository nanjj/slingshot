// ──────────────────────────────────────────────
// Definition extraction via gotreesitter Tagger API
// ──────────────────────────────────────────────

package edit

import (
	"fmt"
	"strings"

	"github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
)

// Tag is a serializable representation of a tree-sitter tag,
// limited to definition.* tags for code intelligence.
type Tag struct {
	Kind      string `json:"kind"`      // e.g. "definition.function"
	Name      string `json:"name"`      // the identifier text
	StartByte uint32 `json:"startByte"` // byte offset of the full definition
	EndByte   uint32 `json:"endByte"`   // byte offset past the end of the definition
	StartLine uint32 `json:"startLine"` // 0-based line of the definition start
	EndLine   uint32 `json:"endLine"`   // 0-based line of the definition end
	StartCol  uint32 `json:"startCol"`  // 0-based column of the definition start
	EndCol    uint32 `json:"endCol"`    // 0-based column of the definition end
}

// StructureSummary groups definitions by kind for a high-level overview.
type StructureSummary struct {
	Language    string `json:"language"`
	Functions   []Tag  `json:"functions,omitempty"`
	Methods     []Tag  `json:"methods,omitempty"`
	Classes     []Tag  `json:"classes,omitempty"`
	Structs     []Tag  `json:"structs,omitempty"`
	Interfaces  []Tag  `json:"interfaces,omitempty"`
	Modules     []Tag  `json:"modules,omitempty"`
	Imports     []Tag  `json:"imports,omitempty"`
	Other       []Tag  `json:"other,omitempty"`
}

// GetDefs returns all definition tags from a document.
// Only tags with kind "definition.*" are included.
// Returns an error if the document cannot be opened or the language is unsupported.
func (ed *Editor) GetDefs(uri string) ([]Tag, error) {
	doc, err := ed.getOrOpenDocument(uri)
	if err != nil {
		return nil, err
	}
	doc.Lock()
	defer doc.Unlock()
	ed.reloadIfExternalModified(doc)

	if doc.tree == nil {
		return nil, ErrDocumentNotReady
	}

	// Resolve LangEntry to get TagsQuery
	entry := grammars.DetectLanguage(doc.origFilePath)
	if entry == nil {
		// Fallback: try virtual path from URI (e.g. scratch:///test.go)
		if vp := extractVirtualPath(doc.uri); vp != "" {
			entry = grammars.DetectLanguage(vp)
		}
	}
	if entry == nil {
		return nil, ErrUnsupportedLanguage
	}

	tagsQuery := grammars.ResolveTagsQuery(*entry)
	if tagsQuery == "" {
		return nil, fmt.Errorf("no tags query available for language %q", entry.Name)
	}

	tagger, err := gotreesitter.NewTagger(doc.language, tagsQuery)
	if err != nil {
		return nil, fmt.Errorf("create tagger: %w", err)
	}

	tags := tagger.TagTree(doc.tree)
	return toTags(tags), nil
}

// LocateDef finds a single definition by semantic selector.
//
// Supported selectors:
//
//	function:main        — find a function named "main"
//	method:GetName       — find a method (or function) named "GetName"
//	class:User           — find a class/struct/interface named "User"
//	struct:Point         — find a struct named "Point"
//	interface:Reader     — find an interface named "Reader"
//	lines:10-20          — (not supported here, use GetNodeAtRange)
func (ed *Editor) LocateDef(uri, selector string) (*Tag, error) {
	kind, name, err := parseSelector(selector)
	if err != nil {
		return nil, err
	}

	// lines:N-M is handled by GetNodeAtRange, not by definition lookup
	if kind == "lines" {
		return nil, fmt.Errorf("lines selector not supported for LocateDef")
	}

	tags, err := ed.GetDefs(uri)
	if err != nil {
		return nil, err
	}

	for _, tag := range tags {
		if matchSelector(tag, kind, name) {
			return &tag, nil
		}
	}
	return nil, fmt.Errorf("%s %q not found", kind, name)
}

// GetStructureSummary returns a high-level summary of definitions in a document,
// grouped by kind.
func (ed *Editor) GetStructureSummary(uri string) (*StructureSummary, error) {
	tags, err := ed.GetDefs(uri)
	if err != nil {
		return nil, err
	}

	// Get language name for the summary
	doc, err := ed.getOrOpenDocument(uri)
	if err != nil {
		return nil, err
	}
	langName := ""
	if entry := grammars.DetectLanguage(doc.origFilePath); entry != nil {
		langName = entry.Name
	} else if vp := extractVirtualPath(doc.uri); vp != "" {
		if entry := grammars.DetectLanguage(vp); entry != nil {
			langName = entry.Name
		}
	}
	if langName == "" {
		langName = "unknown"
	}
	summary := &StructureSummary{Language: langName}
	for _, tag := range tags {
		switch tag.Kind {
		case "definition.function":
			summary.Functions = append(summary.Functions, tag)
		case "definition.method":
			summary.Methods = append(summary.Methods, tag)
		case "definition.class":
			summary.Classes = append(summary.Classes, tag)
		case "definition.struct":
			summary.Structs = append(summary.Structs, tag)
		case "definition.interface":
			summary.Interfaces = append(summary.Interfaces, tag)
		case "definition.module":
			summary.Modules = append(summary.Modules, tag)
		default:
			summary.Other = append(summary.Other, tag)
		}
	}
	return summary, nil
}

// ─── helpers ────────────────────────────────────────────────────────

// toTags converts gotreesitter.Tag slices to our serializable Tag format,
// filtering to include only definition.* tags.
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

// parseSelector parses "function:main" into kind="function" and name="main".
//
// Valid formats:
//
//	function:name
//	method:name
//	class:name
//	struct:name
//	interface:name
//	lines:start-end   (returned as-is, caller handles separately)
func parseSelector(selector string) (kind, name string, err error) {
	idx := strings.Index(selector, ":")
	if idx < 0 {
		return "", "", fmt.Errorf("invalid selector %q: expected kind:name format (e.g. function:main)", selector)
	}
	kind = strings.TrimSpace(selector[:idx])
	name = strings.TrimSpace(selector[idx+1:])
	if kind == "" || name == "" {
		return "", "", fmt.Errorf("invalid selector %q: kind and name must not be empty", selector)
	}
	return kind, name, nil
}

// matchSelector checks whether a tag matches the requested kind and name.
//
// Kind comparison is flexible: "function" matches "definition.function",
// "class" matches "definition.class", "definition.struct", "definition.interface".
func matchSelector(tag Tag, kind, name string) bool {
	if tag.Name != name {
		return false
	}
	// Direct match: full kind string equals requested kind
	if tag.Kind == kind {
		return true
	}
	// Prefix match: "definition.<kind>" equals requested kind
	if strings.HasPrefix(tag.Kind, "definition.") && tag.Kind[len("definition."):] == kind {
		return true
	}
	// Special: "class" matches both "definition.class", "definition.struct", "definition.interface"
	if kind == "class" {
		if tag.Kind == "definition.class" || tag.Kind == "definition.struct" || tag.Kind == "definition.interface" {
			return true
		}
	}
	return false
}

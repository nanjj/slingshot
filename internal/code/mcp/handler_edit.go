// Package mcp — edit & locate: edit, edit_body, locate.
//
// The edit tool supports two interaction styles:
//
//  1. Structural (byte/node based): insert/replace/delete with pos, point,
//     startByte/endByte, or node selectors (paths).
//  2. Text based (LLM-friendly): replace with oldText → newText. The server
//     locates the text itself; pass occurrence=N when the text appears more
//     than once. This is the recommended mode — no byte offsets or AST paths.
package mcp

import (
	"context"
	"fmt"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/nanjj/clog"
	"github.com/nanjj/slingshot/internal/code/edit"
)

// ─── Argument structs ──────────────────────────────────────────────────────────

// codeEditArgs is the unified argument struct for edit (insert/replace/delete).
type codeEditArgs struct {
	File       string            `json:"file"`                 // File path (absolute or relative to project root)
	Mode       string            `json:"mode"`                 // "insert", "replace", "delete"
	Text       string            `json:"text,omitempty"`       // Text to insert or replace with
	NewText    string            `json:"newText,omitempty"`    // Alias for text (LLM find-and-replace style)
	OldText    string            `json:"oldText,omitempty"`    // For replace: original text to locate (text-based mode)
	Occurrence int               `json:"occurrence,omitempty"` // 1-based occurrence of oldText to replace (when ambiguous)
	Position   string            `json:"position,omitempty"`   // For insert: "pos" (default), "point", "before", "after"
	Target     string            `json:"target,omitempty"`     // For replace/delete: "range" (default), "node", "text"
	Pos        uint32            `json:"pos,omitempty"`        // Byte offset for insert/replace/delete
	StartByte  uint32            `json:"startByte,omitempty"`  // Start byte for range replace/delete
	EndByte    uint32            `json:"endByte,omitempty"`    // End byte for range replace/delete
	Row        uint32            `json:"row,omitempty"`        // Row for point insert
	Col        uint32            `json:"col,omitempty"`        // Column for point insert
	Selector   edit.NodeSelector `json:"selector,omitempty"`   // Node selector for before/after/node target
}

// codeEditBodyArgs is the argument struct for edit_body (semantic node replacement).
type codeEditBodyArgs struct {
	File     string `json:"file"`     // File path
	Selector string `json:"selector"` // Semantic selector, e.g. "function:Hello", "struct:Greeter"
	Text     string `json:"text"`     // Replacement text
}

type codeLocateArgs struct {
	QualifiedName string `json:"qualifiedName,omitempty"` // Symbol name to locate
	Name          string `json:"name,omitempty"`          // alias for qualifiedName
	Project       string `json:"project,omitempty"`       // Project name (optional once open_project is bound)
	File          string `json:"file,omitempty"`          // Fallback file for tree-sitter search
}

// ─── registerEdit ──────────────────────────────────────────────────────────

func registerCodeEdit(srv *mcp.Server, s *Server) {
	mcp.AddTool(srv, &mcp.Tool{
		Name:        "edit",
		Description: "Edit a file with write-through semantics (atomic save to disk). Modes: insert (pos/point/before/after), replace (text/range/node), delete (range/node). Recommended: replace with oldText+newText — the server finds the text for you (set occurrence=N when it appears multiple times). Structural modes use byte offsets or node selectors. Example: {file, mode:'replace', oldText:'foo()', newText:'bar()'}.",
		InputSchema: strictSchema[codeEditArgs](),
	}, func(ctx context.Context, req *mcp.CallToolRequest, args codeEditArgs) (*mcp.CallToolResult, any, error) {
		return s.handleCodeEdit(ctx, args), nil, nil
	})
}

func (s *Server) handleCodeEdit(ctx context.Context, args codeEditArgs) *mcp.CallToolResult {
	span, ctx := clog.StartSpanFromContext(ctx, "code_edit")
	defer span.Finish()
	clog.Info(ctx, "code_edit", "file", args.File, "mode", args.Mode, "oldText", args.OldText != "")

	if args.File == "" {
		return errorResult(fmt.Errorf("file is required"))
	}
	if args.Mode == "" {
		return errorResult(fmt.Errorf("mode is required: one of insert, replace, delete"))
	}

	ed := s.editor()
	if ed == nil {
		return errorResult(fmt.Errorf("editor not initialized"))
	}

	// Normalize file path: ensure absolute or resolve via project root
	filePath := args.File
	if root := s.currentRoot(); root != "" && !strings.HasPrefix(filePath, "/") {
		filePath = root + "/" + filePath
	}

	switch args.Mode {
	case "insert":
		if args.Text == "" {
			return errorResult(fmt.Errorf("text is required for insert mode"))
		}
		return s.handleCodeEditInsert(ed, filePath, args)

	case "replace":
		return s.handleCodeEditReplace(ed, filePath, args)

	case "delete":
		return s.handleCodeEditDelete(ed, filePath, args)

	default:
		return errorResult(fmt.Errorf("unsupported mode %q: use insert, replace, or delete", args.Mode))
	}
}

// ─── Insert ────────────────────────────────────────────────────────────────────

func (s *Server) handleCodeEditInsert(ed *edit.Editor, filePath string, args codeEditArgs) *mcp.CallToolResult {
	var result *edit.EditResult
	var err error

	switch args.Position {
	case "point":
		result, err = ed.InsertAtPoint(filePath, args.Row, args.Col, args.Text)
	case "before":
		if isEmptySelector(args.Selector) {
			return errorResult(fmt.Errorf("insert before requires a selector (pos, point, range, or path) — or use mode=insert with pos= for byte offset, or row=/col= for point insertion"))
		}
		result, err = ed.InsertBefore(filePath, args.Selector, args.Text)
	case "after":
		if isEmptySelector(args.Selector) {
			return errorResult(fmt.Errorf("insert after requires a selector (pos, point, range, or path) — or use mode=insert with pos= for byte offset, or row=/col= for point insertion"))
		}
		result, err = ed.InsertAfter(filePath, args.Selector, args.Text)
	default: // "pos"
		result, err = ed.Insert(filePath, args.Pos, args.Text)
	}
	if err != nil {
		return errorResult(fmt.Errorf("insert: %w", err))
	}
	return jsonResult(editResultToResponse(result))
}

// ─── Replace ───────────────────────────────────────────────────────────────────

func (s *Server) handleCodeEditReplace(ed *edit.Editor, filePath string, args codeEditArgs) *mcp.CallToolResult {
	// Text-based mode (recommended): locate oldText in the file and replace it.
	if args.OldText != "" {
		newText := firstNonEmpty(args.NewText, args.Text)
		if newText == "" {
			return errorResult(fmt.Errorf("replace by text requires newText (or text) alongside oldText"))
		}
		return s.replaceByText(ed, filePath, args.OldText, newText, args.Occurrence)
	}

	switch args.Target {
	case "node":
		if isEmptySelector(args.Selector) {
			return errorResult(fmt.Errorf("replace node requires a selector (pos, point, range, or path) — or use replace with oldText+newText for text-based replacement"))
		}
		if args.Text == "" && args.NewText == "" {
			return errorResult(fmt.Errorf("text is required for node replace — provide text (or newText)"))
		}
		result, err := ed.ReplaceNode(filePath, args.Selector, firstNonEmpty(args.NewText, args.Text))
		if err != nil {
			return errorResult(fmt.Errorf("replace: %w", err))
		}
		return jsonResult(editResultToResponse(result))

	default: // "range"
		if args.Text == "" && args.NewText == "" {
			return errorResult(fmt.Errorf("text is required for replace mode — provide text (or newText); for find-and-replace pass oldText+newText instead"))
		}
		result, err := ed.Replace(filePath, args.StartByte, args.EndByte, firstNonEmpty(args.NewText, args.Text))
		if err != nil {
			return errorResult(fmt.Errorf("replace: %w", err))
		}
		return jsonResult(editResultToResponse(result))
	}
}

// replaceByText performs a text-based find-and-replace: locate oldText in the
// file, then replace it with newText. This is the LLM-intuitive edit mode —
// no byte offsets, selectors, or AST paths required.
func (s *Server) replaceByText(ed *edit.Editor, filePath, oldText, newText string, occurrence int) *mcp.CallToolResult {
	if oldText == "" {
		return errorResult(fmt.Errorf("oldText cannot be empty"))
	}

	source, err := ed.GetSource(filePath)
	if err != nil {
		return errorResult(fmt.Errorf("read %s: %w", filePath, err))
	}

	// Find all occurrences (byte offsets).
	var starts []int
	from := 0
	for {
		idx := strings.Index(source[from:], oldText)
		if idx < 0 {
			break
		}
		pos := from + idx
		starts = append(starts, pos)
		from = pos + len(oldText)
	}

	switch len(starts) {
	case 0:
		return errorResult(fmt.Errorf("replace: oldText %q not found in %s — check exact text (whitespace/newlines matter)", oldText, filePath))
	case 1:
		// unique occurrence — proceed below
	default:
		if occurrence < 1 || occurrence > len(starts) {
			lines := make([]string, len(starts))
			for i, pos := range starts {
				lines[i] = fmt.Sprintf("%d", 1+strings.Count(source[:pos], "\n"))
			}
			return errorResult(fmt.Errorf("replace: oldText %q occurs %d times (lines %s) — set occurrence=1..%d to pick one, or use startByte/endByte", oldText, len(starts), strings.Join(lines, ", "), len(starts)))
		}
	}

	start := starts[0]
	if occurrence >= 1 && occurrence <= len(starts) {
		start = starts[occurrence-1]
	}
	result, err := ed.Replace(filePath, uint32(start), uint32(start+len(oldText)), newText)
	if err != nil {
		return errorResult(fmt.Errorf("replace: %w", err))
	}
	return jsonResult(editResultToResponse(result))
}

// ─── Delete ────────────────────────────────────────────────────────────────────

func (s *Server) handleCodeEditDelete(ed *edit.Editor, filePath string, args codeEditArgs) *mcp.CallToolResult {
	var result *edit.EditResult
	var err error

	switch args.Target {
	case "node":
		if isEmptySelector(args.Selector) {
			return errorResult(fmt.Errorf("delete node requires a selector (pos, point, range, or path)"))
		}
		result, err = ed.DeleteNode(filePath, args.Selector)
	default: // "range"
		result, err = ed.Delete(filePath, args.StartByte, args.EndByte)
	}
	if err != nil {
		return errorResult(fmt.Errorf("delete: %w", err))
	}
	return jsonResult(editResultToResponse(result))
}

// ─── registerEditBody ──────────────────────────────────────────────────────

func registerCodeEditBody(srv *mcp.Server, s *Server) {
	mcp.AddTool(srv, &mcp.Tool{
		Name:        "edit_body",
		Description: "Replace the body of a definition identified by a semantic selector. Selector format: 'function:name', 'method:name', 'struct:name', 'class:name', 'interface:name'. Opens the file, locates the definition, replaces its content, and saves to disk atomically.",
		InputSchema: strictSchema[codeEditBodyArgs](),
	}, func(ctx context.Context, req *mcp.CallToolRequest, args codeEditBodyArgs) (*mcp.CallToolResult, any, error) {
		return s.handleCodeEditBody(ctx, args), nil, nil
	})
}

func (s *Server) handleCodeEditBody(ctx context.Context, args codeEditBodyArgs) *mcp.CallToolResult {
	span, ctx := clog.StartSpanFromContext(ctx, "code_edit_body")
	defer span.Finish()
	clog.Info(ctx, "code_edit_body", "file", args.File, "selector", args.Selector)

	if args.File == "" {
		return errorResult(fmt.Errorf("file is required"))
	}
	if args.Selector == "" {
		return errorResult(fmt.Errorf("selector is required: e.g. 'function:Hello'"))
	}
	if args.Text == "" {
		return errorResult(fmt.Errorf("text is required"))
	}

	ed := s.editor()
	if ed == nil {
		return errorResult(fmt.Errorf("editor not initialized"))
	}

	filePath := args.File
	if root := s.currentRoot(); root != "" && !strings.HasPrefix(filePath, "/") {
		filePath = root + "/" + filePath
	}

	// Locate the definition via semantic selector
	tag, err := ed.LocateDef(filePath, args.Selector)
	if err != nil {
		clog.Error(ctx, "error", "error", err.Error())
		if strings.Contains(err.Error(), "no tags query") || strings.Contains(err.Error(), "unsupported language") {
			return errorResult(fmt.Errorf("locate definition: %v — this language has no tree-sitter tags query; use edit with oldText/newText (text-based) instead", err))
		}
		return errorResult(fmt.Errorf("locate definition: %w", err))
	}
	if tag == nil {
		return errorResult(fmt.Errorf("definition %q not found in %s", args.Selector, filePath))
	}

	// Replace the definition body
	result, err := ed.Replace(filePath, tag.StartByte, tag.EndByte, args.Text)
	if err != nil {
		clog.Error(ctx, "error", "error", err.Error())
		return errorResult(fmt.Errorf("replace definition: %w", err))
	}
	return jsonResult(editResultToResponse(result))
}

// ─── registerLocate ────────────────────────────────────────────────────────

func registerCodeLocate(srv *mcp.Server, s *Server) {
	mcp.AddTool(srv, &mcp.Tool{
		Name:        "locate",
		Description: "Locate a symbol definition by qualified name. First searches the SQLite code graph for the symbol's file, line, and column. Falls back to tree-sitter definition search in the specified file when the symbol is not indexed. Alias: name (qualifiedName).",
		InputSchema: strictSchema[codeLocateArgs](),
	}, func(ctx context.Context, req *mcp.CallToolRequest, args codeLocateArgs) (*mcp.CallToolResult, any, error) {
		return s.handleCodeLocate(ctx, args), nil, nil
	})
}

func (s *Server) handleCodeLocate(ctx context.Context, args codeLocateArgs) *mcp.CallToolResult {
	span, ctx := clog.StartSpanFromContext(ctx, "code_locate")
	defer span.Finish()

	qualifiedName := firstNonEmpty(args.QualifiedName, args.Name)
	clog.Info(ctx, "code_locate", "qualifiedName", qualifiedName, "project", args.Project)

	if qualifiedName == "" {
		clog.Error(ctx, "error", "error", "qualifiedName is required")
		return errorResult(fmt.Errorf("qualifiedName is required — pass qualifiedName (or name)"))
	}

	// 1. Try SQLite lookup first (project resolved or bound; failure just
	// falls through to the file-based search below).
	if info, err := s.resolveProject(args.Project); err == nil {
		node, err := s.store.GetNodeByQN(qualifiedName, info.Name)
		if err == nil {
			clog.Info(ctx, "code_locate_result", "source", "sqlite", "file", node.FilePath)
			return jsonResult(map[string]any{
				"found":         true,
				"source":        "sqlite",
				"file":          node.FilePath,
				"line":          node.Line,
				"col":           node.Col,
				"endLine":       node.EndLine,
				"endCol":        node.EndCol,
				"kind":          node.Kind,
				"name":          node.Name,
				"signature":     node.Signature,
				"qualifiedName": node.QualifiedName,
			})
		}
	}

	// 2. Fallback: tree-sitter search in specified file
	if args.File == "" {
		return jsonResult(map[string]any{
			"found":         false,
			"note":          fmt.Sprintf("symbol %q not found in sqlite; provide a file argument for tree-sitter fallback", qualifiedName),
			"qualifiedName": qualifiedName,
		})
	}

	ed := s.editor()
	if ed == nil {
		return jsonResult(map[string]any{
			"found":         false,
			"note":          fmt.Sprintf("symbol %q not found in sqlite; editor not initialized for file fallback", qualifiedName),
			"qualifiedName": qualifiedName,
		})
	}

	filePath := args.File
	if root := s.currentRoot(); root != "" && !strings.HasPrefix(filePath, "/") {
		filePath = root + "/" + filePath
	}

	// Get all definitions and filter by qualified name
	defs, err := ed.GetDefs(filePath)
	if err != nil {
		return jsonResult(map[string]any{
			"found":         false,
			"note":          fmt.Sprintf("symbol %q not found in sqlite; tree-sitter search failed: %v", qualifiedName, err),
			"qualifiedName": qualifiedName,
		})
	}

	for _, def := range defs {
		if def.Name == qualifiedName {
			// Strip "definition." prefix for cleaner output
			kind := strings.TrimPrefix(def.Kind, "definition.")
			return jsonResult(map[string]any{
				"found":         true,
				"source":        "treesitter",
				"file":          args.File,
				"line":          def.StartLine,
				"col":           def.StartCol,
				"endLine":       def.EndLine,
				"endCol":        def.EndCol,
				"kind":          kind,
				"name":          def.Name,
				"qualifiedName": def.Name,
			})
		}
	}
	return jsonResult(map[string]any{
		"found": false,
		"note":  fmt.Sprintf("symbol %q not found in sqlite or file", qualifiedName),
	})
}

// ─── Helpers ───────────────────────────────────────────────────────────────────

// isEmptySelector reports whether a NodeSelector is entirely unset.
func isEmptySelector(sel edit.NodeSelector) bool {
	return sel.Pos == nil && sel.Point == nil && sel.Range == nil && len(sel.Path) == 0
}

// editResultToResponse converts edit.EditResult to a friendly JSON map.
func editResultToResponse(er *edit.EditResult) map[string]any {
	if er == nil {
		return map[string]any{"success": false}
	}
	return map[string]any{
		"success":     er.Success,
		"byteDiff":    er.ByteDiff,
		"parseErrors": er.ParseErrors,
	}
}

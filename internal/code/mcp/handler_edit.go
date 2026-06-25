// Package mcp provides MCP tool handlers for code intelligence.
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

// codeEditArgs is the unified argument struct for code_edit (insert/replace/delete).
type codeEditArgs struct {
	File      string              `json:"file"`                // File path (absolute or relative to project root)
	Mode      string              `json:"mode"`                // "insert", "replace", "delete"
	Text      string              `json:"text,omitempty"`      // Text to insert or replace with
	Position  string              `json:"position,omitempty"`  // For insert: "pos" (default), "point", "before", "after"
	Target    string              `json:"target,omitempty"`    // For replace/delete: "range" (default), "node"
	Pos       uint32              `json:"pos,omitempty"`       // Byte offset for insert/replace/delete
	StartByte uint32              `json:"startByte,omitempty"` // Start byte for range replace/delete
	EndByte   uint32              `json:"endByte,omitempty"`   // End byte for range replace/delete
	Row       uint32              `json:"row,omitempty"`       // Row for point insert
	Col       uint32              `json:"col,omitempty"`       // Column for point insert
	Selector  edit.NodeSelector   `json:"selector,omitempty"`  // Node selector for before/after/node target
}

// codeEditBodyArgs is the argument struct for code_edit_body (semantic node replacement).
type codeEditBodyArgs struct {
	File     string `json:"file"`     // File path
	Selector string `json:"selector"` // Semantic selector, e.g. "function:Hello", "struct:Greeter"
	Text     string `json:"text"`     // Replacement text
}

type codeLocateArgs struct {
	QualifiedName string `json:"qualifiedName"`          // Symbol name to locate
	Project       string `json:"project,omitempty"`       // Project name (for sqlite lookup)
	File          string `json:"file,omitempty"`          // Fallback file for tree-sitter search
}

// ─── registerCodeEdit ──────────────────────────────────────────────────────────

func registerCodeEdit(srv *mcp.Server, s *Server) {
	mcp.AddTool(srv, &mcp.Tool{
		Name:        "edit",
		Description: "Edit a file with write-through semantics. Modes: insert (pos/point/before/after), replace (range/node), delete (range/node). Opens the file with tree-sitter incremental parsing, applies the edit, and saves to disk atomically. The document is cached for subsequent edits.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args codeEditArgs) (*mcp.CallToolResult, any, error) {
		return s.handleCodeEdit(ctx, args), nil, nil
	})
}

func (s *Server) handleCodeEdit(ctx context.Context, args codeEditArgs) *mcp.CallToolResult {
	span, ctx := clog.StartSpanFromContext(ctx, "code_edit")
	defer span.Finish()
	clog.Info(ctx, "code_edit", "file", args.File, "mode", args.Mode)

	if args.File == "" {
		return errorResult(fmt.Errorf("file is required"))
	}
	if args.Mode == "" {
		return errorResult(fmt.Errorf("mode is required: one of insert, replace, delete"))
	}

	if s.ed == nil {
		return errorResult(fmt.Errorf("editor not initialized"))
	}

	// Normalize file path: ensure absolute or resolve via project root
	filePath := args.File
	if s.opts.ProjectRoot != "" && !strings.HasPrefix(filePath, "/") {
		filePath = s.opts.ProjectRoot + "/" + filePath
	}

	switch args.Mode {
	case "insert":
		if args.Text == "" {
			return errorResult(fmt.Errorf("text is required for insert mode"))
		}
		return s.handleCodeEditInsert(filePath, args)

	case "replace":
		if args.Text == "" {
			return errorResult(fmt.Errorf("text is required for replace mode"))
		}
		return s.handleCodeEditReplace(filePath, args)

	case "delete":
		return s.handleCodeEditDelete(filePath, args)

	default:
		return errorResult(fmt.Errorf("unsupported mode %q: use insert, replace, or delete", args.Mode))
	}
}

// ─── Insert ────────────────────────────────────────────────────────────────────

func (s *Server) handleCodeEditInsert(filePath string, args codeEditArgs) *mcp.CallToolResult {
	var result *edit.EditResult
	var err error

	switch args.Position {
	case "point":
		result, err = s.ed.InsertAtPoint(filePath, args.Row, args.Col, args.Text)
	case "before":
		result, err = s.ed.InsertBefore(filePath, args.Selector, args.Text)
	case "after":
		result, err = s.ed.InsertAfter(filePath, args.Selector, args.Text)
	default: // "pos"
		result, err = s.ed.Insert(filePath, args.Pos, args.Text)
	}
	if err != nil {
		return errorResult(fmt.Errorf("insert: %w", err))
	}
	return jsonResult(editResultToResponse(result))
}

// ─── Replace ───────────────────────────────────────────────────────────────────

func (s *Server) handleCodeEditReplace(filePath string, args codeEditArgs) *mcp.CallToolResult {
	var result *edit.EditResult
	var err error

	switch args.Target {
	case "node":
		result, err = s.ed.ReplaceNode(filePath, args.Selector, args.Text)
	default: // "range"
		result, err = s.ed.Replace(filePath, args.StartByte, args.EndByte, args.Text)
	}
	if err != nil {
		return errorResult(fmt.Errorf("replace: %w", err))
	}
	return jsonResult(editResultToResponse(result))
}

// ─── Delete ────────────────────────────────────────────────────────────────────

func (s *Server) handleCodeEditDelete(filePath string, args codeEditArgs) *mcp.CallToolResult {
	var result *edit.EditResult
	var err error

	switch args.Target {
	case "node":
		result, err = s.ed.DeleteNode(filePath, args.Selector)
	default: // "range"
		result, err = s.ed.Delete(filePath, args.StartByte, args.EndByte)
	}
	if err != nil {
		return errorResult(fmt.Errorf("delete: %w", err))
	}
	return jsonResult(editResultToResponse(result))
}

// ─── registerCodeEditBody ──────────────────────────────────────────────────────

func registerCodeEditBody(srv *mcp.Server, s *Server) {
	mcp.AddTool(srv, &mcp.Tool{
		Name:        "edit_body",
		Description: "Replace the body of a definition identified by a semantic selector. Selector format: 'function:name', 'method:name', 'struct:name', 'class:name', 'interface:name'. Opens the file, locates the definition, replaces its content, and saves to disk atomically.",
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
	if s.ed == nil {
		return errorResult(fmt.Errorf("editor not initialized"))
	}

	filePath := args.File
	if s.opts.ProjectRoot != "" && !strings.HasPrefix(filePath, "/") {
		filePath = s.opts.ProjectRoot + "/" + filePath
	}

	// Locate the definition via semantic selector
	tag, err := s.ed.LocateDef(filePath, args.Selector)
	if err != nil {
		clog.Error(ctx, "error", "error", err.Error())
		return errorResult(fmt.Errorf("locate definition: %w", err))
	}
	if tag == nil {
		return errorResult(fmt.Errorf("definition %q not found in %s", args.Selector, filePath))
	}

	// Replace the definition body
	result, err := s.ed.Replace(filePath, tag.StartByte, tag.EndByte, args.Text)
	if err != nil {
		clog.Error(ctx, "error", "error", err.Error())
		return errorResult(fmt.Errorf("replace definition: %w", err))
	}
	return jsonResult(editResultToResponse(result))
}

// ─── registerCodeLocate ────────────────────────────────────────────────────────

func registerCodeLocate(srv *mcp.Server, s *Server) {
	mcp.AddTool(srv, &mcp.Tool{
		Name:        "locate",
		Description: "Locate a symbol definition by qualified name. First searches the SQLite code graph for the symbol's file, line, and column. Falls back to tree-sitter definition search in the specified file when the symbol is not indexed.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args codeLocateArgs) (*mcp.CallToolResult, any, error) {
		return s.handleCodeLocate(ctx, args), nil, nil
	})
}

func (s *Server) handleCodeLocate(ctx context.Context, args codeLocateArgs) *mcp.CallToolResult {
	span, ctx := clog.StartSpanFromContext(ctx, "code_locate")
	defer span.Finish()
	clog.Info(ctx, "code_locate", "qualifiedName", args.QualifiedName, "project", args.Project)

	if args.QualifiedName == "" {
		clog.Error(ctx, "error", "error", "qualifiedName is required")
		return errorResult(fmt.Errorf("qualifiedName is required"))
	}

	// 1. Try SQLite lookup first
	if args.Project != "" {
		node, err := s.store.GetNodeByQN(args.QualifiedName, args.Project)
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
			"note":          fmt.Sprintf("symbol %q not found in sqlite; provide a file argument for tree-sitter fallback", args.QualifiedName),
			"qualifiedName": args.QualifiedName,
		})
	}

	if s.ed == nil {
		return jsonResult(map[string]any{
			"found":         false,
			"note":          fmt.Sprintf("symbol %q not found in sqlite; editor not initialized for file fallback", args.QualifiedName),
			"qualifiedName": args.QualifiedName,
		})
	}

	filePath := args.File
	if s.opts.ProjectRoot != "" && !strings.HasPrefix(filePath, "/") {
		filePath = s.opts.ProjectRoot + "/" + filePath
	}

	// Get all definitions and filter by qualified name
	defs, err := s.ed.GetDefs(filePath)
	if err != nil {
		return jsonResult(map[string]any{
			"found":         false,
			"note":          fmt.Sprintf("symbol %q not found in sqlite; tree-sitter search failed: %v", args.QualifiedName, err),
			"qualifiedName": args.QualifiedName,
		})
	}

	for _, def := range defs {
		if def.Name == args.QualifiedName {
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
		"found":         false,
		"note":          fmt.Sprintf("symbol %q not found in sqlite or file", args.QualifiedName),
	})
}

// ─── Helpers ───────────────────────────────────────────────────────────────────

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

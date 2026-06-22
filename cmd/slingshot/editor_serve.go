package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"strings"

	"github.com/fatih/color"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/spf13/cobra"

	cli "github.com/nanjj/slingshot/internal/cmd"
	"github.com/nanjj/slingshot/internal/editor"
)

// ─── Input structs ───────────────────────────────────────────────────────────

type openDocumentArgs struct {
	URI      string `json:"uri"`
	Source   string `json:"source,omitempty"`
	Language string `json:"language,omitempty"`
}

type closeDocumentArgs struct {
	URI string `json:"uri"`
}

type getStructureArgs struct {
	URI         string `json:"uri"`
	MaxDepth    int    `json:"maxDepth,omitempty"`
	MaxChildren int    `json:"maxChildren,omitempty"`
}

type byteURIPositionArgs struct {
	URI string `json:"uri"`
	Pos uint32 `json:"pos"`
}

type pointArgs struct {
	URI string `json:"uri"`
	Row uint32 `json:"row"`
	Col uint32 `json:"col"`
}

type byteRangeArgs struct {
	URI       string `json:"uri"`
	StartByte uint32 `json:"startByte"`
	EndByte   uint32 `json:"endByte"`
}

type queryArgs struct {
	URI     string `json:"uri"`
	Pattern string `json:"pattern"`
}

type lineArgs struct {
	URI  string `json:"uri"`
	Line uint32 `json:"line"`
}

type insertArgs struct {
	URI  string `json:"uri"`
	Pos  uint32 `json:"pos"`
	Text string `json:"text"`
}

type insertAtPointArgs struct {
	URI  string `json:"uri"`
	Row  uint32 `json:"row"`
	Col  uint32 `json:"col"`
	Text string `json:"text"`
}

type insertBeforeAfterArgs struct {
	URI      string              `json:"uri"`
	Selector editor.NodeSelector `json:"selector"`
	Text     string              `json:"text"`
}

type replaceArgs struct {
	URI       string `json:"uri"`
	StartByte uint32 `json:"startByte"`
	EndByte   uint32 `json:"endByte"`
	Text      string `json:"text"`
}

type replaceNodeArgs struct {
	URI      string              `json:"uri"`
	Selector editor.NodeSelector `json:"selector"`
	Text     string              `json:"text"`
}

type deleteRangeArgs struct {
	URI       string `json:"uri"`
	StartByte uint32 `json:"startByte"`
	EndByte   uint32 `json:"endByte"`
}

type deleteNodeArgs struct {
	URI      string              `json:"uri"`
	Selector editor.NodeSelector `json:"selector"`
}

type validateArgs struct {
	URI string `json:"uri"`
}

type saveArgs struct {
	URI string `json:"uri"`
}

type uriOnlyArgs struct {
	URI string `json:"uri"`
}

// ─── Project root stack ─────────────────────────────────────────────────────

type pushProjectRootArgs struct {
	ProjectRoot string `json:"projectRoot"`
}

// ─── Response types ──────────────────────────────────────────────────────────

// editResultResponse is the MCP-friendly subset of editor.EditResult.
type editResultResponse struct {
	Success     bool     `json:"success"`
	ByteDiff    int      `json:"byteDiff"`
	ParseErrors []string `json:"parseErrors,omitempty"`
}

// saveResultResponse is the MCP-friendly subset of editor.SaveResult.
type saveResultResponse struct {
	Success  bool   `json:"success"`
	Path     string `json:"path"`
	Bytes    int    `json:"bytes"`
	Version  int    `json:"version"`
	Conflict bool   `json:"conflict,omitempty"`
	Message  string `json:"message,omitempty"`
}

// ─── MCP helper utilities ────────────────────────────────────────────────────

// jsonResult wraps any value as a successful MCP CallToolResult with JSON text content.
func jsonResult(v any) *mcp.CallToolResult {
	data, err := json.Marshal(v)
	if err != nil {
		return errorResult(fmt.Errorf("marshal result: %w", err))
	}
	return &mcp.CallToolResult{
		Content: []mcp.Content{
			&mcp.TextContent{
				Text: string(data),
			},
		},
	}
}

// errorResult wraps an error as an MCP CallToolResult with IsError set.
func errorResult(err error) *mcp.CallToolResult {
	return &mcp.CallToolResult{
		IsError: true,
		Content: []mcp.Content{
			&mcp.TextContent{
				Text: err.Error(),
			},
		},
	}
}

// ─── Server + tool handlers ──────────────────────────────────────────────────

// editorServer holds the shared state for all tool handlers.
type editorServer struct {
	ed   *editor.Editor
	opts *serveOptions
}

// --- Document lifecycle ---

func (es *editorServer) openDocument(args openDocumentArgs) *mcp.CallToolResult {
	var source []byte
	if args.Source != "" {
		source = []byte(args.Source)
	}
	if err := es.ed.OpenDocument(args.URI, source, args.Language); err != nil {
		return errorResult(fmt.Errorf("open document: %w", err))
	}
	return jsonResult(map[string]bool{"success": true})
}

func (es *editorServer) closeDocument(args closeDocumentArgs) *mcp.CallToolResult {
	if err := es.ed.CloseDocument(args.URI); err != nil {
		return errorResult(fmt.Errorf("close document: %w", err))
	}
	return jsonResult(map[string]bool{"success": true})
}

// --- Read operations ---

func (es *editorServer) getStructure(args getStructureArgs) *mcp.CallToolResult {
	// Default: maxDepth=-1 (recursive), maxChildren=-1 (unlimited)
	maxDepth := args.MaxDepth
	if maxDepth == 0 {
		maxDepth = -1 // zero in JSON = not specified, default to -1
	}
	maxChildren := args.MaxChildren
	if maxChildren == 0 {
		maxChildren = -1
	}
	nodeInfo, err := es.ed.GetStructure(args.URI, maxDepth, maxChildren)
	if err != nil {
		return errorResult(fmt.Errorf("get structure: %w", err))
	}
	return jsonResult(nodeInfo)
}

func (es *editorServer) getNode(args byteURIPositionArgs) *mcp.CallToolResult {
	nodeInfo, err := es.ed.GetNode(args.URI, args.Pos)
	if err != nil {
		return errorResult(fmt.Errorf("get node: %w", err))
	}
	return jsonResult(nodeInfo)
}

func (es *editorServer) getNodeAtPoint(args pointArgs) *mcp.CallToolResult {
	nodeInfo, err := es.ed.GetNodeAtPoint(args.URI, args.Row, args.Col)
	if err != nil {
		return errorResult(fmt.Errorf("get node at point: %w", err))
	}
	return jsonResult(nodeInfo)
}

func (es *editorServer) getNodeAtRange(args byteRangeArgs) *mcp.CallToolResult {
	nodeInfo, err := es.ed.GetNodeAtRange(args.URI, args.StartByte, args.EndByte)
	if err != nil {
		return errorResult(fmt.Errorf("get node at range: %w", err))
	}
	return jsonResult(nodeInfo)
}

func (es *editorServer) getDescendantsAt(args byteURIPositionArgs) *mcp.CallToolResult {
	infos, err := es.ed.GetDescendantsAt(args.URI, args.Pos)
	if err != nil {
		return errorResult(fmt.Errorf("get descendants at: %w", err))
	}
	return jsonResult(infos)
}

func (es *editorServer) query(args queryArgs) *mcp.CallToolResult {
	results, err := es.ed.Query(args.URI, args.Pattern)
	if err != nil {
		return errorResult(fmt.Errorf("query: %w", err))
	}
	return jsonResult(results)
}

func (es *editorServer) getText(args byteRangeArgs) *mcp.CallToolResult {
	text, err := es.ed.GetText(args.URI, args.StartByte, args.EndByte)
	if err != nil {
		return errorResult(fmt.Errorf("get text: %w", err))
	}
	return jsonResult(map[string]string{"text": text})
}

func (es *editorServer) getLine(args lineArgs) *mcp.CallToolResult {
	text, err := es.ed.GetLine(args.URI, args.Line)
	if err != nil {
		return errorResult(fmt.Errorf("get line: %w", err))
	}
	return jsonResult(map[string]string{"text": text})
}

// --- Edit operations ---

func (es *editorServer) insert(args insertArgs) *mcp.CallToolResult {
	editResult, err := es.ed.Insert(args.URI, args.Pos, args.Text)
	if err != nil {
		return errorResult(fmt.Errorf("insert: %w", err))
	}
	return jsonResult(editResultToResponse(editResult))
}

func (es *editorServer) insertAtPoint(args insertAtPointArgs) *mcp.CallToolResult {
	editResult, err := es.ed.InsertAtPoint(args.URI, args.Row, args.Col, args.Text)
	if err != nil {
		return errorResult(fmt.Errorf("insert at point: %w", err))
	}
	return jsonResult(editResultToResponse(editResult))
}

func (es *editorServer) insertBefore(args insertBeforeAfterArgs) *mcp.CallToolResult {
	editResult, err := es.ed.InsertBefore(args.URI, args.Selector, args.Text)
	if err != nil {
		return errorResult(fmt.Errorf("insert before: %w", err))
	}
	return jsonResult(editResultToResponse(editResult))
}

func (es *editorServer) insertAfter(args insertBeforeAfterArgs) *mcp.CallToolResult {
	editResult, err := es.ed.InsertAfter(args.URI, args.Selector, args.Text)
	if err != nil {
		return errorResult(fmt.Errorf("insert after: %w", err))
	}
	return jsonResult(editResultToResponse(editResult))
}

func (es *editorServer) replace(args replaceArgs) *mcp.CallToolResult {
	editResult, err := es.ed.Replace(args.URI, args.StartByte, args.EndByte, args.Text)
	if err != nil {
		return errorResult(fmt.Errorf("replace: %w", err))
	}
	return jsonResult(editResultToResponse(editResult))
}

func (es *editorServer) replaceNode(args replaceNodeArgs) *mcp.CallToolResult {
	editResult, err := es.ed.ReplaceNode(args.URI, args.Selector, args.Text)
	if err != nil {
		return errorResult(fmt.Errorf("replace node: %w", err))
	}
	return jsonResult(editResultToResponse(editResult))
}

func (es *editorServer) delete(args deleteRangeArgs) *mcp.CallToolResult {
	editResult, err := es.ed.Delete(args.URI, args.StartByte, args.EndByte)
	if err != nil {
		return errorResult(fmt.Errorf("delete: %w", err))
	}
	return jsonResult(editResultToResponse(editResult))
}

func (es *editorServer) deleteNode(args deleteNodeArgs) *mcp.CallToolResult {
	editResult, err := es.ed.DeleteNode(args.URI, args.Selector)
	if err != nil {
		return errorResult(fmt.Errorf("delete node: %w", err))
	}
	return jsonResult(editResultToResponse(editResult))
}

// --- Validation & Save ---

func (es *editorServer) validate(args validateArgs) *mcp.CallToolResult {
	result, err := es.ed.Validate(args.URI)
	if err != nil {
		return errorResult(fmt.Errorf("validate: %w", err))
	}
	return jsonResult(result)
}

func (es *editorServer) save(args saveArgs) *mcp.CallToolResult {
	result, err := es.ed.Save(args.URI)
	if err != nil {
		return errorResult(fmt.Errorf("save: %w", err))
	}
	return jsonResult(saveResultToResponse(result))
}

func (es *editorServer) forceSave(args saveArgs) *mcp.CallToolResult {
	result, err := es.ed.ForceSave(args.URI)
	if err != nil {
		return errorResult(fmt.Errorf("force save: %w", err))
	}
	return jsonResult(saveResultToResponse(result))
}

func (es *editorServer) isDirty(args uriOnlyArgs) *mcp.CallToolResult {
	dirty, err := es.ed.IsDirty(args.URI)
	if err != nil {
		return errorResult(fmt.Errorf("is dirty: %w", err))
	}
	return jsonResult(map[string]bool{"dirty": dirty})
}

func (es *editorServer) listDirty() *mcp.CallToolResult {
	dirty := es.ed.DirtyDocuments()
	return jsonResult(map[string][]string{"uris": dirty})
}

// --- Project root stack ---

func (es *editorServer) pushProjectRoot(args pushProjectRootArgs) *mcp.CallToolResult {
	es.ed.PushProjectRoot(args.ProjectRoot)
	return jsonResult(map[string]string{
		"status":      "ok",
		"projectRoot": args.ProjectRoot,
	})
}

func (es *editorServer) popProjectRoot() *mcp.CallToolResult {
	prev, err := es.ed.PopProjectRoot()
	if err != nil {
		return errorResult(err)
	}
	return jsonResult(map[string]string{
		"status":       "ok",
		"previousRoot": prev,
	})
}

// ─── Helper functions ────────────────────────────────────────────────────────

func editResultToResponse(er *editor.EditResult) any {
	if er == nil {
		return map[string]bool{"success": false}
	}
	return editResultResponse{
		Success:     er.Success,
		ByteDiff:    er.ByteDiff,
		ParseErrors: er.ParseErrors,
	}
}

func saveResultToResponse(sr *editor.SaveResult) any {
	if sr == nil {
		return map[string]bool{"success": false}
	}
	return saveResultResponse{
		Success:  sr.Success,
		Path:     sr.Path,
		Bytes:    sr.Bytes,
		Version:  sr.Version,
		Conflict: sr.Conflict,
		Message:  sr.Message,
	}
}

func parseLogLevel(level string) slog.Level {
	switch strings.ToLower(level) {
	case "debug":
		return slog.LevelDebug
	case "info":
		return slog.LevelInfo
	case "warn", "warning":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelWarn
	}
}

// ─── Tool Registration ───────────────────────────────────────────────────────

func registerAllTools(srv *mcp.Server, es *editorServer) {
	// --- Document lifecycle ---
	mcp.AddTool(srv, &mcp.Tool{
		Name:        "open_document",
		Description: "Open a document for editing. If a document with the same URI is already open, it is closed first. Supports file:// URIs and scratch:// URIs for ephemeral snippets.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args openDocumentArgs) (*mcp.CallToolResult, any, error) {
		return es.openDocument(args), nil, nil
	})

	mcp.AddTool(srv, &mcp.Tool{
		Name:        "close_document",
		Description: "Close a document and release its tree-sitter resources. Unsaved changes are discarded.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args closeDocumentArgs) (*mcp.CallToolResult, any, error) {
		return es.closeDocument(args), nil, nil
	})

	// --- Read operations ---
	mcp.AddTool(srv, &mcp.Tool{
		Name:        "get_structure",
		Description: "Get the hierarchical code structure (syntax tree) of an open document. Returns nested NodeInfo with type, byte range, and child nodes. Use maxDepth and maxChildren to limit output size. Default: recursive, unlimited children.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args getStructureArgs) (*mcp.CallToolResult, any, error) {
		return es.getStructure(args), nil, nil
	})

	mcp.AddTool(srv, &mcp.Tool{
		Name:        "get_node",
		Description: "Get the smallest AST node at a given byte offset. Returns the node's type, position, and source text.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args byteURIPositionArgs) (*mcp.CallToolResult, any, error) {
		return es.getNode(args), nil, nil
	})

	mcp.AddTool(srv, &mcp.Tool{
		Name:        "get_node_at_point",
		Description: "Get the smallest AST node at a given row and column position.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args pointArgs) (*mcp.CallToolResult, any, error) {
		return es.getNodeAtPoint(args), nil, nil
	})

	mcp.AddTool(srv, &mcp.Tool{
		Name:        "get_node_at_range",
		Description: "Get the smallest AST node that covers the specified byte range [startByte, endByte).",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args byteRangeArgs) (*mcp.CallToolResult, any, error) {
		return es.getNodeAtRange(args), nil, nil
	})

	mcp.AddTool(srv, &mcp.Tool{
		Name:        "get_descendants_at",
		Description: "Get all ancestor nodes covering a byte position, ordered from innermost to outermost (root). Useful for understanding context around a position.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args byteURIPositionArgs) (*mcp.CallToolResult, any, error) {
		return es.getDescendantsAt(args), nil, nil
	})

	mcp.AddTool(srv, &mcp.Tool{
		Name:        "query",
		Description: "Execute a tree-sitter S-expression query (.scm pattern) against the document's syntax tree. Returns matching captures grouped by capture name.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args queryArgs) (*mcp.CallToolResult, any, error) {
		return es.query(args), nil, nil
	})

	mcp.AddTool(srv, &mcp.Tool{
		Name:        "get_text",
		Description: "Get the source text in the specified byte range [startByte, endByte).",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args byteRangeArgs) (*mcp.CallToolResult, any, error) {
		return es.getText(args), nil, nil
	})

	mcp.AddTool(srv, &mcp.Tool{
		Name:        "get_line",
		Description: "Get the text content of a specific line (0-indexed). The trailing newline is stripped.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args lineArgs) (*mcp.CallToolResult, any, error) {
		return es.getLine(args), nil, nil
	})

	// --- Edit operations ---
	mcp.AddTool(srv, &mcp.Tool{
		Name:        "insert",
		Description: "Insert text at a byte offset position. The existing text after the position is shifted right.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args insertArgs) (*mcp.CallToolResult, any, error) {
		return es.insert(args), nil, nil
	})

	mcp.AddTool(srv, &mcp.Tool{
		Name:        "insert_at_point",
		Description: "Insert text at a specific row and column position.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args insertAtPointArgs) (*mcp.CallToolResult, any, error) {
		return es.insertAtPoint(args), nil, nil
	})

	mcp.AddTool(srv, &mcp.Tool{
		Name:        "insert_before",
		Description: "Insert text before an AST node identified by a NodeSelector. The indentation is automatically adjusted to match the surrounding context.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args insertBeforeAfterArgs) (*mcp.CallToolResult, any, error) {
		return es.insertBefore(args), nil, nil
	})

	mcp.AddTool(srv, &mcp.Tool{
		Name:        "insert_after",
		Description: "Insert text after an AST node identified by a NodeSelector. The indentation is automatically adjusted to match the surrounding context.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args insertBeforeAfterArgs) (*mcp.CallToolResult, any, error) {
		return es.insertAfter(args), nil, nil
	})

	mcp.AddTool(srv, &mcp.Tool{
		Name:        "replace",
		Description: "Replace the text in the specified byte range [startByte, endByte) with new text.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args replaceArgs) (*mcp.CallToolResult, any, error) {
		return es.replace(args), nil, nil
	})

	mcp.AddTool(srv, &mcp.Tool{
		Name:        "replace_node",
		Description: "Replace the content of an AST node identified by a NodeSelector with new text.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args replaceNodeArgs) (*mcp.CallToolResult, any, error) {
		return es.replaceNode(args), nil, nil
	})

	mcp.AddTool(srv, &mcp.Tool{
		Name:        "delete",
		Description: "Delete the text in the specified byte range [startByte, endByte).",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args deleteRangeArgs) (*mcp.CallToolResult, any, error) {
		return es.delete(args), nil, nil
	})

	mcp.AddTool(srv, &mcp.Tool{
		Name:        "delete_node",
		Description: "Delete an AST node identified by a NodeSelector.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args deleteNodeArgs) (*mcp.CallToolResult, any, error) {
		return es.deleteNode(args), nil, nil
	})

	// --- Validation & Save ---
	mcp.AddTool(srv, &mcp.Tool{
		Name:        "validate",
		Description: "Validate an open document's syntax. Returns syntax errors, line ending style, and trailing newline status. Does not modify the document.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args validateArgs) (*mcp.CallToolResult, any, error) {
		return es.validate(args), nil, nil
	})

	mcp.AddTool(srv, &mcp.Tool{
		Name:        "save",
		Description: "Save an open document to disk. Detects external file modification conflicts and returns a conflict result instead of overwriting. Use force_save to override.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args saveArgs) (*mcp.CallToolResult, any, error) {
		return es.save(args), nil, nil
	})

	mcp.AddTool(srv, &mcp.Tool{
		Name:        "force_save",
		Description: "Force-save an open document to disk, skipping external modification conflict detection.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args saveArgs) (*mcp.CallToolResult, any, error) {
		return es.forceSave(args), nil, nil
	})

	mcp.AddTool(srv, &mcp.Tool{
		Name:        "is_dirty",
		Description: "Check if an open document has unsaved changes.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args uriOnlyArgs) (*mcp.CallToolResult, any, error) {
		return es.isDirty(args), nil, nil
	})
	mcp.AddTool(srv, &mcp.Tool{
		Name:        "list_dirty",
		Description: "List all open documents that have unsaved changes.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args any) (*mcp.CallToolResult, any, error) {
		return es.listDirty(), nil, nil
	})

	// --- Project root stack (push/pop) ---

	mcp.AddTool(srv, &mcp.Tool{
		Name:        "push_project_root",
		Description: "Save the current project root and set a new one. All subsequent relative URI resolution uses the new root. Analogous to cwd_push.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args pushProjectRootArgs) (*mcp.CallToolResult, any, error) {
		return es.pushProjectRoot(args), nil, nil
	})

	mcp.AddTool(srv, &mcp.Tool{
		Name:        "pop_project_root",
		Description: "Restore the previous project root from the stack. Returns the previous root that was active before the last push. Analogous to cwd_pop. Errors if the stack is empty.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args any) (*mcp.CallToolResult, any, error) {
		return es.popProjectRoot(), nil, nil
	})
}

// ─── Command ─────────────────────────────────────────────────────────────────

type serveOptions struct {
	projectRoot        string
	logLevel           string
	allowExternalPaths bool
}

type cmdEditorServe struct {
	global *cmdGlobal
	editor *cmdEditor
	opts   *serveOptions
}

func (c *cmdEditorServe) command() *cobra.Command {
	opts := &serveOptions{}
	c.opts = opts

	cmd := &cobra.Command{}
	cmd.Use = "serve"
	cmd.Short = "Start MCP stdio server for AI code editing"
	cmd.Long = cli.FormatSection(
		color.CyanString("Description:"),
		`Start an MCP stdio server that exposes code editing capabilities
over the Model Context Protocol (stdio transport).

The server reads JSON-RPC requests from stdin and writes responses to stdout.
All diagnostic logs are written to stderr.

Environment:
  SLINGSHOT_PROJECT_ROOT  Project root directory (default: current directory)

Flags:
  --project-root <path>   Project root directory
  --log-level <level>     Log level: debug, info, warn, error (default: warn)
  --allow-external-paths  Allow opening files outside project root`,
	)
	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		// Resolve project root: flag > env > CWD
		if opts.projectRoot == "" {
			opts.projectRoot = os.Getenv("SLINGSHOT_PROJECT_ROOT")
		}
		if opts.projectRoot == "" {
			cwd, err := os.Getwd()
			if err != nil {
				return fmt.Errorf("get current directory: %w", err)
			}
			opts.projectRoot = cwd
		}
		return c.run(cmd.Context())
	}
	cmd.SilenceUsage = true

	cmd.Flags().StringVar(&opts.projectRoot, "project-root", "", "Project root directory")
	cmd.Flags().StringVar(&opts.logLevel, "log-level", "warn", "Log level: debug, info, warn, error")
	cmd.Flags().BoolVar(&opts.allowExternalPaths, "allow-external-paths", false, "Allow opening files outside project root")

	return cmd
}

func (c *cmdEditorServe) run(ctx context.Context) error {
	opts := c.opts

	// 1. Create logger (output to stderr only)
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
		Level: parseLogLevel(opts.logLevel),
	}))
	slog.SetDefault(logger)

	// 2. Create Editor instance
	ed := editor.NewEditor(opts.projectRoot)

	// 3. Create MCP Server
	srv := mcp.NewServer(&mcp.Implementation{
		Name:    "slingshot-editor",
		Version: "0.1.0",
	}, &mcp.ServerOptions{
		Logger: logger,
	})

	// 4. Register all tools
	es := &editorServer{ed: ed, opts: opts}
	registerAllTools(srv, es)

	// 5. Connect stdio transport
	logger.Info("editor MCP server starting",
		"projectRoot", opts.projectRoot,
		"logLevel", opts.logLevel,
		"allowExternalPaths", opts.allowExternalPaths,
	)
	session, err := srv.Connect(ctx, &mcp.StdioTransport{}, nil)
	if err != nil {
		return fmt.Errorf("connect: %w", err)
	}

	// 6. Wait for shutdown
	logger.Info("editor MCP server started")
	return session.Wait()
}

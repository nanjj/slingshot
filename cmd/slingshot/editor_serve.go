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

// ─── Consolidated input structs ──────────────────────────────────────────────

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

type editorGetNodeArgs struct {
	URI       string `json:"uri"`
	Scope     string `json:"scope,omitempty"` // "pos" (default), "point", "range", "descendants"
	Pos       uint32 `json:"pos,omitempty"`
	Row       uint32 `json:"row,omitempty"`
	Col       uint32 `json:"col,omitempty"`
	StartByte uint32 `json:"startByte,omitempty"`
	EndByte   uint32 `json:"endByte,omitempty"`
}

type editorGetTextArgs struct {
	URI       string `json:"uri"`
	By        string `json:"by,omitempty"` // "range" (default), "line"
	Line      uint32 `json:"line,omitempty"`
	StartByte uint32 `json:"startByte,omitempty"`
	EndByte   uint32 `json:"endByte,omitempty"`
}

type queryArgs struct {
	URI     string `json:"uri"`
	Pattern string `json:"pattern"`
}

type editorInsertArgs struct {
	URI      string              `json:"uri"`
	Text     string              `json:"text"`
	Position string              `json:"position,omitempty"` // "pos" (default), "point", "before", "after"
	Pos      uint32              `json:"pos,omitempty"`
	Row      uint32              `json:"row,omitempty"`
	Col      uint32              `json:"col,omitempty"`
	Selector editor.NodeSelector `json:"selector,omitempty"`
}

type editorReplaceArgs struct {
	URI       string              `json:"uri"`
	Text      string              `json:"text"`
	Target    string              `json:"target,omitempty"` // "range" (default), "node"
	StartByte uint32              `json:"startByte,omitempty"`
	EndByte   uint32              `json:"endByte,omitempty"`
	Selector  editor.NodeSelector `json:"selector,omitempty"`
}

type editorDeleteArgs struct {
	URI       string              `json:"uri"`
	Target    string              `json:"target,omitempty"` // "range" (default), "node"
	StartByte uint32              `json:"startByte,omitempty"`
	EndByte   uint32              `json:"endByte,omitempty"`
	Selector  editor.NodeSelector `json:"selector,omitempty"`
}

type editorSaveArgs struct {
	URI   string `json:"uri"`
	Force bool   `json:"force,omitempty"`
}

type editorValidateArgs struct {
	URI          string `json:"uri"`
	IncludeDirty bool   `json:"includeDirty,omitempty"`
}

// ─── Response types ─────────────────────────────────────────────────────────

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

// validateResponse combines validation result with optional dirty info.
type validateResponse struct {
	Valid        bool                 `json:"valid"`
	SyntaxErrors []editor.SyntaxError `json:"syntaxErrors,omitempty"`
	LineEnding   string               `json:"lineEnding"`
	TrailingNL   bool                 `json:"trailingNewline"`
	Dirty        *bool                `json:"dirty,omitempty"`
	DirtyURIs    []string             `json:"dirtyURIs,omitempty"`
}

// ─── MCP helper utilities ───────────────────────────────────────────────────

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

// editorServer holds the shared state for all tool handlers.
type editorServer struct {
	mgr  *editor.EditorManager
	opts *serveOptions
}

// --- Document lifecycle ---

func (es *editorServer) openDocument(args openDocumentArgs) *mcp.CallToolResult {
	var source []byte
	if args.Source != "" {
		source = []byte(args.Source)
	}
	if err := es.mgr.Current().OpenDocument(args.URI, source, args.Language); err != nil {
		return errorResult(fmt.Errorf("open document: %w", err))
	}
	return jsonResult(map[string]bool{"success": true})
}

func (es *editorServer) closeDocument(args closeDocumentArgs) *mcp.CallToolResult {
	if err := es.mgr.Current().CloseDocument(args.URI); err != nil {
		return errorResult(fmt.Errorf("close document: %w", err))
	}
	return jsonResult(map[string]bool{"success": true})
}

// --- Read operations ---

func (es *editorServer) getStructure(args getStructureArgs) *mcp.CallToolResult {
	maxDepth := args.MaxDepth
	if maxDepth == 0 {
		maxDepth = -1 // zero in JSON = not specified, default to -1 (recursive)
	}
	maxChildren := args.MaxChildren
	if maxChildren == 0 {
		maxChildren = -1
	}
	nodeInfo, err := es.mgr.Current().GetStructure(args.URI, maxDepth, maxChildren)
	if err != nil {
		return errorResult(fmt.Errorf("get structure: %w", err))
	}
	return jsonResult(nodeInfo)
}

func (es *editorServer) getNode(args editorGetNodeArgs) *mcp.CallToolResult {
	switch args.Scope {
	case "point":
		nodeInfo, err := es.mgr.Current().GetNodeAtPoint(args.URI, args.Row, args.Col)
		if err != nil {
			return errorResult(fmt.Errorf("get node at point: %w", err))
		}
		return jsonResult(nodeInfo)
	case "range":
		nodeInfo, err := es.mgr.Current().GetNodeAtRange(args.URI, args.StartByte, args.EndByte)
		if err != nil {
			return errorResult(fmt.Errorf("get node at range: %w", err))
		}
		return jsonResult(nodeInfo)
	case "descendants":
		infos, err := es.mgr.Current().GetDescendantsAt(args.URI, args.Pos)
		if err != nil {
			return errorResult(fmt.Errorf("get descendants: %w", err))
		}
		return jsonResult(infos)
	default: // "pos"
		nodeInfo, err := es.mgr.Current().GetNode(args.URI, args.Pos)
		if err != nil {
			return errorResult(fmt.Errorf("get node: %w", err))
		}
		return jsonResult(nodeInfo)
	}
}

func (es *editorServer) getText(args editorGetTextArgs) *mcp.CallToolResult {
	switch args.By {
	case "line":
		text, err := es.mgr.Current().GetLine(args.URI, args.Line)
		if err != nil {
			return errorResult(fmt.Errorf("get line: %w", err))
		}
		return jsonResult(map[string]string{"text": text})
	default: // "range"
		text, err := es.mgr.Current().GetText(args.URI, args.StartByte, args.EndByte)
		if err != nil {
			return errorResult(fmt.Errorf("get text: %w", err))
		}
		return jsonResult(map[string]string{"text": text})
	}
}

func (es *editorServer) query(args queryArgs) *mcp.CallToolResult {
	results, err := es.mgr.Current().Query(args.URI, args.Pattern)
	if err != nil {
		return errorResult(fmt.Errorf("query: %w", err))
	}
	return jsonResult(results)
}

// --- Edit operations ---

func (es *editorServer) insert(args editorInsertArgs) *mcp.CallToolResult {
	var editResult *editor.EditResult
	var err error

	switch args.Position {
	case "point":
		editResult, err = es.mgr.Current().InsertAtPoint(args.URI, args.Row, args.Col, args.Text)
		if err != nil {
			return errorResult(fmt.Errorf("insert at point: %w", err))
		}
	case "before":
		editResult, err = es.mgr.Current().InsertBefore(args.URI, args.Selector, args.Text)
		if err != nil {
			return errorResult(fmt.Errorf("insert before: %w", err))
		}
	case "after":
		editResult, err = es.mgr.Current().InsertAfter(args.URI, args.Selector, args.Text)
		if err != nil {
			return errorResult(fmt.Errorf("insert after: %w", err))
		}
	default: // "pos"
		editResult, err = es.mgr.Current().Insert(args.URI, args.Pos, args.Text)
		if err != nil {
			return errorResult(fmt.Errorf("insert: %w", err))
		}
	}
	return jsonResult(editResultToResponse(editResult))
}

func (es *editorServer) replace(args editorReplaceArgs) *mcp.CallToolResult {
	var editResult *editor.EditResult
	var err error

	switch args.Target {
	case "node":
		editResult, err = es.mgr.Current().ReplaceNode(args.URI, args.Selector, args.Text)
		if err != nil {
			return errorResult(fmt.Errorf("replace node: %w", err))
		}
	default: // "range"
		editResult, err = es.mgr.Current().Replace(args.URI, args.StartByte, args.EndByte, args.Text)
		if err != nil {
			return errorResult(fmt.Errorf("replace: %w", err))
		}
	}
	return jsonResult(editResultToResponse(editResult))
}

func (es *editorServer) delete(args editorDeleteArgs) *mcp.CallToolResult {
	var editResult *editor.EditResult
	var err error

	switch args.Target {
	case "node":
		editResult, err = es.mgr.Current().DeleteNode(args.URI, args.Selector)
		if err != nil {
			return errorResult(fmt.Errorf("delete node: %w", err))
		}
	default: // "range"
		editResult, err = es.mgr.Current().Delete(args.URI, args.StartByte, args.EndByte)
		if err != nil {
			return errorResult(fmt.Errorf("delete: %w", err))
		}
	}
	return jsonResult(editResultToResponse(editResult))
}

// --- Save & Validate ---

func (es *editorServer) save(args editorSaveArgs) *mcp.CallToolResult {
	var result *editor.SaveResult
	var err error

	if args.Force {
		result, err = es.mgr.Current().ForceSave(args.URI)
		if err != nil {
			return errorResult(fmt.Errorf("force save: %w", err))
		}
	} else {
		result, err = es.mgr.Current().Save(args.URI)
		if err != nil {
			return errorResult(fmt.Errorf("save: %w", err))
		}
	}
	return jsonResult(saveResultToResponse(result))
}

func (es *editorServer) validate(args editorValidateArgs) *mcp.CallToolResult {
	valResult, err := es.mgr.Current().Validate(args.URI)
	if err != nil {
		return errorResult(fmt.Errorf("validate: %w", err))
	}

	resp := validateResponse{
		Valid:        valResult.Valid,
		SyntaxErrors: valResult.SyntaxErrors,
		LineEnding:   valResult.LineEnding,
		TrailingNL:   valResult.TrailingNewline,
	}

	if args.IncludeDirty {
		dirty, err := es.mgr.Current().IsDirty(args.URI)
		if err == nil {
			resp.Dirty = &dirty
		}
		dirtyURIs := es.mgr.Current().DirtyDocuments()
		resp.DirtyURIs = dirtyURIs
	}

	return jsonResult(resp)
}

func (es *editorServer) getProjectRoot() *mcp.CallToolResult {
	return jsonResult(map[string]string{
		"projectRoot": es.mgr.Current().ProjectRoot(),
	})
}

// --- Helper functions ---

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

// --- Tool Registration ---

func registerAllTools(srv *mcp.Server, es *editorServer) {
	// --- Document lifecycle ---
	mcp.AddTool(srv, &mcp.Tool{
		Name:        "editor_open_document",
		Description: "Open a document for editing. If a document with the same URI is already open, it is closed first. Supports file:// URIs and scratch:// URIs for ephemeral snippets.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args openDocumentArgs) (*mcp.CallToolResult, any, error) {
		return es.openDocument(args), nil, nil
	})

	mcp.AddTool(srv, &mcp.Tool{
		Name:        "editor_close_document",
		Description: "Close a document and release its tree-sitter resources. Unsaved changes are discarded.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args closeDocumentArgs) (*mcp.CallToolResult, any, error) {
		return es.closeDocument(args), nil, nil
	})

	// --- Read operations ---
	mcp.AddTool(srv, &mcp.Tool{
		Name:        "editor_get_structure",
		Description: "Get the hierarchical code structure (syntax tree) of an open document. Returns nested NodeInfo with type, byte range, and child nodes. Use maxDepth and maxChildren to limit output size. Default: recursive, unlimited children.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args getStructureArgs) (*mcp.CallToolResult, any, error) {
		return es.getStructure(args), nil, nil
	})

	mcp.AddTool(srv, &mcp.Tool{
		Name:        "editor_get_node",
		Description: "Get AST nodes from a document. The 'scope' parameter selects the mode:\n" +
			"  - 'pos' (default): Get the smallest AST node at a byte offset.\n" +
			"  - 'point': Get the smallest AST node at a row/col position.\n" +
			"  - 'range': Get the smallest AST node covering [startByte, endByte).\n" +
			"  - 'descendants': Get all ancestor nodes at a byte position, innermost to root.\n" +
			"Returns node type, position, source text, and children.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args editorGetNodeArgs) (*mcp.CallToolResult, any, error) {
		return es.getNode(args), nil, nil
	})

	mcp.AddTool(srv, &mcp.Tool{
		Name:        "editor_get_text",
		Description: "Get source text from a document. Use 'by' to select mode:\n" +
			"  - 'range' (default): Get text in byte range [startByte, endByte).\n" +
			"  - 'line': Get the content of a specific line (0-indexed, trailing newline stripped).",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args editorGetTextArgs) (*mcp.CallToolResult, any, error) {
		return es.getText(args), nil, nil
	})

	mcp.AddTool(srv, &mcp.Tool{
		Name:        "editor_query",
		Description: "Execute a tree-sitter S-expression query (.scm pattern) against the document's syntax tree. Returns matching captures grouped by capture name.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args queryArgs) (*mcp.CallToolResult, any, error) {
		return es.query(args), nil, nil
	})

	// --- Edit operations ---
	mcp.AddTool(srv, &mcp.Tool{
		Name:        "editor_insert",
		Description: "Insert text into a document. Use 'position' to select mode:\n" +
			"  - 'pos' (default): Insert at a byte offset.\n" +
			"  - 'point': Insert at a row/col position.\n" +
			"  - 'before': Insert before an AST node (identified by NodeSelector).\n" +
			"  - 'after': Insert after an AST node (identified by NodeSelector).\n" +
			"Indentation is auto-adjusted for 'before'/'after' modes.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args editorInsertArgs) (*mcp.CallToolResult, any, error) {
		return es.insert(args), nil, nil
	})

	mcp.AddTool(srv, &mcp.Tool{
		Name:        "editor_replace",
		Description: "Replace text in a document. Use 'target' to select mode:\n" +
			"  - 'range' (default): Replace text in byte range [startByte, endByte).\n" +
			"  - 'node': Replace the content of an AST node (identified by NodeSelector).",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args editorReplaceArgs) (*mcp.CallToolResult, any, error) {
		return es.replace(args), nil, nil
	})

	mcp.AddTool(srv, &mcp.Tool{
		Name:        "editor_delete",
		Description: "Delete text from a document. Use 'target' to select mode:\n" +
			"  - 'range' (default): Delete text in byte range [startByte, endByte).\n" +
			"  - 'node': Delete an AST node (identified by NodeSelector).",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args editorDeleteArgs) (*mcp.CallToolResult, any, error) {
		return es.delete(args), nil, nil
	})

	// --- Save & Validate ---
	mcp.AddTool(srv, &mcp.Tool{
		Name:        "editor_save",
		Description: "Save an open document to disk. By default detects external file modification conflicts. Set 'force' to true to skip conflict detection and overwrite.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args editorSaveArgs) (*mcp.CallToolResult, any, error) {
		return es.save(args), nil, nil
	})

	mcp.AddTool(srv, &mcp.Tool{
		Name:        "editor_validate",
		Description: "Validate an open document's syntax. Returns syntax errors, line ending style, and trailing newline status. Set 'includeDirty' to true to also return dirty status and list of all dirty document URIs.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args editorValidateArgs) (*mcp.CallToolResult, any, error) {
		return es.validate(args), nil, nil
	})

	// --- Project root ---
	mcp.AddTool(srv, &mcp.Tool{
		Name:        "editor_get_project_root",
		Description: "Get the current project root directory. Returns the absolute path of the active editor's project root.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args any) (*mcp.CallToolResult, any, error) {
		return es.getProjectRoot(), nil, nil
	})
}

// --- Command ---

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

	// 2. Create EditorManager (manages per-project-root Editor instances)
	mgr, err := editor.NewEditorManager(opts.projectRoot)
	if err != nil {
		return fmt.Errorf("create editor manager: %w", err)
	}

	// 3. Create MCP Server
	srv := mcp.NewServer(&mcp.Implementation{
		Name:    "slingshot-editor",
		Version: "0.2.0",
	}, &mcp.ServerOptions{
		Logger: logger,
	})

	// 4. Register all tools
	es := &editorServer{mgr: mgr, opts: opts}
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

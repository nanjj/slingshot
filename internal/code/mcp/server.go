// Package mcp provides MCP tool handlers for code intelligence.
//
// It connects the base (storage/graph) and lsp (analysis) packages
// to the Model Context Protocol, exposing 27 tools for code search,
// navigation, editing, referencing, analysis, and project management.
package mcp

import (
	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/nanjj/slingshot/internal/code/base"
	"github.com/nanjj/slingshot/internal/code/lsp"
	"github.com/nanjj/slingshot/internal/code/edit"
)

// Server manages MCP tool handlers for code intelligence.
//
// It connects the base (storage/graph) and lsp (analysis) packages
// to the Model Context Protocol via go-sdk.
type Server struct {
	store    *base.Store
	analyzer *lsp.Analyzer
	ed       *edit.Editor
	opts     *Options
}

// Options configures the MCP server.
type Options struct {
	// ProjectRoot is the current workspace root for file operations.
	ProjectRoot string
	// DBPath is the path to the SQLite code graph database.
	DBPath string
}

// NewServer creates a new MCP server with the given store, analyzer, and options.
func NewServer(store *base.Store, analyzer *lsp.Analyzer, opts *Options) *Server {
	ed := edit.NewEditor(opts.ProjectRoot)
	return &Server{
		store:    store,
		analyzer: analyzer,
		ed:       ed,
		opts:     opts,
	}
}

// RegisterAll registers all 27 code intelligence tools on the given MCP server.
func (s *Server) RegisterAll(srv *mcp.Server) {
	// ─── Search & Navigation (7) ─────────────────────────────────────────
	registerSearchGraph(srv, s)
	registerSearchCode(srv, s)
	registerGetArchitecture(srv, s)
	registerGetCodeSnippet(srv, s)
	registerGetGraphSchema(srv, s)
	registerTracePath(srv, s)
	registerDetectChanges(srv, s)

	// ─── Index Management (5) ────────────────────────────────────────────
	registerIndexRepository(srv, s)
	registerIndexStatus(srv, s)
	registerListProjects(srv, s)
	registerDeleteProject(srv, s)
	registerQueryGraph(srv, s)

	// ─── AST Analysis / Editor (6) ───────────────────────────────────────
	registerGetStructure(srv, s)
	registerGetNode(srv, s)
	registerGetDefinitions(srv, s)
	registerGetText(srv, s)
	registerValidate(srv, s)
	registerQueryAST(srv, s)

	// ─── Memo & ADR Management (3) ───────────────────────────────────────
	registerSearchMemos(srv, s)
	registerSaveMemo(srv, s)
	registerManageADR(srv, s)

	// ─── Project / Configuration (2) + Ingest ────────────────────────────
	registerGetProjectRoot(srv, s)
	registerOpenProject(srv, s)
	registerIngestTraces(srv, s)

	// ─── Edit & Locate (3, Phase 1) ─────────────────────────────────────
	registerCodeEdit(srv, s)
	registerCodeEditBody(srv, s)
	registerCodeLocate(srv, s)

	// ─── Analysis & References (2, Phase 2) ─────────────────────────────
	registerCodeFindReferences(srv, s)
	registerCodeAnalysis(srv, s)
}

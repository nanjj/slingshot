// Package mcp provides MCP tool handlers for code intelligence.
//
// It connects the base (storage/graph) and lsp (analysis) packages
// to the Model Context Protocol, exposing tools for code search,
// navigation, editing, referencing, analysis, and project management.
package mcp

import (
	"fmt"
	"strings"
	"sync"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/nanjj/slingshot/internal/code/base"
	"github.com/nanjj/slingshot/internal/code/edit"
	"github.com/nanjj/slingshot/internal/code/lsp"
)

// Server manages MCP tool handlers for code intelligence.
type Server struct {
	store    *base.Store
	analyzer *lsp.Analyzer
	ed       *edit.Editor
	opts     *Options

	// mu guards currentProject and field swaps (open_project, editor root).
	mu             sync.RWMutex
	currentProject string // project bound via open_project or auto-derived
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
	s := &Server{
		store:    store,
		analyzer: analyzer,
		ed:       edit.NewEditor(opts.ProjectRoot),
		opts:     opts,
	}
	// Auto-bind the current project at startup so graph tools work without
	// an explicit project argument when the workspace root is indexed.
	if opts.ProjectRoot != "" {
		if info, err := store.ResolveProject(opts.ProjectRoot); err == nil {
			s.currentProject = info.Name
		}
	}
	return s
}

// editor returns the active editor under the read lock.
func (s *Server) editor() *edit.Editor {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.ed
}

// currentRoot returns the active project root under the read lock.
func (s *Server) currentRoot() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.opts.ProjectRoot
}

// resolveProject resolves the effective project for a tool call.
//
// Resolution order:
//  1. explicit identifier (name / root path / basename / unique substring)
//  2. project bound by open_project (or auto-bound at startup)
//  3. the workspace root (opts.ProjectRoot), auto-bound on first use
//
// The resolved name is memoized so subsequent calls may omit the project
// argument entirely. On failure the error includes the list of available
// projects so the LLM can pick a valid identifier.
func (s *Server) resolveProject(explicit string) (*base.ProjectInfo, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	var (
		info *base.ProjectInfo
		err  error
	)
	switch {
	case explicit != "":
		info, err = s.store.ResolveProject(explicit)
	case s.currentProject != "":
		info, err = s.store.ProjectStatus(s.currentProject)
	case s.opts.ProjectRoot != "":
		info, err = s.store.ResolveProject(s.opts.ProjectRoot)
	default:
		err = fmt.Errorf("project is required — use open_project to bind one, or pass project explicitly")
	}

	if err != nil {
		return nil, s.withAvailableProjects(err)
	}
	if info == nil {
		return nil, fmt.Errorf("project resolved to empty info")
	}
	s.currentProject = info.Name
	return info, nil
}

// withAvailableProjects appends the list of indexed projects to an error so
// the LLM can pick a valid project identifier on its next call.
func (s *Server) withAvailableProjects(err error) error {
	projects, lerr := s.store.ListProjects()
	if lerr != nil || len(projects) == 0 {
		return fmt.Errorf("%v. No projects are indexed — run index_repository to index one", err)
	}
	var b strings.Builder
	b.WriteString(err.Error())
	b.WriteString(". Available projects: ")
	for i, p := range projects {
		if i > 0 {
			b.WriteString(", ")
		}
		fmt.Fprintf(&b, "%s (%s)", p.Name, p.Root)
	}
	b.WriteString(". Use open_project to bind one, or pass project explicitly.")
	return fmt.Errorf("%s", b.String())
}

// bindProject switches the active project to an indexed project and points
// the editor at its root.
func (s *Server) bindProject(info *base.ProjectInfo) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.currentProject = info.Name
	s.opts.ProjectRoot = info.Root
	s.ed = edit.NewEditor(info.Root)
}

// setProjectRoot switches the workspace root (open_project by path). The
// editor moves to the new root; the bound project is re-derived if the new
// root is indexed, otherwise cleared.
func (s *Server) setProjectRoot(root string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.opts.ProjectRoot = root
	s.ed = edit.NewEditor(root)
	s.currentProject = ""
	if info, err := s.store.ResolveProject(root); err == nil {
		s.currentProject = info.Name
	}
}

// firstNonEmpty returns the first non-empty string, or "".
func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}

// RegisterAll registers all code intelligence tools on the given MCP server.
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

	// ─── AST Analysis / Editor (5) ───────────────────────────────────────
	registerGetStructure(srv, s)
	registerGetDefinitions(srv, s)
	registerGetText(srv, s)
	registerValidate(srv, s)

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

package mcp

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/nanjj/slingshot/internal/code/base"
)

// ─── Argument structs ──────────────────────────────────────────────────────────

type indexRepositoryArgs struct {
	RepoPath        string   `json:"repoPath"`
	Mode            string   `json:"mode,omitempty"` // full, moderate, fast
	Persistence     bool     `json:"persistence,omitempty"`
	TargetProjects  []string `json:"targetProjects,omitempty"`
}

type indexStatusArgs struct {
	Project string `json:"project"`
}

type listProjectsArgs struct{}

type deleteProjectArgs struct {
	Project string `json:"project"`
}

type queryGraphArgs struct {
	Project string `json:"project"`
	Query   string `json:"query"`
	MaxRows int    `json:"maxRows,omitempty"`
}

// ─── index_repository ─────────────────────────────────────────────────────────

func registerIndexRepository(srv *mcp.Server, s *Server) {
	mcp.AddTool(srv, &mcp.Tool{
		Name:        "index_repository",
		Description: "Index a repository into the knowledge graph. Modes: full (all files + similarity), moderate (filtered + similarity), fast (filtered, no similarity). Cross-repo mode matches routes/channels across projects.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args indexRepositoryArgs) (*mcp.CallToolResult, any, error) {
		return s.handleIndexRepository(args), nil, nil
	})
}

func (s *Server) handleIndexRepository(args indexRepositoryArgs) *mcp.CallToolResult {
	if args.RepoPath == "" {
		return errorResult(fmt.Errorf("repoPath is required"))
	}

	// Derive project name from repo path
	projectName := filepath.Base(args.RepoPath)

	// Resolve index mode
	mode := base.IndexModeFull
	switch args.Mode {
	case "moderate":
		mode = base.IndexModeModerate
	case "fast":
		mode = base.IndexModeFast
	}

	result, err := s.store.IndexProject(args.RepoPath, projectName, mode)
	if err != nil {
		return errorResult(fmt.Errorf("index repository: %w", err))
	}

	return jsonResult(result)
}

// ─── index_status ─────────────────────────────────────────────────────────────

func registerIndexStatus(srv *mcp.Server, s *Server) {
	mcp.AddTool(srv, &mcp.Tool{
		Name:        "index_status",
		Description: "Get the indexing status of a project.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args indexStatusArgs) (*mcp.CallToolResult, any, error) {
		return s.handleIndexStatus(args), nil, nil
	})
}

func (s *Server) handleIndexStatus(args indexStatusArgs) *mcp.CallToolResult {
	if args.Project == "" {
		return errorResult(fmt.Errorf("project is required"))
	}

	info, err := s.store.ProjectStatus(args.Project)
	if err != nil {
		return errorResult(fmt.Errorf("project status: %w", err))
	}

	return jsonResult(info)
}

// ─── list_projects ────────────────────────────────────────────────────────────

func registerListProjects(srv *mcp.Server, s *Server) {
	mcp.AddTool(srv, &mcp.Tool{
		Name:        "list_projects",
		Description: "List all indexed projects.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args listProjectsArgs) (*mcp.CallToolResult, any, error) {
		return s.handleListProjects(), nil, nil
	})
}

func (s *Server) handleListProjects() *mcp.CallToolResult {
	projects, err := s.store.ListProjects()
	if err != nil {
		return errorResult(fmt.Errorf("list projects: %w", err))
	}
	if projects == nil {
		projects = []base.ProjectInfo{}
	}
	return jsonResult(projects)
}

// ─── delete_project ───────────────────────────────────────────────────────────

func registerDeleteProject(srv *mcp.Server, s *Server) {
	mcp.AddTool(srv, &mcp.Tool{
		Name:        "delete_project",
		Description: "Delete a project from the index.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args deleteProjectArgs) (*mcp.CallToolResult, any, error) {
		return s.handleDeleteProject(args), nil, nil
	})
}

func (s *Server) handleDeleteProject(args deleteProjectArgs) *mcp.CallToolResult {
	if args.Project == "" {
		return errorResult(fmt.Errorf("project is required"))
	}

	if err := s.store.DeleteProject(args.Project); err != nil {
		return errorResult(fmt.Errorf("delete project: %w", err))
	}

	return jsonResult(map[string]bool{"success": true})
}

// ─── query_graph ──────────────────────────────────────────────────────────────

func registerQueryGraph(srv *mcp.Server, s *Server) {
	mcp.AddTool(srv, &mcp.Tool{
		Name:        "query_graph",
		Description: "Execute a SQL query against the knowledge graph for complex multi-hop patterns, aggregations, and cross-service analysis.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args queryGraphArgs) (*mcp.CallToolResult, any, error) {
		return s.handleQueryGraph(args), nil, nil
	})
}

func (s *Server) handleQueryGraph(args queryGraphArgs) *mcp.CallToolResult {
	if args.Query == "" {
		return errorResult(fmt.Errorf("query is required"))
	}

	results, err := s.store.QueryGraph(args.Query)
	if err != nil {
		return errorResult(fmt.Errorf("query graph: %w", err))
	}
	if results == nil {
		results = []map[string]any{}
	}

	return jsonResult(map[string]any{
		"results": results,
		"total":   len(results),
	})
}

package mcp

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/nanjj/clog"
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
		return s.handleIndexRepository(ctx, args), nil, nil
	})
}

func (s *Server) handleIndexRepository(ctx context.Context, args indexRepositoryArgs) *mcp.CallToolResult {
	span, ctx := clog.StartSpanFromContext(ctx, "index_repository")
	defer span.Finish()
	clog.Info(ctx, "index_repository", "repoPath", args.RepoPath, "mode", args.Mode, "persistence", args.Persistence)

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
		clog.Error(ctx, "error", "error", err.Error())
		return errorResult(fmt.Errorf("index repository: %w", err))
	}

	clog.Info(ctx, "index_repository_result", "project", projectName, "mode", args.Mode)
	return jsonResult(result)
}

// ─── index_status ─────────────────────────────────────────────────────────────

func registerIndexStatus(srv *mcp.Server, s *Server) {
	mcp.AddTool(srv, &mcp.Tool{
		Name:        "index_status",
		Description: "Get the indexing status of a project.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args indexStatusArgs) (*mcp.CallToolResult, any, error) {
		return s.handleIndexStatus(ctx, args), nil, nil
	})
}

func (s *Server) handleIndexStatus(ctx context.Context, args indexStatusArgs) *mcp.CallToolResult {
	span, ctx := clog.StartSpanFromContext(ctx, "index_status")
	defer span.Finish()
	clog.Info(ctx, "index_status", "project", args.Project)

	if args.Project == "" {
		return errorResult(fmt.Errorf("project is required"))
	}

	info, err := s.store.ProjectStatus(args.Project)
	if err != nil {
		clog.Error(ctx, "error", "error", err.Error())
		return errorResult(fmt.Errorf("project status: %w", err))
	}

	clog.Info(ctx, "index_status_result", "indexed", info.NodeCount > 0)
	return jsonResult(info)
}

// ─── list_projects ────────────────────────────────────────────────────────────

func registerListProjects(srv *mcp.Server, s *Server) {
	mcp.AddTool(srv, &mcp.Tool{
		Name:        "list_projects",
		Description: "List all indexed projects.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args listProjectsArgs) (*mcp.CallToolResult, any, error) {
		return s.handleListProjects(ctx), nil, nil
	})
}

func (s *Server) handleListProjects(ctx context.Context) *mcp.CallToolResult {
	span, ctx := clog.StartSpanFromContext(ctx, "list_projects")
	defer span.Finish()

	projects, err := s.store.ListProjects()
	if err != nil {
		clog.Error(ctx, "error", "error", err.Error())
		return errorResult(fmt.Errorf("list projects: %w", err))
	}
	if projects == nil {
		projects = []base.ProjectInfo{}
	}
	clog.Info(ctx, "list_projects_result", "count", len(projects))
	return jsonResult(projects)
}

// ─── delete_project ───────────────────────────────────────────────────────────

func registerDeleteProject(srv *mcp.Server, s *Server) {
	mcp.AddTool(srv, &mcp.Tool{
		Name:        "delete_project",
		Description: "Delete a project from the index.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args deleteProjectArgs) (*mcp.CallToolResult, any, error) {
		return s.handleDeleteProject(ctx, args), nil, nil
	})
}

func (s *Server) handleDeleteProject(ctx context.Context, args deleteProjectArgs) *mcp.CallToolResult {
	span, ctx := clog.StartSpanFromContext(ctx, "delete_project")
	defer span.Finish()
	clog.Info(ctx, "delete_project", "project", args.Project)

	if args.Project == "" {
		return errorResult(fmt.Errorf("project is required"))
	}

	if err := s.store.DeleteProject(args.Project); err != nil {
		clog.Error(ctx, "error", "error", err.Error())
		return errorResult(fmt.Errorf("delete project: %w", err))
	}

	clog.Info(ctx, "delete_project_result", "success", true)
	return jsonResult(map[string]bool{"success": true})
}

// ─── query_graph ──────────────────────────────────────────────────────────────

func registerQueryGraph(srv *mcp.Server, s *Server) {
	mcp.AddTool(srv, &mcp.Tool{
		Name:        "query_graph",
		Description: "Execute a SQL query against the knowledge graph for complex multi-hop patterns, aggregations, and cross-service analysis.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args queryGraphArgs) (*mcp.CallToolResult, any, error) {
		return s.handleQueryGraph(ctx, args), nil, nil
	})
}

func (s *Server) handleQueryGraph(ctx context.Context, args queryGraphArgs) *mcp.CallToolResult {
	span, ctx := clog.StartSpanFromContext(ctx, "query_graph")
	defer span.Finish()
	clog.Info(ctx, "query_graph", "project", args.Project, "queryLen", len(args.Query))

	if args.Query == "" {
		return errorResult(fmt.Errorf("query is required"))
	}

	results, err := s.store.QueryGraph(args.Query)
	if err != nil {
		clog.Error(ctx, "error", "error", err.Error())
		return errorResult(fmt.Errorf("query graph: %w", err))
	}
	if results == nil {
		results = []map[string]any{}
	}

	clog.Info(ctx, "query_graph_result", "total", len(results))
	return jsonResult(map[string]any{
		"results": results,
		"total":   len(results),
	})
}

// Package mcp — project lifecycle: get_project_root, open_project.
package mcp

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/nanjj/clog"
)

// ─── Argument structs ──────────────────────────────────────────────────────────

type getProjectRootArgs struct{}

type openProjectArgs struct {
	// Path is the project root directory (absolute or relative), or the name
	// of an indexed project.
	Path string `json:"path,omitempty"`
	// Project is an alias for Path — LLMs naturally pass the project name.
	Project string `json:"project,omitempty"`
}

// ─── get_project_root ─────────────────────────────────────────────────────────

func registerGetProjectRoot(srv *mcp.Server, s *Server) {
	mcp.AddTool(srv, &mcp.Tool{
		Name:        "get_project_root",
		Description: "Get the active project root directory and the bound indexed project (if any).",
		InputSchema: lenientSchema[getProjectRootArgs](),
	}, func(ctx context.Context, req *mcp.CallToolRequest, args getProjectRootArgs) (*mcp.CallToolResult, any, error) {
		return s.handleGetProjectRoot(ctx), nil, nil
	})
}

func (s *Server) handleGetProjectRoot(ctx context.Context) *mcp.CallToolResult {
	span, ctx := clog.StartSpanFromContext(ctx, "get_project_root")
	defer span.Finish()

	s.mu.RLock()
	root, project := s.opts.ProjectRoot, s.currentProject
	s.mu.RUnlock()

	clog.Info(ctx, "get_project_root", "projectRoot", root, "project", project)
	return jsonResult(map[string]string{
		"projectRoot": root,
		"project":     project,
	})
}

// ─── open_project ──────────────────────────────────────────────────────────────

func registerOpenProject(srv *mcp.Server, s *Server) {
	mcp.AddTool(srv, &mcp.Tool{
		Name:        "open_project",
		Description: "Bind a project for the rest of the session: subsequent tool calls may omit the project argument. Accepts an indexed project name, a root path, or a path suffix (e.g. 'dscli', '/home/me/src/dscli', 'me/src/dscli'). If the path is not indexed yet, the workspace root is switched and you should run index_repository.",
		InputSchema: lenientSchema[openProjectArgs](),
	}, func(ctx context.Context, req *mcp.CallToolRequest, args openProjectArgs) (*mcp.CallToolResult, any, error) {
		return s.handleOpenProject(ctx, args), nil, nil
	})
}

func (s *Server) handleOpenProject(ctx context.Context, args openProjectArgs) *mcp.CallToolResult {
	span, ctx := clog.StartSpanFromContext(ctx, "open_project")
	defer span.Finish()

	identifier := firstNonEmpty(args.Path, args.Project)
	if identifier == "" {
		return errorResult(fmt.Errorf("path or project is required — pass an indexed project name, a root path, or a path suffix"))
	}
	clog.Info(ctx, "open_project", "identifier", identifier)

	// 1. If the identifier names an indexed project, bind it directly.
	if info, err := s.store.ResolveProject(identifier); err == nil {
		s.bindProject(info)
		clog.Info(ctx, "open_project_result", "project", info.Name, "root", info.Root)
		return jsonResult(map[string]any{
			"project":     info.Name,
			"projectRoot": info.Root,
			"indexed":     true,
		})
	}

	// 2. Otherwise treat it as a filesystem path (absolute or relative).
	abs, err := filepath.Abs(identifier)
	if err != nil {
		return errorResult(fmt.Errorf("resolve path %q: %w", identifier, err))
	}
	st, err := os.Stat(abs)
	if err != nil || !st.IsDir() {
		return errorResult(fmt.Errorf("project %q not found in index and %q is not a directory", identifier, abs))
	}
	s.setProjectRoot(abs)
	clog.Info(ctx, "open_project_result", "projectRoot", abs, "indexed", false)
	return jsonResult(map[string]any{
		"projectRoot": abs,
		"indexed":     false,
		"note":        "workspace root set but project not indexed — run index_repository to enable graph tools",
	})
}

package mcp

import (
	"context"
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/nanjj/clog"
)

// ─── Argument structs ──────────────────────────────────────────────────────────

type getProjectRootArgs struct{}

type openProjectArgs struct {
	Path string `json:"path"`
}

// ─── get_project_root ─────────────────────────────────────────────────────────

func registerGetProjectRoot(srv *mcp.Server, s *Server) {
	mcp.AddTool(srv, &mcp.Tool{
		Name:        "get_project_root",
		Description: "Get the current project root directory. Returns the absolute path of the active project root.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args getProjectRootArgs) (*mcp.CallToolResult, any, error) {
		return s.handleGetProjectRoot(ctx), nil, nil
	})
}

func (s *Server) handleGetProjectRoot(ctx context.Context) *mcp.CallToolResult {
	span, _ := clog.StartSpanFromContext(ctx, "get_project_root")
	defer span.Finish()
	span.LogKV("event", "get_project_root", "projectRoot", s.opts.ProjectRoot)

	return jsonResult(map[string]string{
		"projectRoot": s.opts.ProjectRoot,
	})
}

// ─── open_project ──────────────────────────────────────────────────────────────

func registerOpenProject(srv *mcp.Server, s *Server) {
	mcp.AddTool(srv, &mcp.Tool{
		Name:        "open_project",
		Description: "Switch to a different project root directory. All subsequent tool calls operate on the new project.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args openProjectArgs) (*mcp.CallToolResult, any, error) {
		return s.handleOpenProject(ctx, args), nil, nil
	})
}

func (s *Server) handleOpenProject(ctx context.Context, args openProjectArgs) *mcp.CallToolResult {
	span, _ := clog.StartSpanFromContext(ctx, "open_project")
	defer span.Finish()
	span.LogKV("event", "open_project", "path", args.Path)

	if args.Path == "" {
		return errorResult(fmt.Errorf("path is required"))
	}

	// Update the project root in options
	s.opts.ProjectRoot = args.Path

	span.LogKV("event", "open_project_result", "projectRoot", args.Path)
	return jsonResult(map[string]string{
		"projectRoot": args.Path,
	})
}

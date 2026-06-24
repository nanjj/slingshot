package mcp

import (
	"context"
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/nanjj/clog"
	"github.com/nanjj/slingshot/internal/code/base"
)

// ─── Argument structs ──────────────────────────────────────────────────────────

type searchMemosArgs struct {
	Project    string `json:"project"`
	Query      string `json:"query,omitempty"`
	TypeFilter string `json:"typeFilter,omitempty"`
	Limit      int    `json:"limit,omitempty"`
}

type saveMemoArgs struct {
	Project string `json:"project"`
	Title   string `json:"title"`
	Content string `json:"content"`
	Type    string `json:"type,omitempty"` // decision, architecture, pattern, bugfix, learning, discovery, config
}

type manageADRArgs struct {
	Project string `json:"project"`
	Action  string `json:"action"` // get, update, list
	Mode    string `json:"mode,omitempty"` // for "get" action: sections
	ID      int64  `json:"id,omitempty"`
	Title   string `json:"title,omitempty"`
	Content string `json:"content,omitempty"`
	Status  string `json:"status,omitempty"` // proposed, accepted, deprecated, superseded
}

// ─── search_memos ──────────────────────────────────────────────────────────────

func registerSearchMemos(srv *mcp.Server, s *Server) {
	mcp.AddTool(srv, &mcp.Tool{
		Name:        "search_memos",
		Description: "Search persistent memories by keyword, with optional type filter.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args searchMemosArgs) (*mcp.CallToolResult, any, error) {
		return s.handleSearchMemos(ctx, args), nil, nil
	})
}

func (s *Server) handleSearchMemos(ctx context.Context, args searchMemosArgs) *mcp.CallToolResult {
	span, _ := clog.StartSpanFromContext(ctx, "search_memos")
	defer span.Finish()
	span.LogKV("event", "search_memos", "project", args.Project, "query", args.Query)

	if args.Project == "" {
		return errorResult(fmt.Errorf("project is required"))
	}

	limit := args.Limit
	if limit <= 0 {
		limit = 20
	}

	memos, err := s.store.SearchMemos(args.Project, args.Query, args.TypeFilter, limit)
	if err != nil {
		span.LogKV("event", "error", "error", err.Error())
		return errorResult(fmt.Errorf("search memos: %w", err))
	}
	if memos == nil {
		memos = []base.Memo{}
	}

	span.LogKV("event", "search_memos_result", "total", len(memos))
	return jsonResult(map[string]any{
		"results": memos,
		"total":   len(memos),
	})
}

// ─── save_memo ──────────────────────────────────────────────────────────────────

func registerSaveMemo(srv *mcp.Server, s *Server) {
	mcp.AddTool(srv, &mcp.Tool{
		Name:        "save_memo",
		Description: "Save a persistent memory entry. Use for recording decisions, patterns, bug fixes, and learnings.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args saveMemoArgs) (*mcp.CallToolResult, any, error) {
		return s.handleSaveMemo(ctx, args), nil, nil
	})
}

func (s *Server) handleSaveMemo(ctx context.Context, args saveMemoArgs) *mcp.CallToolResult {
	span, _ := clog.StartSpanFromContext(ctx, "save_memo")
	defer span.Finish()
	span.LogKV("event", "save_memo", "project", args.Project, "title", args.Title, "type", args.Type)

	if args.Project == "" {
		return errorResult(fmt.Errorf("project is required"))
	}
	if args.Title == "" {
		return errorResult(fmt.Errorf("title is required"))
	}

	memoType := args.Type
	if memoType == "" {
		memoType = "learning"
	}

	id, err := s.store.SaveMemo(args.Project, &base.Memo{
		Type:    memoType,
		Title:   args.Title,
		Content: args.Content,
	})
	if err != nil {
		span.LogKV("event", "error", "error", err.Error())
		return errorResult(fmt.Errorf("save memo: %w", err))
	}

	span.LogKV("event", "save_memo_result", "id", id)
	return jsonResult(map[string]any{
		"id":      id,
		"success": true,
	})
}

// ─── manage_adr ────────────────────────────────────────────────────────────────

func registerManageADR(srv *mcp.Server, s *Server) {
	mcp.AddTool(srv, &mcp.Tool{
		Name:        "manage_adr",
		Description: "Create or update Architecture Decision Records. Actions: get (retrieve), update (modify), list (all ADRs for project).",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args manageADRArgs) (*mcp.CallToolResult, any, error) {
		return s.handleManageADR(ctx, args), nil, nil
	})
}

func (s *Server) handleManageADR(ctx context.Context, args manageADRArgs) *mcp.CallToolResult {
	span, _ := clog.StartSpanFromContext(ctx, "manage_adr")
	defer span.Finish()
	span.LogKV("event", "manage_adr", "project", args.Project, "action", args.Action)

	if args.Project == "" {
		return errorResult(fmt.Errorf("project is required"))
	}

	switch args.Action {
	case "list":
		adrs, err := s.store.ListADRs(args.Project)
		if err != nil {
			span.LogKV("event", "error", "error", err.Error())
			return errorResult(fmt.Errorf("list ADRs: %w", err))
		}
		if adrs == nil {
			adrs = []base.ADR{}
		}
		span.LogKV("event", "manage_adr_result", "action", "list", "count", len(adrs))
		return jsonResult(adrs)

	case "get":
		if args.ID == 0 {
			return errorResult(fmt.Errorf("id is required for get action"))
		}
		return errorResult(fmt.Errorf("get ADR by ID not implemented yet; use mode=sections approach"))

	case "update", "create":
		if args.Title == "" {
			return errorResult(fmt.Errorf("title is required"))
		}
		status := args.Status
		if status == "" {
			status = "proposed"
		}
		id, err := s.store.SaveADR(args.Project, &base.ADR{
			Title:   args.Title,
			Content: args.Content,
			Status:  status,
		})
		if err != nil {
			span.LogKV("event", "error", "error", err.Error())
			return errorResult(fmt.Errorf("save ADR: %w", err))
		}
		span.LogKV("event", "manage_adr_result", "action", args.Action, "id", id)
		return jsonResult(map[string]any{
			"id":      id,
			"success": true,
		})

	default:
		return errorResult(fmt.Errorf("unknown action %q; use list, get, update, or create", args.Action))
	}
}

// Package mcp provides MCP tool handlers for code intelligence.
package mcp

import (
	"context"
	"fmt"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/nanjj/clog"
)

// ─── Argument structs ──────────────────────────────────────────────────────────

type codeFindReferencesArgs struct {
	QualifiedName string `json:"qualifiedName"`
	Project       string `json:"project"`
	Direction     string `json:"direction,omitempty"` // "inbound" (default), "outbound", "both"
	Depth         int    `json:"depth,omitempty"`     // 0 = direct references only (default)
}

type codeAnalysisArgs struct {
	File string `json:"file"` // File path (absolute or relative to project root)
}

// ─── code_find_references ─────────────────────────────────────────────────────

func registerCodeFindReferences(srv *mcp.Server, s *Server) {
	mcp.AddTool(srv, &mcp.Tool{
		Name:        "code_find_references",
		Description: "Find all references to a symbol in the code graph. Uses the indexed edges table to find callers/consumers of the given symbol. Supports inbound (who calls this), outbound (who this calls), or both. Depth=0 returns direct references only.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args codeFindReferencesArgs) (*mcp.CallToolResult, any, error) {
		return s.handleCodeFindReferences(ctx, args), nil, nil
	})
}

func (s *Server) handleCodeFindReferences(ctx context.Context, args codeFindReferencesArgs) *mcp.CallToolResult {
	span, ctx := clog.StartSpanFromContext(ctx, "code_find_references")
	defer span.Finish()
	clog.Info(ctx, "code_find_references", "qualifiedName", args.QualifiedName, "project", args.Project, "direction", args.Direction)

	if args.QualifiedName == "" {
		return errorResult(fmt.Errorf("qualifiedName is required"))
	}
	if args.Project == "" {
		return errorResult(fmt.Errorf("project is required"))
	}

	direction := args.Direction
	if direction == "" {
		direction = "inbound"
	}

	depth := args.Depth
	if depth <= 0 {
		depth = 1 // depth=0 → direct only; store uses >=1 for the first level
	}

	// Get references from the store
	edges, err := s.store.GetReferences(args.QualifiedName, args.Project, direction, depth)
	if err != nil {
		clog.Error(ctx, "error", "error", err.Error())
		return errorResult(fmt.Errorf("get references: %w", err))
	}

	// Enrich each edge with source node location info
	type refItem struct {
		SourceQN    string `json:"sourceQN"`
		TargetQN    string `json:"targetQN"`
		EdgeType    string `json:"edgeType"`
		Depth       int    `json:"depth,omitempty"`
		File        string `json:"file,omitempty"`
		Line        uint32 `json:"line,omitempty"`
		Col         uint32 `json:"col,omitempty"`
		SourceKind  string `json:"sourceKind,omitempty"`
	}

	var references []refItem
	seen := make(map[string]bool) // deduplicate by sourceQN

	for _, e := range edges {
		// Only include edges that reference our symbol
		if direction == "inbound" && e.TargetQN != args.QualifiedName {
			continue
		}
		if direction == "outbound" && e.SourceQN != args.QualifiedName {
			continue
		}

		key := e.SourceQN + "->" + e.TargetQN + ":" + e.EdgeType
		if seen[key] {
			continue
		}
		seen[key] = true

		item := refItem{
			SourceQN: e.SourceQN,
			TargetQN: e.TargetQN,
			EdgeType: e.EdgeType,
		}

		// Look up source node for file/line info
		srcNode, err := s.store.GetNodeByQN(e.SourceQN, args.Project)
		if err == nil {
			item.File = srcNode.FilePath
			item.Line = srcNode.Line
			item.Col = srcNode.Col
			item.SourceKind = srcNode.Kind
		}

		references = append(references, item)
	}

	if references == nil {
		references = []refItem{} // ensure JSON array
	}

	clog.Info(ctx, "code_find_references_result", "total", len(references))
	return jsonResult(map[string]any{
		"symbol":     args.QualifiedName,
		"references": references,
		"total":      len(references),
	})
}

// ─── code_analysis ────────────────────────────────────────────────────────────

func registerCodeAnalysis(srv *mcp.Server, s *Server) {
	mcp.AddTool(srv, &mcp.Tool{
		Name:        "code_analysis",
		Description: "Analyze code complexity and quality metrics for a file. Returns per-function breakdown (cyclomatic, cognitive, loop depth, param count, recursion, linear scans in loop) plus summary statistics. Uses tree-sitter for AST parsing.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args codeAnalysisArgs) (*mcp.CallToolResult, any, error) {
		return s.handleCodeAnalysis(ctx, args), nil, nil
	})
}

func (s *Server) handleCodeAnalysis(ctx context.Context, args codeAnalysisArgs) *mcp.CallToolResult {
	span, ctx := clog.StartSpanFromContext(ctx, "code_analysis")
	defer span.Finish()
	clog.Info(ctx, "code_analysis", "file", args.File)

	if args.File == "" {
		return errorResult(fmt.Errorf("file is required"))
	}

	filePath := args.File
	if s.opts != nil && s.opts.ProjectRoot != "" && !strings.HasPrefix(filePath, "/") {
		filePath = s.opts.ProjectRoot + "/" + filePath
	}

	result, err := s.analyzer.AnalyzeFile(filePath)
	if err != nil {
		clog.Error(ctx, "error", "error", err.Error())
		return errorResult(fmt.Errorf("analyze file: %w", err))
	}

	clog.Info(ctx, "code_analysis_result", "functions", countFunctions(result))
	return jsonResult(result)
}

// countFunctions returns the number of FunctionAnalysis results.
func countFunctions(v any) int {
	m, ok := v.(map[string]any)
	if !ok {
		return 0
	}
	fns, ok := m["functions"].([]any)
	if !ok {
		return 0
	}
	return len(fns)
}

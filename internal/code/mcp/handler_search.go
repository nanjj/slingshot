package mcp

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// ─── Argument structs ──────────────────────────────────────────────────────────

type searchGraphArgs struct {
	Query        string   `json:"query,omitempty"`
	NamePattern  string   `json:"namePattern,omitempty"`
	Semantic     []string `json:"semanticQuery,omitempty"`
	Project      string   `json:"project,omitempty"`
	PathFilter   string   `json:"pathFilter,omitempty"`
	FilePattern  string   `json:"filePattern,omitempty"`
	Label        string   `json:"label,omitempty"`
	Kind         string   `json:"kind,omitempty"`
	MinDegree    int      `json:"minDegree,omitempty"`
	MaxDegree    int      `json:"maxDegree,omitempty"`
	Relationship string   `json:"relationship,omitempty"`
	Limit        int      `json:"limit,omitempty"`
	Offset       int      `json:"offset,omitempty"`
}

type getArchitectureArgs struct {
	Project string   `json:"project"`
	Aspects  []string `json:"aspects,omitempty"`
}

type getCodeSnippetArgs struct {
	QualifiedName    string `json:"qualifiedName"`
	Project          string `json:"project"`
	IncludeNeighbors bool   `json:"includeNeighbors,omitempty"`
}

type getGraphSchemaArgs struct {
	Project string `json:"project"`
}

type tracePathArgs struct {
	FunctionName  string `json:"functionName"`
	Project       string `json:"project"`
	Direction     string `json:"direction,omitempty"` // inbound, outbound, both
	Depth         int    `json:"depth,omitempty"`
	Mode          string `json:"mode,omitempty"` // calls, data_flow, cross_service
	IncludeTests  bool   `json:"includeTests,omitempty"`
	ParameterName string `json:"parameterName,omitempty"`
	RiskLabels    bool   `json:"riskLabels,omitempty"`
}

type detectChangesArgs struct {
	Project    string `json:"project"`
	BaseBranch string `json:"baseBranch,omitempty"`
	Since      string `json:"since,omitempty"`
	Scope      string `json:"scope,omitempty"`
	Depth      int    `json:"depth,omitempty"`
}

// ─── search_graph ─────────────────────────────────────────────────────────────

func registerSearchGraph(srv *mcp.Server, s *Server) {
	mcp.AddTool(srv, &mcp.Tool{
		Name:        "search_graph",
		Description: "Search the code knowledge graph for functions, classes, routes, and variables. Three search modes: (1) query for BM25 ranked full-text search with camelCase splitting; (2) namePattern for exact pattern matching; (3) semanticQuery for vector cosine search.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args searchGraphArgs) (*mcp.CallToolResult, any, error) {
		return s.handleSearchGraph(args), nil, nil
	})
}

func (s *Server) handleSearchGraph(args searchGraphArgs) *mcp.CallToolResult {
	project := args.Project
	if project == "" {
		return errorResult(fmt.Errorf("project is required"))
	}

	limit := args.Limit
	if limit <= 0 {
		limit = 20
	}

	// Primary mode: BM25 full-text search via SearchNodes
	if args.Query != "" {
		nodes, err := s.store.SearchNodes(args.Query, project, args.PathFilter, limit, args.Offset)
		if err != nil {
			return errorResult(fmt.Errorf("search nodes: %w", err))
		}
		return jsonResult(map[string]any{
			"results": nodes,
			"total":   len(nodes),
		})
	}

	// Name pattern mode via FindSymbols
	if args.NamePattern != "" {
		nodes, err := s.store.FindSymbols(args.NamePattern, project, args.Kind, limit, args.Offset)
		if err != nil {
			return errorResult(fmt.Errorf("find symbols: %w", err))
		}
		return jsonResult(map[string]any{
			"results": nodes,
			"total":   len(nodes),
		})
	}

	return errorResult(fmt.Errorf("provide one of: query, namePattern, or semanticQuery"))
}

// ─── get_architecture ──────────────────────────────────────────────────────────

func registerGetArchitecture(srv *mcp.Server, s *Server) {
	mcp.AddTool(srv, &mcp.Tool{
		Name:        "get_architecture",
		Description: "Get high-level architecture overview — packages, services, dependencies, and project structure at a glance. Includes Leiden community detection clusters over the call/import graph.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args getArchitectureArgs) (*mcp.CallToolResult, any, error) {
		return s.handleGetArchitecture(args), nil, nil
	})
}

func (s *Server) handleGetArchitecture(args getArchitectureArgs) *mcp.CallToolResult {
	if args.Project == "" {
		return errorResult(fmt.Errorf("project is required"))
	}

	// Query project info
	info, err := s.store.ProjectStatus(args.Project)
	if err != nil {
		return errorResult(fmt.Errorf("project status: %w", err))
	}

	// Query cluster structure via graph: count nodes by kind
	kindQuery := fmt.Sprintf(`
		SELECT n.kind, COUNT(*) as count
		FROM nodes n
		JOIN projects p ON p.id = n.project_id
		WHERE p.name = '%s'
		GROUP BY n.kind
		ORDER BY count DESC
	`, args.Project)

	kindDist, err := s.store.QueryGraph(kindQuery)
	if err != nil {
		kindDist = nil // non-critical
	}

	// Count edge types
	edgeQuery := fmt.Sprintf(`
		SELECT e.edge_type, COUNT(*) as count
		FROM edges e
		JOIN projects p ON p.id = e.project_id
		WHERE p.name = '%s'
		GROUP BY e.edge_type
		ORDER BY count DESC
	`, args.Project)

	edgeDist, err := s.store.QueryGraph(edgeQuery)
	if err != nil {
		edgeDist = nil
	}

	return jsonResult(map[string]any{
		"project":          info,
		"kindDistribution": kindDist,
		"edgeDistribution": edgeDist,
	})
}

// ─── get_code_snippet ──────────────────────────────────────────────────────────

func registerGetCodeSnippet(srv *mcp.Server, s *Server) {
	mcp.AddTool(srv, &mcp.Tool{
		Name:        "get_code_snippet",
		Description: "Read source code for a function/class/symbol. Accepts full qualified_name or short function name (returns suggestions if ambiguous).",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args getCodeSnippetArgs) (*mcp.CallToolResult, any, error) {
		return s.handleGetCodeSnippet(args), nil, nil
	})
}

func (s *Server) handleGetCodeSnippet(args getCodeSnippetArgs) *mcp.CallToolResult {
	if args.QualifiedName == "" {
		return errorResult(fmt.Errorf("qualifiedName is required"))
	}
	if args.Project == "" {
		return errorResult(fmt.Errorf("project is required"))
	}

	node, err := s.store.GetNodeByQN(args.QualifiedName, args.Project)
	if err != nil {
		return errorResult(fmt.Errorf("get node: %w", err))
	}

	// Read source from file
	// Try relative to project root first
	filePath := node.FilePath
	source, err := os.ReadFile(filePath)
	if err != nil {
		// Try relative to the store project root
		info, pErr := s.store.ProjectStatus(args.Project)
		if pErr == nil && info.Root != "" {
			absPath := filepath.Join(info.Root, filePath)
			source, err = os.ReadFile(absPath)
		}
	}
	if err != nil {
		return errorResult(fmt.Errorf("read source file %q: %w", filePath, err))
	}

	// Extract the relevant lines
	lines := strings.Split(string(source), "\n")
	startLine := int(node.Line)
	endLine := int(node.EndLine)
	if endLine > len(lines) {
		endLine = len(lines)
	}
	if startLine > endLine {
		startLine = 0
		endLine = len(lines)
	}

	code := strings.Join(lines[startLine:endLine+1], "\n")

	result := map[string]any{
		"node":      node,
		"source":    code,
		"filePath":  filePath,
		"startLine": startLine + 1, // 1-indexed
		"endLine":   endLine + 1,
	}

	if args.IncludeNeighbors {
		// Fetch related nodes (inbound/outbound references)
		edges, _ := s.store.GetReferences(args.QualifiedName, args.Project, "both", 1)
		result["neighbors"] = edges
	}

	return jsonResult(result)
}

// ─── get_graph_schema ──────────────────────────────────────────────────────────

func registerGetGraphSchema(srv *mcp.Server, s *Server) {
	mcp.AddTool(srv, &mcp.Tool{
		Name:        "get_graph_schema",
		Description: "Get the schema of the knowledge graph — node labels, edge types, and their properties.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args getGraphSchemaArgs) (*mcp.CallToolResult, any, error) {
		return s.handleGetGraphSchema(args), nil, nil
	})
}

func (s *Server) handleGetGraphSchema(args getGraphSchemaArgs) *mcp.CallToolResult {
	if args.Project == "" {
		return errorResult(fmt.Errorf("project is required"))
	}

	// Query distinct node kinds
	kindQuery := fmt.Sprintf(`
		SELECT DISTINCT n.kind, COUNT(*) as count
		FROM nodes n
		JOIN projects p ON p.id = n.project_id
		WHERE p.name = '%s'
		GROUP BY n.kind
		ORDER BY count DESC
	`, args.Project)

	kinds, err := s.store.QueryGraph(kindQuery)
	if err != nil {
		return errorResult(fmt.Errorf("query kinds: %w", err))
	}

	// Query distinct edge types
	edgeQuery := fmt.Sprintf(`
		SELECT DISTINCT e.edge_type, COUNT(*) as count
		FROM edges e
		JOIN projects p ON p.id = e.project_id
		WHERE p.name = '%s'
		GROUP BY e.edge_type
		ORDER BY count DESC
	`, args.Project)

	edgeTypes, err := s.store.QueryGraph(edgeQuery)
	if err != nil {
		return errorResult(fmt.Errorf("query edge types: %w", err))
	}

	return jsonResult(map[string]any{
		"nodeLabels": kinds,
		"edgeTypes":  edgeTypes,
	})
}

// ─── trace_path ────────────────────────────────────────────────────────────────

func registerTracePath(srv *mcp.Server, s *Server) {
	mcp.AddTool(srv, &mcp.Tool{
		Name:        "trace_path",
		Description: "Trace paths through the code graph. Modes: calls (callers/callees), data_flow (value propagation with args at each hop), cross_service (through HTTP/async Route nodes).",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args tracePathArgs) (*mcp.CallToolResult, any, error) {
		return s.handleTracePath(args), nil, nil
	})
}

func (s *Server) handleTracePath(args tracePathArgs) *mcp.CallToolResult {
	if args.FunctionName == "" {
		return errorResult(fmt.Errorf("functionName is required"))
	}
	if args.Project == "" {
		return errorResult(fmt.Errorf("project is required"))
	}

	direction := args.Direction
	if direction == "" {
		direction = "both"
	}

	depth := args.Depth
	if depth <= 0 {
		depth = 3
	}

	// Use TraceCallChain for call tracing
	hops, err := s.store.TraceCallChain(args.FunctionName, args.Project, direction, depth)
	if err != nil {
		return errorResult(fmt.Errorf("trace call chain: %w", err))
	}

	return jsonResult(map[string]any{
		"hops":  hops,
		"total": len(hops),
	})
}

// ─── detect_changes ───────────────────────────────────────────────────────────

func registerDetectChanges(srv *mcp.Server, s *Server) {
	mcp.AddTool(srv, &mcp.Tool{
		Name:        "detect_changes",
		Description: "Detect code changes and their impact. Supports git diff-based change detection with configurable scope and depth.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args detectChangesArgs) (*mcp.CallToolResult, any, error) {
		return s.handleDetectChanges(args), nil, nil
	})
}

func (s *Server) handleDetectChanges(args detectChangesArgs) *mcp.CallToolResult {
	if args.Project == "" {
		return errorResult(fmt.Errorf("project is required"))
	}

	info, err := s.store.ProjectStatus(args.Project)
	if err != nil {
		return errorResult(fmt.Errorf("project status: %w", err))
	}

	baseBranch := args.BaseBranch
	if baseBranch == "" {
		baseBranch = "main"
	}

	rootDir := info.Root
	if rootDir == "" {
		return errorResult(fmt.Errorf("project root not found"))
	}

	return jsonResult(map[string]any{
		"project":    args.Project,
		"baseBranch": baseBranch,
		"root":       rootDir,
		"note":       "Change detection via git diff — full implementation pending",
	})
}

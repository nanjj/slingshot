package mcp

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/nanjj/clog"
	"github.com/nanjj/slingshot/internal/code/base"
)

// ─── Argument structs ──────────────────────────────────────────────────────────

type searchGraphArgs struct {
	Query            string   `json:"query,omitempty"`
	Regex            string   `json:"regex,omitempty"` // alias for query
	NamePattern      string   `json:"namePattern,omitempty"`
	Semantic         []string `json:"semanticQuery,omitempty"`
	Project          string   `json:"project,omitempty"` // optional once open_project is bound
	PathFilter       string   `json:"pathFilter,omitempty"`
	FilePattern      string   `json:"filePattern,omitempty"`
	FilePatternSnake string   `json:"file_pattern,omitempty"` // alias for filePattern
	Label            string   `json:"label,omitempty"`
	Kind             string   `json:"kind,omitempty"`
	MinDegree        int      `json:"minDegree,omitempty"`
	MaxDegree        int      `json:"maxDegree,omitempty"`
	Relationship     string   `json:"relationship,omitempty"`
	Limit            int      `json:"limit,omitempty"`
	Offset           int      `json:"offset,omitempty"`
}

type getArchitectureArgs struct {
	Project string   `json:"project,omitempty"` // optional once open_project is bound
	Aspects []string `json:"aspects,omitempty"`
}

type getCodeSnippetArgs struct {
	QualifiedName    string `json:"qualifiedName"`
	Project          string `json:"project,omitempty"` // optional once open_project is bound
	IncludeNeighbors bool   `json:"includeNeighbors,omitempty"`
}

type getGraphSchemaArgs struct {
	Project string `json:"project,omitempty"` // optional once open_project is bound
}

type tracePathArgs struct {
	FunctionName  string `json:"functionName"`
	Function      string `json:"function,omitempty"`  // alias for functionName
	Project       string `json:"project,omitempty"`   // optional once open_project is bound
	Direction     string `json:"direction,omitempty"` // inbound, outbound, both
	Depth         int    `json:"depth,omitempty"`
	Mode          string `json:"mode,omitempty"` // calls, data_flow, cross_service
	IncludeTests  bool   `json:"includeTests,omitempty"`
	ParameterName string `json:"parameterName,omitempty"`
	RiskLabels    bool   `json:"riskLabels,omitempty"`
}

type detectChangesArgs struct {
	Project    string `json:"project,omitempty"` // optional once open_project is bound
	BaseBranch string `json:"baseBranch,omitempty"`
	Since      string `json:"since,omitempty"`
	Scope      string `json:"scope,omitempty"`
	Depth      int    `json:"depth,omitempty"`
}

// ─── search_graph ─────────────────────────────────────────────────────────────

func registerSearchGraph(srv *mcp.Server, s *Server) {
	mcp.AddTool(srv, &mcp.Tool{
		Name:        "search_graph",
		Description: "Search the code knowledge graph for functions, classes, routes, and variables. Three search modes: (1) query for BM25 ranked full-text search with camelCase splitting — Function/Method/Route nodes are boosted; (2) namePattern for exact pattern matching; (3) semanticQuery for vector cosine search. Supports pagination: use limit/offset, response includes total (full count) and has_more flag. project is optional once open_project has been called. Aliases: regex (query), file_pattern (filePattern).",
		InputSchema: lenientSchema[searchGraphArgs](),
	}, func(ctx context.Context, req *mcp.CallToolRequest, args searchGraphArgs) (*mcp.CallToolResult, any, error) {
		return s.handleSearchGraph(ctx, args), nil, nil
	})
}

func (s *Server) handleSearchGraph(ctx context.Context, args searchGraphArgs) *mcp.CallToolResult {
	span, ctx := clog.StartSpanFromContext(ctx, "search_graph")
	defer span.Finish()

	query := firstNonEmpty(args.Query, args.Regex)
	filePattern := firstNonEmpty(args.FilePattern, args.FilePatternSnake)
	clog.Info(ctx, "search_graph_start", "project", args.Project, "query", query, "namePattern", args.NamePattern)

	info, err := s.resolveProject(args.Project)
	if err != nil {
		clog.Error(ctx, "error", "error", err.Error())
		return errorResult(err)
	}
	project := info.Name

	limit := args.Limit
	if limit <= 0 {
		limit = 20
	}

	// Primary mode: BM25 full-text search via SearchNodes
	if query != "" {
		nodes, total, err := s.store.SearchNodes(query, project, filePattern, limit, args.Offset)
		if err != nil {
			clog.Error(ctx, "error", "error", err.Error())
			return errorResult(fmt.Errorf("search nodes: %w", err))
		}
		hasMore := args.Offset+len(nodes) < total
		clog.Info(ctx, "search_graph_result", "mode", "bm25", "total", total, "returned", len(nodes))
		return jsonResult(map[string]any{
			"results":  nodes,
			"total":    total,
			"has_more": hasMore,
			"mode":     "bm25",
		})
	}

	// Name pattern mode via FindSymbols
	if args.NamePattern != "" {
		nodes, total, err := s.store.FindSymbols(args.NamePattern, project, args.Kind, limit, args.Offset)
		if err != nil {
			clog.Error(ctx, "error", "error", err.Error())
			return errorResult(fmt.Errorf("find symbols: %w", err))
		}
		hasMore := args.Offset+len(nodes) < total
		clog.Info(ctx, "search_graph_result", "mode", "namePattern", "total", total, "returned", len(nodes))
		return jsonResult(map[string]any{
			"results":  nodes,
			"total":    total,
			"has_more": hasMore,
			"mode":     "namePattern",
		})
	}

	// Semantic query mode: BM25 fallback (true vector search requires embeddings)
	if len(args.Semantic) > 0 {
		semQuery := strings.Join(args.Semantic, " ")
		nodes, total, err := s.store.SearchNodes(semQuery, project, filePattern, limit, args.Offset)
		if err != nil {
			clog.Error(ctx, "error", "error", err.Error())
			return errorResult(fmt.Errorf("semantic search: %w", err))
		}
		hasMore := args.Offset+len(nodes) < total
		clog.Info(ctx, "search_graph_result", "mode", "semantic", "total", total, "returned", len(nodes))
		return jsonResult(map[string]any{
			"results":  nodes,
			"total":    total,
			"has_more": hasMore,
			"mode":     "semantic",
			"note":     "BM25 fallback — true vector semantic search requires embedding infrastructure",
		})
	}

	return errorResult(fmt.Errorf("provide one of: query, namePattern, or semanticQuery"))
}

// ─── get_architecture ──────────────────────────────────────────────────────────

func registerGetArchitecture(srv *mcp.Server, s *Server) {
	mcp.AddTool(srv, &mcp.Tool{
		Name:        "get_architecture",
		Description: "Get high-level architecture overview — packages, services, dependencies, and project structure at a glance. Includes Leiden community detection clusters over the call/import graph. Use `aspects` to select subsets: kindDistribution, edgeDistribution, hotspots, fileTree, packageDeps. Default (empty aspects) returns all. project is optional once open_project has been called.",
		InputSchema: lenientSchema[getArchitectureArgs](),
	}, func(ctx context.Context, req *mcp.CallToolRequest, args getArchitectureArgs) (*mcp.CallToolResult, any, error) {
		return s.handleGetArchitecture(ctx, args), nil, nil
	})
}

func (s *Server) handleGetArchitecture(ctx context.Context, args getArchitectureArgs) *mcp.CallToolResult {
	span, ctx := clog.StartSpanFromContext(ctx, "get_architecture")
	defer span.Finish()
	clog.Info(ctx, "get_architecture", "project", args.Project)

	info, err := s.resolveProject(args.Project)
	if err != nil {
		clog.Error(ctx, "error", "error", err.Error())
		return errorResult(err)
	}
	project := info.Name

	result := map[string]any{
		"project": info,
	}

	// Determine which aspects to include.
	include := func(name string) bool {
		if len(args.Aspects) == 0 {
			return true
		}
		for _, a := range args.Aspects {
			if a == name {
				return true
			}
		}
		return false
	}

	if include("kindDistribution") || len(args.Aspects) == 0 {
		kindQuery := fmt.Sprintf(`
			SELECT n.kind, COUNT(*) as count
			FROM nodes n
			JOIN projects p ON p.id = n.project_id
			WHERE p.name = '%s'
			GROUP BY n.kind
			ORDER BY count DESC
		`, project)
		kindDist, err := s.store.QueryGraph(kindQuery)
		if err == nil {
			result["kindDistribution"] = kindDist
		}
	}

	if include("edgeDistribution") || len(args.Aspects) == 0 {
		edgeQuery := fmt.Sprintf(`
			SELECT e.edge_type, COUNT(*) as count
			FROM edges e
			JOIN projects p ON p.id = e.project_id
			WHERE p.name = '%s'
			GROUP BY e.edge_type
			ORDER BY count DESC
		`, project)
		edgeDist, err := s.store.QueryGraph(edgeQuery)
		if err == nil {
			result["edgeDistribution"] = edgeDist
		}
	}

	if include("hotspots") {
		hotspots, err := s.store.Hotspots(project, 20)
		if err == nil {
			result["hotspots"] = hotspots
		}
	}

	if include("fileTree") {
		ft, err := s.store.FileTree(project)
		if err == nil {
			result["fileTree"] = ft
		}
	}

	if include("packageDeps") {
		deps, err := s.store.PackageDeps(project)
		if err == nil {
			result["packageDeps"] = deps
		}
	}

	clog.Info(ctx, "get_architecture_result", "aspects", len(result))
	return jsonResult(result)
}

// ─── get_code_snippet ──────────────────────────────────────────────────────────

func registerGetCodeSnippet(srv *mcp.Server, s *Server) {
	mcp.AddTool(srv, &mcp.Tool{
		Name:        "get_code_snippet",
		Description: "Read source code for a function/class/symbol. Accepts full qualified_name or short function name (returns suggestions if ambiguous). project is optional once open_project has been called.",
		InputSchema: lenientSchema[getCodeSnippetArgs](),
	}, func(ctx context.Context, req *mcp.CallToolRequest, args getCodeSnippetArgs) (*mcp.CallToolResult, any, error) {
		return s.handleGetCodeSnippet(ctx, args), nil, nil
	})
}

func (s *Server) handleGetCodeSnippet(ctx context.Context, args getCodeSnippetArgs) *mcp.CallToolResult {
	span, ctx := clog.StartSpanFromContext(ctx, "get_code_snippet")
	defer span.Finish()
	clog.Info(ctx, "get_code_snippet", "qualifiedName", args.QualifiedName, "project", args.Project)

	if args.QualifiedName == "" {
		return errorResult(fmt.Errorf("qualifiedName is required"))
	}

	info, err := s.resolveProject(args.Project)
	if err != nil {
		clog.Error(ctx, "error", "error", err.Error())
		return errorResult(err)
	}
	project := info.Name

	node, err := s.store.GetNodeByQN(args.QualifiedName, project)
	if err != nil {
		clog.Error(ctx, "error", "error", err.Error())
		return errorResult(fmt.Errorf("get node: %w", err))
	}

	// Read source from file
	// Try relative to project root first
	filePath := node.FilePath
	source, err := os.ReadFile(filePath)
	if err != nil {
		// Try relative to the store project root
		if info.Root != "" {
			absPath := filepath.Join(info.Root, filePath)
			source, err = os.ReadFile(absPath)
		}
	}
	if err != nil {
		clog.Error(ctx, "error", "error", err.Error())
		if errors.Is(err, fs.ErrNotExist) {
			return errorResult(fmt.Errorf("read source file %q: %w — index may be stale (file moved/deleted); run index_repository to refresh", filePath, err))
		}
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
		edges, _ := s.store.GetReferences(args.QualifiedName, project, "both", 1)
		result["neighbors"] = edges
	}

	clog.Info(ctx, "get_code_snippet_result", "file", filePath, "lines", endLine-startLine+1)
	return jsonResult(result)
}

// ─── get_graph_schema ──────────────────────────────────────────────────────────

func registerGetGraphSchema(srv *mcp.Server, s *Server) {
	mcp.AddTool(srv, &mcp.Tool{
		Name:        "get_graph_schema",
		Description: "Get the schema of the knowledge graph — node labels, edge types, and their properties. project is optional once open_project has been called.",
		InputSchema: lenientSchema[getGraphSchemaArgs](),
	}, func(ctx context.Context, req *mcp.CallToolRequest, args getGraphSchemaArgs) (*mcp.CallToolResult, any, error) {
		return s.handleGetGraphSchema(ctx, args), nil, nil
	})
}

func (s *Server) handleGetGraphSchema(ctx context.Context, args getGraphSchemaArgs) *mcp.CallToolResult {
	span, ctx := clog.StartSpanFromContext(ctx, "get_graph_schema")
	defer span.Finish()
	clog.Info(ctx, "get_graph_schema", "project", args.Project)

	info, err := s.resolveProject(args.Project)
	if err != nil {
		clog.Error(ctx, "error", "error", err.Error())
		return errorResult(err)
	}
	project := info.Name

	// Query distinct node kinds
	kindQuery := fmt.Sprintf(`
		SELECT DISTINCT n.kind, COUNT(*) as count
		FROM nodes n
		JOIN projects p ON p.id = n.project_id
		WHERE p.name = '%s'
		GROUP BY n.kind
		ORDER BY count DESC
	`, project)

	kinds, err := s.store.QueryGraph(kindQuery)
	if err != nil {
		clog.Error(ctx, "error", "error", err.Error())
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
	`, project)

	edgeTypes, err := s.store.QueryGraph(edgeQuery)
	if err != nil {
		clog.Error(ctx, "error", "error", err.Error())
		return errorResult(fmt.Errorf("query edge types: %w", err))
	}

	clog.Info(ctx, "get_graph_schema_result", "nodeLabels", len(kinds), "edgeTypes", len(edgeTypes))
	return jsonResult(map[string]any{
		"nodeLabels": kinds,
		"edgeTypes":  edgeTypes,
	})
}

// ─── trace_path ────────────────────────────────────────────────────────────────

func registerTracePath(srv *mcp.Server, s *Server) {
	mcp.AddTool(srv, &mcp.Tool{
		Name:        "trace_path",
		Description: "Trace paths through the code graph. Modes: calls (callers/callees), data_flow (value propagation with args at each hop), cross_service (through HTTP/async Route nodes). Supports risk_labels (HIGH/MEDIUM/LOW by depth), include_tests (exclude test files by default). project is optional once open_project has been called. Alias: function (functionName).",
		InputSchema: lenientSchema[tracePathArgs](),
	}, func(ctx context.Context, req *mcp.CallToolRequest, args tracePathArgs) (*mcp.CallToolResult, any, error) {
		return s.handleTracePath(ctx, args), nil, nil
	})
}

func (s *Server) handleTracePath(ctx context.Context, args tracePathArgs) *mcp.CallToolResult {
	span, ctx := clog.StartSpanFromContext(ctx, "trace_path")
	defer span.Finish()

	functionName := firstNonEmpty(args.FunctionName, args.Function)
	clog.Info(ctx, "trace_path", "functionName", functionName, "project", args.Project, "direction", args.Direction)

	if functionName == "" {
		return errorResult(fmt.Errorf("functionName is required — pass functionName (or function)"))
	}

	info, err := s.resolveProject(args.Project)
	if err != nil {
		clog.Error(ctx, "error", "error", err.Error())
		return errorResult(err)
	}
	project := info.Name

	// Build a list of QN candidates. The user may provide a short name
	// (e.g. "handleSearchGraph") while the graph stores edges with a
	// receiver prefix (e.g. "s.handleSearchGraph" for method calls).
	candidates := []string{functionName}

	// Add prefix variants for method calls
	if !strings.HasPrefix(functionName, "s.") {
		candidates = append(candidates, "s."+functionName)
	}
	// Try fuzzy match via SearchNodes for the original name
	nodes, _, searchErr := s.store.SearchNodes(functionName, project, "", 5, 0)
	if searchErr == nil && len(nodes) > 0 {
		for _, n := range nodes {
			if n.QualifiedName != functionName {
				candidates = append(candidates, n.QualifiedName)
			}
		}
	}

	// Try each candidate until we get non-empty results
	var hops []base.TraceHop
	var lastErr error
	resolvedQN := functionName

	for _, qn := range candidates {
		req := base.TracePathRequest{
			FunctionName:  qn,
			Project:       project,
			Direction:     args.Direction,
			Depth:         args.Depth,
			Mode:          args.Mode,
			IncludeTests:  args.IncludeTests,
			ParameterName: args.ParameterName,
			RiskLabels:    args.RiskLabels,
		}

		h, err := s.store.TracePath(req)
		if err != nil {
			lastErr = err
			continue
		}
		if len(h) > 0 {
			hops = h
			resolvedQN = qn
			break
		}
	}

	if len(hops) == 0 && lastErr != nil {
		clog.Error(ctx, "error", "error", lastErr.Error())
		return errorResult(fmt.Errorf("trace path: %w", lastErr))
	}

	clog.Info(ctx, "trace_path_result", "resolvedQN", resolvedQN, "hops", len(hops))
	return jsonResult(map[string]any{
		"hops":       hops,
		"total":      len(hops),
		"resolvedQN": resolvedQN,
		"originalQN": functionName,
	})
}

// ─── detect_changes ────────────────────────────────────────────────────────

func registerDetectChanges(srv *mcp.Server, s *Server) {
	mcp.AddTool(srv, &mcp.Tool{
		Name:        "detect_changes",
		Description: "Detect code changes and their impact. Supports git diff-based change detection with configurable scope and depth. Scope 'impact' also returns impacted symbols via graph propagation. project is optional once open_project has been called.",
		InputSchema: lenientSchema[detectChangesArgs](),
	}, func(ctx context.Context, req *mcp.CallToolRequest, args detectChangesArgs) (*mcp.CallToolResult, any, error) {
		return s.handleDetectChanges(ctx, args), nil, nil
	})
}

func (s *Server) handleDetectChanges(ctx context.Context, args detectChangesArgs) *mcp.CallToolResult {
	span, ctx := clog.StartSpanFromContext(ctx, "detect_changes")
	defer span.Finish()
	clog.Info(ctx, "detect_changes", "project", args.Project, "baseBranch", args.BaseBranch, "scope", args.Scope)

	info, err := s.resolveProject(args.Project)
	if err != nil {
		clog.Error(ctx, "error", "error", err.Error())
		return errorResult(err)
	}
	project := info.Name

	baseBranch := args.BaseBranch
	if baseBranch == "" {
		baseBranch = "main"
	}

	rootDir := info.Root
	if rootDir == "" {
		return errorResult(fmt.Errorf("project root not found"))
	}

	scope := args.Scope
	depth := args.Depth
	if depth <= 0 {
		depth = 2
	}

	// Run git diff --name-status to get changed files
	changedFiles, err := gitDiffNameStatus(rootDir, baseBranch)
	if err != nil {
		clog.Info(ctx, "git_diff_error", "error", err.Error())
		return jsonResult(map[string]any{
			"project":    project,
			"baseBranch": baseBranch,
			"root":       rootDir,
			"error":      fmt.Sprintf("git diff failed: %v", err),
			"note":       "Ensure the project is a git repository with the base branch available",
		})
	}

	// Run git diff --stat for summary
	statOutput, _ := gitDiffStat(rootDir, baseBranch)

	result := map[string]any{
		"project":      project,
		"baseBranch":   baseBranch,
		"root":         rootDir,
		"changedFiles": changedFiles,
		"stat":         statOutput,
		"total":        len(changedFiles),
		"scope":        scope,
		"depth":        depth,
	}

	// Impact analysis mode: propagate changes through the call graph
	if scope == "impact" && len(changedFiles) > 0 {
		// Extract just the file paths from changed files
		var filePaths []string
		for _, f := range changedFiles {
			if path, ok := f["file"]; ok {
				filePaths = append(filePaths, path)
			}
		}

		impacted, err := s.store.ImpactAnalysis(project, filePaths, depth)
		if err != nil {
			result["impactError"] = fmt.Sprintf("impact analysis: %v", err)
		} else {
			result["impactedSymbols"] = impacted
			result["impactedCount"] = len(impacted)
		}
	}

	clog.Info(ctx, "detect_changes_result", "changedFiles", len(changedFiles))
	return jsonResult(result)
}

// gitDiffNameStatus returns a list of changed files with their status.
func gitDiffNameStatus(repoDir, baseBranch string) ([]map[string]string, error) {
	cmd := exec.Command("git", "diff", "--name-status", baseBranch+"...HEAD")
	cmd.Dir = repoDir
	out, err := cmd.Output()
	if err != nil {
		return nil, err
	}

	var files []map[string]string
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, "\t", 2)
		if len(parts) != 2 {
			continue
		}
		files = append(files, map[string]string{
			"status": parts[0],
			"file":   parts[1],
		})
	}
	if files == nil {
		files = []map[string]string{}
	}
	return files, nil
}

// gitDiffStat returns a human-readable diff stat summary.
func gitDiffStat(repoDir, baseBranch string) (string, error) {
	cmd := exec.Command("git", "diff", "--stat", baseBranch+"...HEAD")
	cmd.Dir = repoDir
	out, err := cmd.Output()
	return strings.TrimSpace(string(out)), err
}

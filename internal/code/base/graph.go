package base

import (
	"database/sql"
	"fmt"
	"strings"
)

// ─── Graph Traversal ──────────────────────────────────────────────────────────

// GetReferences returns all edges referencing the given qualified name.
// Direction "inbound" = nodes that call/use this node (target_qn = qn).
// Direction "outbound" = nodes that this node calls/uses (source_qn = qn).
// Direction "both" = all edges.
func (s *Store) GetReferences(qn, project, direction string, depth int) ([]Edge, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if depth <= 0 {
		depth = 1
	}
	if depth > 10 {
		depth = 10 // safety limit
	}

	var query string
	var args []any

	switch direction {
	case "inbound":
		match, matchArgs := s.qnMatchClause(project, qn, "e.target_qn")
		args = []any{project}
		args = append(args, matchArgs...)
		args = append(args, project, depth)
		query = `
			WITH RECURSIVE refs(id, project_id, source_qn, target_qn, edge_type, metadata, created_at, lvl) AS (
				SELECT e.id, e.project_id, e.source_qn, e.target_qn, e.edge_type, e.metadata, e.created_at, 1
				FROM edges e
				JOIN projects p ON p.id = e.project_id
				WHERE p.name = ? AND ` + match + `
				UNION ALL
				SELECT e.id, e.project_id, e.source_qn, e.target_qn, e.edge_type, e.metadata, e.created_at, r.lvl + 1
				FROM edges e
				JOIN refs r ON e.target_qn = r.source_qn
				JOIN projects p ON p.id = e.project_id
				WHERE p.name = ? AND r.lvl < ?
			)
			SELECT id, project_id, source_qn, target_qn, edge_type, metadata, created_at
			FROM refs
			ORDER BY lvl, source_qn
		`
	case "outbound":
		args = []any{project, qn, project, depth}
		query = `
			WITH RECURSIVE refs(id, project_id, source_qn, target_qn, edge_type, metadata, created_at, lvl) AS (
				SELECT e.id, e.project_id, e.source_qn, e.target_qn, e.edge_type, e.metadata, e.created_at, 1
				FROM edges e
				JOIN projects p ON p.id = e.project_id
				WHERE p.name = ? AND e.source_qn = ?
				UNION ALL
				SELECT e.id, e.project_id, e.source_qn, e.target_qn, e.edge_type, e.metadata, e.created_at, r.lvl + 1
				FROM edges e
				JOIN refs r ON e.source_qn = r.target_qn
				JOIN projects p ON p.id = e.project_id
				WHERE p.name = ? AND r.lvl < ?
			)
			SELECT id, project_id, source_qn, target_qn, edge_type, metadata, created_at
			FROM refs
			ORDER BY lvl, source_qn
		`
	default: // both
		match, matchArgs := s.qnMatchClause(project, qn, "e.target_qn")
		args = []any{project, qn}
		args = append(args, matchArgs...)
		args = append(args, project, depth)
		query = `
			WITH RECURSIVE refs(id, project_id, source_qn, target_qn, edge_type, metadata, created_at, lvl) AS (
				SELECT e.id, e.project_id, e.source_qn, e.target_qn, e.edge_type, e.metadata, e.created_at, 1
				FROM edges e
				JOIN projects p ON p.id = e.project_id
				WHERE p.name = ? AND (e.source_qn = ? OR ` + match + `)
				UNION ALL
				SELECT e.id, e.project_id, e.source_qn, e.target_qn, e.edge_type, e.metadata, e.created_at, r.lvl + 1
				FROM edges e
				JOIN refs r ON (e.target_qn = r.source_qn OR e.source_qn = r.target_qn)
				JOIN projects p ON p.id = e.project_id
				WHERE p.name = ? AND r.lvl < ?
			)
			SELECT id, project_id, source_qn, target_qn, edge_type, metadata, created_at
			FROM refs
			ORDER BY lvl, source_qn
		`
	}

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("get references: %w", err)
	}
	defer rows.Close()

	return scanEdges(rows)
}

// TraceCallChain traces call chains between nodes using recursive CTE.
// Returns a flat list of hops.
//
// Deprecated: use TracePath for full mode/risk/test support.
func (s *Store) TraceCallChain(qn, project, direction string, depth int) ([]TraceHop, error) {
	return s.TracePath(TracePathRequest{
		FunctionName: qn,
		Project:      project,
		Direction:    direction,
		Depth:        depth,
		Mode:         "calls",
	})
}

// TracePath traces code paths through the graph with support for multiple
// modes (calls, data_flow, cross_service), risk labels, and test filtering.
func (s *Store) TracePath(req TracePathRequest) ([]TraceHop, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if req.Depth <= 0 {
		req.Depth = 3
	}
	if req.Depth > 20 {
		req.Depth = 20 // safety limit
	}

	direction := req.Direction
	if direction == "" {
		direction = "both"
	}

	mode := req.Mode
	if mode == "" {
		mode = "calls"
	}

	// Determine which edge types to follow based on mode
	edgeTypes := "'CALLS'"
	if mode == "cross_service" {
		edgeTypes = "'CALLS', 'HTTP_CALLS'"
	}

	// Always select metadata so the scan is always 7 fixed columns
	metadataCol := "COALESCE(e.metadata, '')"

	var query string
	var args []any

	switch direction {
	case "inbound":
		match, matchArgs := s.qnMatchClause(req.Project, req.FunctionName, "e.target_qn")
		args = []any{req.Project}
		args = append(args, matchArgs...)
		args = append(args, req.Project, req.Depth)
		query = `
			WITH RECURSIVE chain(source_qn, target_qn, edge_type, lvl, metadata) AS (
				SELECT e.source_qn, e.target_qn, e.edge_type, 1, ` + metadataCol + `
				FROM edges e
				JOIN projects p ON p.id = e.project_id
				WHERE p.name = ? AND ` + match + ` AND e.edge_type IN (` + edgeTypes + `)
				UNION ALL
				SELECT e.source_qn, e.target_qn, e.edge_type, c.lvl + 1, ` + metadataCol + `
				FROM edges e
				JOIN chain c ON e.target_qn = c.source_qn
				JOIN projects p ON p.id = e.project_id
				WHERE p.name = ? AND e.edge_type IN (` + edgeTypes + `) AND c.lvl < ?
			)
			SELECT c.source_qn, c.target_qn, c.edge_type, c.lvl,
			       COALESCE(ns.file_path, ''), COALESCE(nt.file_path, ''),
			       COALESCE(ns.line, 0), c.metadata
			FROM chain c
			LEFT JOIN nodes ns ON ns.qualified_name = c.source_qn
			LEFT JOIN nodes nt ON nt.qualified_name = c.target_qn
			ORDER BY c.lvl, c.source_qn
		`
	case "outbound":
		args = []any{req.Project, req.FunctionName, req.Project, req.Depth}
		query = `
			WITH RECURSIVE chain(source_qn, target_qn, edge_type, lvl, metadata) AS (
				SELECT e.source_qn, e.target_qn, e.edge_type, 1, ` + metadataCol + `
				FROM edges e
				JOIN projects p ON p.id = e.project_id
				WHERE p.name = ? AND e.source_qn = ? AND e.edge_type IN (` + edgeTypes + `)
				UNION ALL
				SELECT e.source_qn, e.target_qn, e.edge_type, c.lvl + 1, ` + metadataCol + `
				FROM edges e
				JOIN chain c ON e.source_qn = c.target_qn
				JOIN projects p ON p.id = e.project_id
				WHERE p.name = ? AND e.edge_type IN (` + edgeTypes + `) AND c.lvl < ?
			)
			SELECT c.source_qn, c.target_qn, c.edge_type, c.lvl,
			       COALESCE(ns.file_path, ''), COALESCE(nt.file_path, ''),
			       COALESCE(ns.line, 0), c.metadata
			FROM chain c
			LEFT JOIN nodes ns ON ns.qualified_name = c.source_qn
			LEFT JOIN nodes nt ON nt.qualified_name = c.target_qn
			ORDER BY c.lvl, c.source_qn
		`
	default: // both
		match, matchArgs := s.qnMatchClause(req.Project, req.FunctionName, "e.target_qn")
		args = []any{req.Project, req.FunctionName}
		args = append(args, matchArgs...)
		args = append(args, req.Project, req.Depth)
		query = `
			WITH RECURSIVE chain(source_qn, target_qn, edge_type, lvl, metadata) AS (
				SELECT e.source_qn, e.target_qn, e.edge_type, 1, ` + metadataCol + `
				FROM edges e
				JOIN projects p ON p.id = e.project_id
				WHERE p.name = ? AND (e.source_qn = ? OR ` + match + `) AND e.edge_type IN (` + edgeTypes + `)
				UNION ALL
				SELECT e.source_qn, e.target_qn, e.edge_type, c.lvl + 1, ` + metadataCol + `
				FROM edges e
				JOIN chain c ON (e.source_qn = c.target_qn OR e.target_qn = c.source_qn)
				JOIN projects p ON p.id = e.project_id
				WHERE p.name = ? AND e.edge_type IN (` + edgeTypes + `) AND c.lvl < ?
			)
			SELECT c.source_qn, c.target_qn, c.edge_type, c.lvl,
			       COALESCE(ns.file_path, ''), COALESCE(nt.file_path, ''),
			       COALESCE(ns.line, 0), c.metadata
			FROM chain c
			LEFT JOIN nodes ns ON ns.qualified_name = c.source_qn
			LEFT JOIN nodes nt ON nt.qualified_name = c.target_qn
			ORDER BY c.lvl, c.source_qn
		`
	}

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("trace path: %w", err)
	}
	defer rows.Close()

	var hops []TraceHop
	for rows.Next() {
		var h TraceHop
		var srcFile, tgtFile string
		var line uint32
		var metadata string

		// Scan order: source_qn, target_qn, edge_type, lvl, srcFile, tgtFile, line, metadata
		if err := rows.Scan(&h.SourceQN, &h.TargetQN, &h.EdgeType, &h.Depth, &srcFile, &tgtFile, &line, &metadata); err != nil {
			return nil, fmt.Errorf("scan hop: %w", err)
		}
		h.File = srcFile
		h.Line = line

		// In data_flow mode, metadata contains the call arguments
		if mode == "data_flow" && metadata != "" {
			h.Args = metadata
		}

		// Filter test files if not requested — check both source and target
		if !req.IncludeTests && (isTestFile(srcFile) || isTestFile(tgtFile)) {
			continue
		}

		// Add risk labels
		if req.RiskLabels {
			switch {
			case h.Depth <= 2:
				h.Risk = "HIGH"
			case h.Depth <= 4:
				h.Risk = "MEDIUM"
			default:
				h.Risk = "LOW"
			}
		}

		hops = append(hops, h)
	}
	return hops, rows.Err()
}

// Only SELECT queries are allowed for safety.
func (s *Store) QueryGraph(cql string) ([]map[string]any, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	rows, err := s.db.Query(cql)
	if err != nil {
		return nil, fmt.Errorf("query graph: %w", err)
	}
	defer rows.Close()

	cols, err := rows.Columns()
	if err != nil {
		return nil, fmt.Errorf("get columns: %w", err)
	}

	var results []map[string]any
	for rows.Next() {
		vals := make([]any, len(cols))
		valPtrs := make([]any, len(cols))
		for i := range vals {
			valPtrs[i] = &vals[i]
		}

		if err := rows.Scan(valPtrs...); err != nil {
			return nil, fmt.Errorf("scan row: %w", err)
		}

		row := make(map[string]any)
		for i, col := range cols {
			val := vals[i]
			switch v := val.(type) {
			case []byte:
				row[col] = string(v)
			default:
				row[col] = v
			}
		}
		results = append(results, row)
	}
	return results, rows.Err()
}

// ─── helpers ──────────────────────────────────────────────────────────────────

// qnMatchClause builds an SQL condition that matches a qualified name exactly,
// plus a receiver-stripped fallback when qn refers to a function/method node.
//
// Method calls whose receiver could not be resolved at index time are stored
// with the raw receiver text (e.g. h.dispatchToServer). The fallback matches
// them by method name (LIKE '%.dispatchToServer') so find_references and
// trace_path still find them. Single-dot receivers only — multi-segment
// targets (a.b.method) are more likely qualified references than method calls
// and would produce noisy false positives.
//
// Callers must hold the store lock (mu) — nodeKind runs a query.
func (s *Store) qnMatchClause(project, qn, col string) (string, []any) {
	cond := col + " = ?"
	args := []any{qn}
	idx := strings.LastIndex(qn, ".")
	if idx <= 0 {
		return cond, args
	}
	kind := s.nodeKind(project, qn)
	if kind != "method" && kind != "function" {
		return cond, args
	}
	method := qn[idx+1:]
	if method == "" {
		return cond, args
	}
	// The fallback matches single-dot targets ending in the method name, and
	// excludes edges flagged as package-qualified function calls (metadata
	// "pkg":"true", e.g. fmt.Println) — those are not method invocations.
	// Edges without a pkg flag (pre-fix indexes) still participate.
	cond = "(" + cond + " OR (" + col + " LIKE '%.' || ? AND " + col +
		" NOT LIKE '%.%.%' AND COALESCE(json_extract(e.metadata, '$.pkg'), 0) = 0))"
	args = append(args, method)
	return cond, args
}

// nodeKind returns the kind of a node in a project, or "" when absent.
func (s *Store) nodeKind(project, qn string) string {
	var kind string
	err := s.db.QueryRow(`
		SELECT n.kind FROM nodes n
		JOIN projects p ON p.id = n.project_id
		WHERE p.name = ? AND n.qualified_name = ?
		LIMIT 1`, project, qn).Scan(&kind)
	if err != nil {
		return ""
	}
	return kind
}

func scanEdges(rows *sql.Rows) ([]Edge, error) {
	var edges []Edge
	for rows.Next() {
		var e Edge
		if err := rows.Scan(&e.ID, &e.ProjectID, &e.SourceQN, &e.TargetQN, &e.EdgeType, &e.Metadata, &e.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan edge: %w", err)
		}
		edges = append(edges, e)
	}
	return edges, rows.Err()
}

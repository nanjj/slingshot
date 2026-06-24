package base

import (
	"database/sql"
	"fmt"
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
		args = []any{project, qn, project, depth}
		query = `
			WITH RECURSIVE refs(id, project_id, source_qn, target_qn, edge_type, metadata, created_at, lvl) AS (
				SELECT e.id, e.project_id, e.source_qn, e.target_qn, e.edge_type, e.metadata, e.created_at, 1
				FROM edges e
				JOIN projects p ON p.id = e.project_id
				WHERE p.name = ? AND e.target_qn = ?
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
		args = []any{project, qn, qn, project, depth}
		query = `
			WITH RECURSIVE refs(id, project_id, source_qn, target_qn, edge_type, metadata, created_at, lvl) AS (
				SELECT e.id, e.project_id, e.source_qn, e.target_qn, e.edge_type, e.metadata, e.created_at, 1
				FROM edges e
				JOIN projects p ON p.id = e.project_id
				WHERE p.name = ? AND (e.source_qn = ? OR e.target_qn = ?)
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
func (s *Store) TraceCallChain(qn, project, direction string, depth int) ([]TraceHop, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if depth <= 0 {
		depth = 3
	}
	if depth > 20 {
		depth = 20 // safety limit
	}

	var query string
	var args []any

	switch direction {
	case "inbound":
		args = []any{project, qn, project, depth}
		query = `
			WITH RECURSIVE chain(source_qn, target_qn, edge_type, lvl) AS (
				SELECT e.source_qn, e.target_qn, e.edge_type, 1
				FROM edges e
				JOIN projects p ON p.id = e.project_id
				WHERE p.name = ? AND e.target_qn = ? AND e.edge_type = 'CALLS'
				UNION ALL
				SELECT e.source_qn, e.target_qn, e.edge_type, c.lvl + 1
				FROM edges e
				JOIN chain c ON e.target_qn = c.source_qn
				JOIN projects p ON p.id = e.project_id
				WHERE p.name = ? AND e.edge_type = 'CALLS' AND c.lvl < ?
			)
			SELECT c.source_qn, c.target_qn, c.edge_type, c.lvl,
			       COALESCE(n.file_path, ''), COALESCE(n.line, 0)
			FROM chain c
			LEFT JOIN nodes n ON n.qualified_name = c.source_qn
			ORDER BY c.lvl, c.source_qn
		`
	case "outbound":
		args = []any{project, qn, project, depth}
		query = `
			WITH RECURSIVE chain(source_qn, target_qn, edge_type, lvl) AS (
				SELECT e.source_qn, e.target_qn, e.edge_type, 1
				FROM edges e
				JOIN projects p ON p.id = e.project_id
				WHERE p.name = ? AND e.source_qn = ? AND e.edge_type = 'CALLS'
				UNION ALL
				SELECT e.source_qn, e.target_qn, e.edge_type, c.lvl + 1
				FROM edges e
				JOIN chain c ON e.source_qn = c.target_qn
				JOIN projects p ON p.id = e.project_id
				WHERE p.name = ? AND e.edge_type = 'CALLS' AND c.lvl < ?
			)
			SELECT c.source_qn, c.target_qn, c.edge_type, c.lvl,
			       COALESCE(n.file_path, ''), COALESCE(n.line, 0)
			FROM chain c
			LEFT JOIN nodes n ON n.qualified_name = c.source_qn
			ORDER BY c.lvl, c.source_qn
		`
	default: // both
		args = []any{project, qn, qn, project, depth}
		query = `
			WITH RECURSIVE chain(source_qn, target_qn, edge_type, lvl) AS (
				SELECT e.source_qn, e.target_qn, e.edge_type, 1
				FROM edges e
				JOIN projects p ON p.id = e.project_id
				WHERE p.name = ? AND (e.source_qn = ? OR e.target_qn = ?) AND e.edge_type = 'CALLS'
				UNION ALL
				SELECT e.source_qn, e.target_qn, e.edge_type, c.lvl + 1
				FROM edges e
				JOIN chain c ON (e.source_qn = c.target_qn OR e.target_qn = c.source_qn)
				JOIN projects p ON p.id = e.project_id
				WHERE p.name = ? AND e.edge_type = 'CALLS' AND c.lvl < ?
			)
			SELECT c.source_qn, c.target_qn, c.edge_type, c.lvl,
			       COALESCE(n.file_path, ''), COALESCE(n.line, 0)
			FROM chain c
			LEFT JOIN nodes n ON n.qualified_name = c.source_qn
			ORDER BY c.lvl, c.source_qn
		`
	}


	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("trace call chain: %w", err)
	}
	defer rows.Close()

	var hops []TraceHop
	for rows.Next() {
		var h TraceHop
		if err := rows.Scan(&h.SourceQN, &h.TargetQN, &h.EdgeType, &h.Depth, &h.File, &h.Line); err != nil {
			return nil, fmt.Errorf("scan hop: %w", err)
		}
		hops = append(hops, h)
	}
	return hops, rows.Err()
}

// QueryGraph executes a raw SQL query against the graph tables.
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

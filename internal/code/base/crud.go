package base

import (
	"database/sql"
	"fmt"
	"strings"
)

// ─── Node CRUD ────────────────────────────────────────────────────────────────

// SaveNode inserts or updates a node.
func (s *Store) SaveNode(n *Node) (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	result, err := s.db.Exec(`
		INSERT INTO nodes (
			project_id, qualified_name, kind, name, file_path,
			line, col, end_line, end_col, signature, doc_comment,
			complexity, cognitive, loop_depth, transitive_loop_depth,
			loop_count, recursive, param_count, linear_scan_in_loop,
			alloc_in_loop, recursion_in_loop, unguarded_recursion, max_access_depth
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(project_id, qualified_name) DO UPDATE SET
			kind=excluded.kind, name=excluded.name, file_path=excluded.file_path,
			line=excluded.line, col=excluded.col, end_line=excluded.end_line, end_col=excluded.end_col,
			signature=excluded.signature, doc_comment=excluded.doc_comment,
			complexity=excluded.complexity, cognitive=excluded.cognitive,
			loop_depth=excluded.loop_depth, transitive_loop_depth=excluded.transitive_loop_depth,
			loop_count=excluded.loop_count, recursive=excluded.recursive,
			param_count=excluded.param_count, linear_scan_in_loop=excluded.linear_scan_in_loop,
			alloc_in_loop=excluded.alloc_in_loop, recursion_in_loop=excluded.recursion_in_loop,
			unguarded_recursion=excluded.unguarded_recursion, max_access_depth=excluded.max_access_depth
	`,
		n.ProjectID, n.QualifiedName, n.Kind, n.Name, n.FilePath,
		n.Line, n.Col, n.EndLine, n.EndCol, n.Signature, n.DocComment,
		n.Complexity, n.Cognitive, n.LoopDepth, n.TransitiveLoopDepth,
		n.LoopCount, boolToInt(n.Recursive), n.ParamCount, n.LinearScanInLoop,
		n.AllocInLoop, boolToInt(n.RecursionInLoop), boolToInt(n.UnguardedRecursion), n.MaxAccessDepth,
	)
	if err != nil {
		return 0, fmt.Errorf("save node: %w", err)
	}
	id, err := result.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("get last insert id: %w", err)
	}
	return id, nil
}

// SaveNodesBatch inserts multiple nodes in a transaction.
func (s *Store) SaveNodesBatch(nodes []Node) error {
	if len(nodes) == 0 {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	stmt, err := tx.Prepare(`
		INSERT INTO nodes (
			project_id, qualified_name, kind, name, file_path,
			line, col, end_line, end_col, signature, doc_comment,
			complexity, cognitive, loop_depth, transitive_loop_depth,
			loop_count, recursive, param_count, linear_scan_in_loop,
			alloc_in_loop, recursion_in_loop, unguarded_recursion, max_access_depth
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(project_id, qualified_name) DO UPDATE SET
			kind=excluded.kind, name=excluded.name, file_path=excluded.file_path,
			line=excluded.line, col=excluded.col, end_line=excluded.end_line, end_col=excluded.end_col,
			signature=excluded.signature, doc_comment=excluded.doc_comment,
			complexity=excluded.complexity, cognitive=excluded.cognitive,
			loop_depth=excluded.loop_depth, transitive_loop_depth=excluded.transitive_loop_depth,
			loop_count=excluded.loop_count, recursive=excluded.recursive,
			param_count=excluded.param_count, linear_scan_in_loop=excluded.linear_scan_in_loop,
			alloc_in_loop=excluded.alloc_in_loop, recursion_in_loop=excluded.recursion_in_loop,
			unguarded_recursion=excluded.unguarded_recursion, max_access_depth=excluded.max_access_depth
	`)
	if err != nil {
		return fmt.Errorf("prepare stmt: %w", err)
	}
	defer stmt.Close()

	for _, n := range nodes {
		_, err := stmt.Exec(
			n.ProjectID, n.QualifiedName, n.Kind, n.Name, n.FilePath,
			n.Line, n.Col, n.EndLine, n.EndCol, n.Signature, n.DocComment,
			n.Complexity, n.Cognitive, n.LoopDepth, n.TransitiveLoopDepth,
			n.LoopCount, boolToInt(n.Recursive), n.ParamCount, n.LinearScanInLoop,
			n.AllocInLoop, boolToInt(n.RecursionInLoop), boolToInt(n.UnguardedRecursion), n.MaxAccessDepth,
		)
		if err != nil {
			return fmt.Errorf("save node %q: %w", n.QualifiedName, err)
		}
	}

	return tx.Commit()
}

// GetNodeByQN retrieves a single node by its qualified name.
func (s *Store) GetNodeByQN(qn, project string) (*Node, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var n Node
	var rec, recInLoop, unguard int
	err := s.db.QueryRow(`
		SELECT n.id, n.project_id, n.qualified_name, n.kind, n.name,
		       n.file_path, n.line, n.col, n.end_line, n.end_col,
		       n.signature, n.doc_comment,
		       n.complexity, n.cognitive, n.loop_depth, n.transitive_loop_depth,
		       n.loop_count, n.recursive, n.param_count, n.linear_scan_in_loop,
		       n.alloc_in_loop, n.recursion_in_loop, n.unguarded_recursion, n.max_access_depth
		FROM nodes n
		JOIN projects p ON p.id = n.project_id
		WHERE n.qualified_name = ? AND p.name = ?
	`, qn, project).Scan(
		&n.ID, &n.ProjectID, &n.QualifiedName, &n.Kind, &n.Name,
		&n.FilePath, &n.Line, &n.Col, &n.EndLine, &n.EndCol,
		&n.Signature, &n.DocComment,
		&n.Complexity, &n.Cognitive, &n.LoopDepth, &n.TransitiveLoopDepth,
		&n.LoopCount, &rec, &n.ParamCount, &n.LinearScanInLoop,
		&n.AllocInLoop, &recInLoop, &unguard, &n.MaxAccessDepth,
	)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("node %q not found in project %q", qn, project)
	}
	if err != nil {
		return nil, fmt.Errorf("get node by qn: %w", err)
	}
	n.Recursive = intToBool(rec)
	n.RecursionInLoop = intToBool(recInLoop)
	n.UnguardedRecursion = intToBool(unguard)
	return &n, nil
}

// FindSymbols finds nodes by pattern (qualified name or name substring) and optional kind filter.
func (s *Store) FindSymbols(pattern, project, kind string, limit, offset int) ([]Node, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var args []any
	var conditions []string

	conditions = append(conditions, "p.name = ?")
	args = append(args, project)

	if pattern != "" {
		conditions = append(conditions, "(n.qualified_name LIKE ? OR n.name LIKE ?)")
		like := "%" + pattern + "%"
		args = append(args, like, like)
	}
	if kind != "" {
		kinds := strings.Split(kind, ",")
		placeholders := make([]string, len(kinds))
		for i, k := range kinds {
			placeholders[i] = "?"
			args = append(args, strings.TrimSpace(k))
		}
		conditions = append(conditions, "n.kind IN ("+strings.Join(placeholders, ",")+")")
	}

	if limit <= 0 {
		limit = 50
	}
	args = append(args, limit, offset)

	query := fmt.Sprintf(`
		SELECT n.id, n.project_id, n.qualified_name, n.kind, n.name,
		       n.file_path, n.line, n.col, n.end_line, n.end_col,
		       n.signature, n.doc_comment,
		       n.complexity, n.cognitive, n.loop_depth, n.transitive_loop_depth,
		       n.loop_count, n.recursive, n.param_count, n.linear_scan_in_loop,
		       n.alloc_in_loop, n.recursion_in_loop, n.unguarded_recursion, n.max_access_depth
		FROM nodes n
		JOIN projects p ON p.id = n.project_id
		WHERE %s
		ORDER BY n.qualified_name
		LIMIT ? OFFSET ?
	`, strings.Join(conditions, " AND "))

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("find symbols: %w", err)
	}
	defer rows.Close()

	return scanNodes(rows)
}

// SearchNodes performs full-text search across nodes using FTS5.
// The query string supports FTS5 syntax (e.g., "function" OR "method").
func (s *Store) SearchNodes(query, project, pathFilter string, limit, offset int) ([]Node, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if limit <= 0 {
		limit = 20
	}

	var args []any
	args = append(args, query)

	var extraJoins string
	var whereExtra string

	if project != "" {
		extraJoins = "JOIN projects p ON p.id = n.project_id"
		whereExtra = "AND p.name = ?"
		args = append(args, project)
	}
	if pathFilter != "" {
		whereExtra += " AND n.file_path LIKE ?"
		args = append(args, "%"+pathFilter+"%")
	}

	args = append(args, limit, offset)

	sqlQuery := fmt.Sprintf(`
		SELECT n.id, n.project_id, n.qualified_name, n.kind, n.name,
		       n.file_path, n.line, n.col, n.end_line, n.end_col,
		       n.signature, n.doc_comment,
		       n.complexity, n.cognitive, n.loop_depth, n.transitive_loop_depth,
		       n.loop_count, n.recursive, n.param_count, n.linear_scan_in_loop,
		       n.alloc_in_loop, n.recursion_in_loop, n.unguarded_recursion, n.max_access_depth
		FROM node_fts f
		JOIN nodes n ON n.id = f.rowid
		%s
		WHERE node_fts MATCH ?
		%s
		ORDER BY rank
		LIMIT ? OFFSET ?
	`, extraJoins, whereExtra)

	rows, err := s.db.Query(sqlQuery, args...)
	if err != nil {
		return nil, fmt.Errorf("search nodes: %w", err)
	}
	defer rows.Close()

	return scanNodes(rows)
}

// ─── Edge CRUD ────────────────────────────────────────────────────────────────

// SaveEdge inserts or updates an edge.
func (s *Store) SaveEdge(e *Edge) (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	result, err := s.db.Exec(`
		INSERT INTO edges (project_id, source_qn, target_qn, edge_type, metadata)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(project_id, source_qn, target_qn, edge_type) DO UPDATE SET
			metadata = excluded.metadata
	`, e.ProjectID, e.SourceQN, e.TargetQN, e.EdgeType, e.Metadata)
	if err != nil {
		return 0, fmt.Errorf("save edge: %w", err)
	}
	return result.LastInsertId()
}

// SaveEdgesBatch inserts multiple edges in a transaction.
func (s *Store) SaveEdgesBatch(edges []Edge) error {
	if len(edges) == 0 {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	stmt, err := tx.Prepare(`
		INSERT INTO edges (project_id, source_qn, target_qn, edge_type, metadata)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(project_id, source_qn, target_qn, edge_type) DO UPDATE SET
			metadata = excluded.metadata
	`)
	if err != nil {
		return fmt.Errorf("prepare stmt: %w", err)
	}
	defer stmt.Close()

	for _, e := range edges {
		_, err := stmt.Exec(e.ProjectID, e.SourceQN, e.TargetQN, e.EdgeType, e.Metadata)
		if err != nil {
			return fmt.Errorf("save edge %s->%s: %w", e.SourceQN, e.TargetQN, err)
		}
	}

	return tx.Commit()
}

// ─── ADR CRUD ─────────────────────────────────────────────────────────────────

// SaveADR creates or updates an ADR.
func (s *Store) SaveADR(project string, a *ADR) (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	result, err := s.db.Exec(`
		INSERT INTO adrs (project_id, title, content, status)
		VALUES ((SELECT id FROM projects WHERE name = ?), ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			title=excluded.title, content=excluded.content,
			status=excluded.status, updated_at=datetime('now')
	`, project, a.Title, a.Content, a.Status)
	if err != nil {
		return 0, fmt.Errorf("save adr: %w", err)
	}
	return result.LastInsertId()
}

// ListADRs returns all ADRs for a project.
func (s *Store) ListADRs(project string) ([]ADR, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	rows, err := s.db.Query(`
		SELECT a.id, a.project_id, a.title, a.content, a.status, a.created_at, a.updated_at
		FROM adrs a
		JOIN projects p ON p.id = a.project_id
		WHERE p.name = ?
		ORDER BY a.created_at DESC
	`, project)
	if err != nil {
		return nil, fmt.Errorf("list adrs: %w", err)
	}
	defer rows.Close()

	var adrs []ADR
	for rows.Next() {
		var a ADR
		if err := rows.Scan(&a.ID, &a.ProjectID, &a.Title, &a.Content, &a.Status, &a.CreatedAt, &a.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan adr: %w", err)
		}
		adrs = append(adrs, a)
	}
	return adrs, rows.Err()
}

// ─── Memo CRUD ────────────────────────────────────────────────────────────────

// SaveMemo creates or updates a memo.
func (s *Store) SaveMemo(project string, m *Memo) (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	result, err := s.db.Exec(`
		INSERT INTO memos (project_id, type, title, content)
		VALUES ((SELECT id FROM projects WHERE name = ?), ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			type=excluded.type, title=excluded.title, content=excluded.content,
			updated_at=datetime('now')
	`, project, m.Type, m.Title, m.Content)
	if err != nil {
		return 0, fmt.Errorf("save memo: %w", err)
	}
	return result.LastInsertId()
}

// SearchMemos searches memos by FTS (via LIKE for now, later FTS5).
func (s *Store) SearchMemos(project, query, typeFilter string, limit int) ([]Memo, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if limit <= 0 {
		limit = 20
	}

	var args []any
	args = append(args, project)

	var whereExtra string
	if query != "" {
		whereExtra = "AND (m.title LIKE ? OR m.content LIKE ?)"
		like := "%" + query + "%"
		args = append(args, like, like)
	}
	if typeFilter != "" {
		whereExtra += " AND m.type = ?"
		args = append(args, typeFilter)
	}

	rows, err := s.db.Query(fmt.Sprintf(`
		SELECT m.id, m.project_id, m.type, m.title, m.content, m.created_at, m.updated_at
		FROM memos m
		JOIN projects p ON p.id = m.project_id
		WHERE p.name = ?
		%s
		ORDER BY m.updated_at DESC
		LIMIT ?
	`, whereExtra), append(args, limit)...)
	if err != nil {
		return nil, fmt.Errorf("search memos: %w", err)
	}
	defer rows.Close()

	var memos []Memo
	for rows.Next() {
		var m Memo
		if err := rows.Scan(&m.ID, &m.ProjectID, &m.Type, &m.Title, &m.Content, &m.CreatedAt, &m.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan memo: %w", err)
		}
		memos = append(memos, m)
	}
	return memos, rows.Err()
}

// ─── helpers ──────────────────────────────────────────────────────────────────

func scanNodes(rows *sql.Rows) ([]Node, error) {
	var nodes []Node
	for rows.Next() {
		var n Node
		var rec, recInLoop, unguard int
		err := rows.Scan(
			&n.ID, &n.ProjectID, &n.QualifiedName, &n.Kind, &n.Name,
			&n.FilePath, &n.Line, &n.Col, &n.EndLine, &n.EndCol,
			&n.Signature, &n.DocComment,
			&n.Complexity, &n.Cognitive, &n.LoopDepth, &n.TransitiveLoopDepth,
			&n.LoopCount, &rec, &n.ParamCount, &n.LinearScanInLoop,
			&n.AllocInLoop, &recInLoop, &unguard, &n.MaxAccessDepth,
		)
		if err != nil {
			return nil, fmt.Errorf("scan node: %w", err)
		}
		n.Recursive = intToBool(rec)
		n.RecursionInLoop = intToBool(recInLoop)
		n.UnguardedRecursion = intToBool(unguard)
		nodes = append(nodes, n)
	}
	return nodes, rows.Err()
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

func intToBool(v int) bool {
	return v != 0
}

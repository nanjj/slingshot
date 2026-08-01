package base

import (
	"database/sql"
	"fmt"
	"strings"
)

// ─── Node CRUD ───────────────────────────────────────────────────────
// GetNodesByFile returns all nodes in a given file for a project.
func (s *Store) GetNodesByFile(filePath, project string) ([]Node, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	query := `
		SELECT n.id, n.project_id, n.qualified_name, n.kind, n.name,
		       n.file_path, n.line, n.col, n.end_line, n.end_col,
		       n.signature, n.doc_comment,
		       n.complexity, n.cognitive, n.loop_depth, n.transitive_loop_depth,
		       n.loop_count, n.recursive, n.param_count, n.linear_scan_in_loop,
		       n.alloc_in_loop, n.recursion_in_loop, n.unguarded_recursion, n.max_access_depth
		FROM nodes n
		JOIN projects p ON p.id = n.project_id
		WHERE p.name = ? AND n.file_path = ?
		ORDER BY n.line ASC
	`
	rows, err := s.db.Query(query, project, filePath)
	if err != nil {
		return nil, fmt.Errorf("get nodes by file: %w", err)
	}
	defer rows.Close()

	return scanNodes(rows)
}

// ─────────

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

// expandFTSQuery OR-extends a plain lowercase multi-word query with its
// space-free (camelCase-merged) form. FTS5's unicode61 tokenizer folds case
// but does not split camelCase: "RegisterTool" is indexed as the single token
// "registertool", so the natural-language query "register tool" (two ANDed
// terms) misses it. The extension `(register AND tool) OR registertool`
// matches both styles. Queries containing anything but lowercase ASCII
// letters, digits, spaces, or hyphens are passed through untouched (they may
// already be FTS5 syntax or case-sensitive input).
func expandFTSQuery(q string) string {
	trimmed := strings.TrimSpace(q)
	if trimmed == "" {
		return q
	}
	for _, r := range trimmed {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9', r == ' ', r == '-':
		default:
			return q
		}
	}
	terms := strings.Fields(trimmed)
	if len(terms) < 2 {
		return q
	}
	merged := strings.ReplaceAll(trimmed, " ", "")
	if merged == trimmed {
		return q
	}
	quoted := make([]string, len(terms))
	for i, t := range terms {
		quoted[i] = `"` + t + `"`
	}
	return "(" + strings.Join(quoted, " AND ") + ") OR \"" + merged + "\""
}

// FindSymbols finds nodes by pattern (qualified name or name substring) and optional kind filter.
func (s *Store) FindSymbols(pattern, project, kind string, limit, offset int) ([]Node, int, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var conditions []string
	var countArgs, selectArgs []any

	conditions = append(conditions, "p.name = ?")
	countArgs = append(countArgs, project)
	selectArgs = append(selectArgs, project)

	if pattern != "" {
		conditions = append(conditions, "(n.qualified_name LIKE ? OR n.name LIKE ?)")
		like := "%" + pattern + "%"
		countArgs = append(countArgs, like, like)
		selectArgs = append(selectArgs, like, like)
	}
	if kind != "" {
		kinds := strings.Split(kind, ",")
		placeholders := make([]string, len(kinds))
		for i, k := range kinds {
			placeholders[i] = "?"
			countArgs = append(countArgs, strings.TrimSpace(k))
			selectArgs = append(selectArgs, strings.TrimSpace(k))
		}
		conditions = append(conditions, "n.kind IN ("+strings.Join(placeholders, ",")+")")
	}

	if limit <= 0 {
		limit = 50
	}

	whereSQL := strings.Join(conditions, " AND ")

	// Total count (before limit)
	var total int
	countQuery := fmt.Sprintf("SELECT COUNT(*) FROM nodes n JOIN projects p ON p.id = n.project_id WHERE %s", whereSQL)
	if err := s.db.QueryRow(countQuery, countArgs...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count symbols: %w", err)
	}

	selectArgs = append(selectArgs, limit, offset)
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
	`, whereSQL)

	rows, err := s.db.Query(query, selectArgs...)
	if err != nil {
		return nil, 0, fmt.Errorf("find symbols: %w", err)
	}
	defer rows.Close()

	nodes, err := scanNodes(rows)
	if err != nil {
		return nil, 0, err
	}

	return nodes, total, nil
}

// SearchNodes performs full-text search across nodes using FTS5.
// The query string supports FTS5 syntax.
// Returns the matching nodes, total count (before limit), and any error.
// BM25 ranking is boosted for Function/Method (2x), Route (1.5x),
// and Class/Struct/Interface (1.3x) nodes.
func (s *Store) SearchNodes(query, project, pathFilter string, limit, offset int) ([]Node, int, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if limit <= 0 {
		limit = 20
	}

	// Natural-language queries ("register tool") are OR-extended with the
	// camelCase-merged form ("registertool") so they match identifiers like
	// RegisterTool, whose FTS5 token is the folded single word (Archimedes
	// feedback: BM25 was case/case-sensitive to camelCase boundaries).
	query = expandFTSQuery(query)

	fromClause := "FROM node_fts f JOIN nodes n ON n.id = f.rowid"
	var extraJoins string
	var whereClauses []string
	var countArgs, selectArgs []any

	whereClauses = append(whereClauses, "node_fts MATCH ?")
	countArgs = append(countArgs, query)
	selectArgs = append(selectArgs, query)

	if project != "" {
		extraJoins = "JOIN projects p ON p.id = n.project_id"
		whereClauses = append(whereClauses, "p.name = ?")
		countArgs = append(countArgs, project)
		selectArgs = append(selectArgs, project)
	}
	if pathFilter != "" {
		whereClauses = append(whereClauses, "n.file_path LIKE ?")
		countArgs = append(countArgs, "%"+pathFilter+"%")
		selectArgs = append(selectArgs, "%"+pathFilter+"%")
	}

	whereSQL := strings.Join(whereClauses, " AND ")

	// Total count (before limit)
	var total int
	countQuery := fmt.Sprintf("SELECT COUNT(*) %s %s WHERE %s",
		fromClause, extraJoins, whereSQL)
	if err := s.db.QueryRow(countQuery, countArgs...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count nodes: %w", err)
	}

	// Results with BM25 structural boosting
	selectArgs = append(selectArgs, limit, offset)
	selectQuery := fmt.Sprintf(`
		SELECT n.id, n.project_id, n.qualified_name, n.kind, n.name,
		       n.file_path, n.line, n.col, n.end_line, n.end_col,
		       n.signature, n.doc_comment,
		       n.complexity, n.cognitive, n.loop_depth, n.transitive_loop_depth,
		       n.loop_count, n.recursive, n.param_count, n.linear_scan_in_loop,
		       n.alloc_in_loop, n.recursion_in_loop, n.unguarded_recursion, n.max_access_depth
		%s %s
		WHERE %s
		ORDER BY rank / CASE
			WHEN n.kind IN ('Function', 'Method') THEN 2.0
			WHEN n.kind = 'Route' THEN 1.5
			WHEN n.kind IN ('Class', 'Struct', 'Interface') THEN 1.3
			ELSE 1.0
		END
		LIMIT ? OFFSET ?
	`, fromClause, extraJoins, whereSQL)

	rows, err := s.db.Query(selectQuery, selectArgs...)
	if err != nil {
		return nil, 0, fmt.Errorf("search nodes: %w", err)
	}
	defer rows.Close()

	nodes, err := scanNodes(rows)
	if err != nil {
		return nil, 0, err
	}

	return nodes, total, nil
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

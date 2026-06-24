package base

import (
	"database/sql"
	"fmt"
	"sync"

	_ "modernc.org/sqlite"
)

// Store provides SQLite-backed code graph storage.
// It manages projects, nodes, edges, ADRs, memos, and traces.
// All public methods are safe for concurrent use.
type Store struct {
	mu   sync.RWMutex
	db   *sql.DB
	path string
}

// OpenStore opens (or creates) a SQLite database at the given path.
// The schema is automatically initialized on first use.
func OpenStore(dbPath string) (*Store, error) {
	db, err := sql.Open("sqlite", dbPath+"?_journal_mode=WAL&_cache_size=-65536")
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}

	// WAL mode + synchronous=NORMAL for better concurrency
	pragmas := []string{
		"PRAGMA journal_mode=WAL",
		"PRAGMA synchronous=NORMAL",
		"PRAGMA busy_timeout=5000",
		"PRAGMA foreign_keys=ON",
	}
	for _, p := range pragmas {
		if _, err := db.Exec(p); err != nil {
			db.Close()
			return nil, fmt.Errorf("set pragma %q: %w", p, err)
		}
	}

	// Initialize schema
	if _, err := db.Exec(schemaDDL); err != nil {
		db.Close()
		return nil, fmt.Errorf("init schema: %w", err)
	}

	return &Store{db: db, path: dbPath}, nil
}

// Close closes the database connection.
func (s *Store) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.db.Close()
}

// Path returns the database file path.
func (s *Store) Path() string { return s.path }

// DB returns the underlying *sql.DB for advanced operations.
// Callers must not close this connection.
func (s *Store) DB() *sql.DB { return s.db }

// ─── Project operations ──────────────────────────────────────────────────────

// UpsertProject creates or updates a project entry.
// Returns the project ID.
func (s *Store) UpsertProject(name, root string) (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	result, err := s.db.Exec(`
		INSERT INTO projects (name, root, status)
		VALUES (?, ?, 'indexing')
		ON CONFLICT(name) DO UPDATE SET
			root = excluded.root,
			status = 'indexing',
			indexed_at = datetime('now')
	`, name, root)
	if err != nil {
		return 0, fmt.Errorf("upsert project: %w", err)
	}
	return result.LastInsertId()
}

// ListProjects returns all indexed projects.
func (s *Store) ListProjects() ([]ProjectInfo, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	rows, err := s.db.Query(`
		SELECT p.id, p.name, p.root, p.indexed_at, p.status, p.meta,
		       (SELECT COUNT(*) FROM nodes WHERE project_id = p.id) AS node_count,
		       (SELECT COUNT(*) FROM edges WHERE project_id = p.id) AS edge_count
		FROM projects p
		ORDER BY p.name
	`)
	if err != nil {
		return nil, fmt.Errorf("list projects: %w", err)
	}
	defer rows.Close()

	var projects []ProjectInfo
	for rows.Next() {
		var p ProjectInfo
		if err := rows.Scan(&p.ID, &p.Name, &p.Root, &p.IndexedAt, &p.Status, &p.Meta,
			&p.NodeCount, &p.EdgeCount); err != nil {
			return nil, fmt.Errorf("scan project: %w", err)
		}
		projects = append(projects, p)
	}
	return projects, rows.Err()
}

// ProjectStatus returns the status of a project.
func (s *Store) ProjectStatus(name string) (*ProjectInfo, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var p ProjectInfo
	err := s.db.QueryRow(`
		SELECT p.id, p.name, p.root, p.indexed_at, p.status, p.meta,
		       (SELECT COUNT(*) FROM nodes WHERE project_id = p.id),
		       (SELECT COUNT(*) FROM edges WHERE project_id = p.id)
		FROM projects p WHERE p.name = ?
	`, name).Scan(&p.ID, &p.Name, &p.Root, &p.IndexedAt, &p.Status, &p.Meta,
		&p.NodeCount, &p.EdgeCount)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("project %q not found", name)
	}
	if err != nil {
		return nil, fmt.Errorf("get project status: %w", err)
	}
	return &p, nil
}

// DeleteProject removes a project and all its data.
func (s *Store) DeleteProject(name string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	result, err := s.db.Exec("DELETE FROM projects WHERE name = ?", name)
	if err != nil {
		return fmt.Errorf("delete project: %w", err)
	}
	n, _ := result.RowsAffected()
	if n == 0 {
		return fmt.Errorf("project %q not found", name)
	}
	return nil
}

// SetProjectStatus updates the status of a project.
func (s *Store) SetProjectStatus(name, status string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	_, err := s.db.Exec("UPDATE projects SET status = ? WHERE name = ?", status, name)
	return err
}

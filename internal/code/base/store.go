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
	// Run schema migrations
	store := &Store{db: db, path: dbPath}
	if err := store.migrateSchema(); err != nil {
		db.Close()
		return nil, fmt.Errorf("migrate schema: %w", err)
	}

	return store, nil
}

// ─── Schema Migration ─────────────────────────────────────────────────────────

// migrateSchema handles schema changes that can't be done with CREATE IF NOT EXISTS.
// It uses a schema_migrations table to track applied migrations.
func (s *Store) migrateSchema() error {
	// Create migrations tracking table
	_, err := s.db.Exec(`
		CREATE TABLE IF NOT EXISTS schema_migrations (
			version   INTEGER PRIMARY KEY,
			applied_at TEXT NOT NULL DEFAULT (datetime('now'))
		)
	`)
	if err != nil {
		return fmt.Errorf("create migrations table: %w", err)
	}

	// Migration 1: Add TESTS/TESTS_FILE to edges CHECK constraint
	var hasV1 int
	err = s.db.QueryRow("SELECT COUNT(*) FROM schema_migrations WHERE version = 1").Scan(&hasV1)
	if err != nil {
		return fmt.Errorf("check migration v1: %w", err)
	}
	if hasV1 == 0 {
		if err := s.migrateV1(); err != nil {
			return fmt.Errorf("migrate v1: %w", err)
		}
		_, err = s.db.Exec("INSERT INTO schema_migrations (version) VALUES (1)")
		if err != nil {
			return fmt.Errorf("record migration v1: %w", err)
		}
	}

	return nil
}

// migrateV1 recreates the edges table to add TESTS and TESTS_FILE to the CHECK constraint.
func (s *Store) migrateV1() error {
	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	// Create new edges table with updated CHECK constraint
	_, err = tx.Exec(`
		CREATE TABLE IF NOT EXISTS edges_v2 (
			id          INTEGER PRIMARY KEY AUTOINCREMENT,
			project_id  INTEGER NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
			source_qn   TEXT    NOT NULL,
			target_qn   TEXT    NOT NULL,
			edge_type   TEXT    NOT NULL CHECK(edge_type IN ('CALLS','IMPLEMENTS','CONTAINS','IMPORTS','REFERENCES','DATA_FLOWS','DEFINES','TESTS','TESTS_FILE')),
			metadata    TEXT    DEFAULT '{}',
			created_at  TEXT    NOT NULL DEFAULT (datetime('now')),
			UNIQUE(project_id, source_qn, target_qn, edge_type)
		)
	`)
	if err != nil {
		return fmt.Errorf("create edges_v2: %w", err)
	}

	// Copy existing data
	_, err = tx.Exec(`
		INSERT OR IGNORE INTO edges_v2 (id, project_id, source_qn, target_qn, edge_type, metadata, created_at)
		SELECT id, project_id, source_qn, target_qn, edge_type, metadata, created_at FROM edges
	`)
	if err != nil {
		return fmt.Errorf("copy edges: %w", err)
	}

	// Drop old table and rename new one
	_, err = tx.Exec("DROP TABLE edges")
	if err != nil {
		return fmt.Errorf("drop edges: %w", err)
	}
	_, err = tx.Exec("ALTER TABLE edges_v2 RENAME TO edges")
	if err != nil {
		return fmt.Errorf("rename edges_v2: %w", err)
	}

	// Recreate indexes
	_, err = tx.Exec("CREATE INDEX IF NOT EXISTS idx_edges_source ON edges(project_id, source_qn)")
	if err != nil {
		return fmt.Errorf("create idx_edges_source: %w", err)
	}
	_, err = tx.Exec("CREATE INDEX IF NOT EXISTS idx_edges_target ON edges(project_id, target_qn)")
	if err != nil {
		return fmt.Errorf("create idx_edges_target: %w", err)
	}
	_, err = tx.Exec("CREATE INDEX IF NOT EXISTS idx_edges_type ON edges(project_id, edge_type)")
	if err != nil {
		return fmt.Errorf("create idx_edges_type: %w", err)
	}

	return tx.Commit()
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
	return s.listProjectsLocked()
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

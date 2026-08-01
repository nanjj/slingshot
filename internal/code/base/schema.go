package base

// Schema DDL for the code graph SQLite database.
// All tables use IF NOT EXISTS for idempotent creation.
//
// The schema is organized into:
//   projects — indexed project roots
//   nodes    — code symbols (functions, classes, etc.)
//   edges    — relationships between nodes
//   adrs     — Architecture Decision Records
//   memos    — persistent memory entries
//   traces   — runtime trace spans
//   node_fts — FTS5 full-text search over nodes

const schemaDDL = `
-- Projects table
CREATE TABLE IF NOT EXISTS projects (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    name        TEXT    NOT NULL UNIQUE,
    root        TEXT    NOT NULL,
    indexed_at  TEXT    NOT NULL DEFAULT (datetime('now')),
    status      TEXT    NOT NULL DEFAULT 'indexing',
    meta        TEXT    DEFAULT '{}'
);

-- Nodes table: code symbols with complexity metrics
CREATE TABLE IF NOT EXISTS nodes (
    id                   INTEGER PRIMARY KEY AUTOINCREMENT,
    project_id           INTEGER NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    qualified_name       TEXT    NOT NULL,
    kind                 TEXT    NOT NULL,
    name                 TEXT    NOT NULL,
    file_path            TEXT    NOT NULL,
    line                 INTEGER NOT NULL DEFAULT 0,
    col                  INTEGER NOT NULL DEFAULT 0,
    end_line             INTEGER NOT NULL DEFAULT 0,
    end_col              INTEGER NOT NULL DEFAULT 0,
    signature            TEXT    DEFAULT '',
    doc_comment          TEXT    DEFAULT '',
    complexity           INTEGER NOT NULL DEFAULT 0,
    cognitive            INTEGER NOT NULL DEFAULT 0,
    loop_depth           INTEGER NOT NULL DEFAULT 0,
    transitive_loop_depth INTEGER NOT NULL DEFAULT 0,
    loop_count           INTEGER NOT NULL DEFAULT 0,
    recursive            INTEGER NOT NULL DEFAULT 0,
    param_count          INTEGER NOT NULL DEFAULT 0,
    linear_scan_in_loop  INTEGER NOT NULL DEFAULT 0,
    alloc_in_loop        INTEGER NOT NULL DEFAULT 0,
    recursion_in_loop    INTEGER NOT NULL DEFAULT 0,
    unguarded_recursion  INTEGER NOT NULL DEFAULT 0,
    max_access_depth     INTEGER NOT NULL DEFAULT 0,
    created_at           TEXT    NOT NULL DEFAULT (datetime('now')),
    UNIQUE(project_id, qualified_name)
);

CREATE INDEX IF NOT EXISTS idx_nodes_project ON nodes(project_id);
CREATE INDEX IF NOT EXISTS idx_nodes_kind ON nodes(project_id, kind);
CREATE INDEX IF NOT EXISTS idx_nodes_file ON nodes(project_id, file_path);
CREATE INDEX IF NOT EXISTS idx_nodes_name ON nodes(project_id, name);

-- Edges table: relationships between nodes
CREATE TABLE IF NOT EXISTS edges (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    project_id  INTEGER NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    source_qn   TEXT    NOT NULL,
    target_qn   TEXT    NOT NULL,
    edge_type   TEXT    NOT NULL CHECK(edge_type IN ('CALLS','IMPLEMENTS','CONTAINS','IMPORTS','REFERENCES','DATA_FLOWS','DEFINES','TESTS','TESTS_FILE')),
    metadata    TEXT    DEFAULT '{}',
    created_at  TEXT    NOT NULL DEFAULT (datetime('now')),
    UNIQUE(project_id, source_qn, target_qn, edge_type)
);

CREATE INDEX IF NOT EXISTS idx_edges_source ON edges(project_id, source_qn);
CREATE INDEX IF NOT EXISTS idx_edges_target ON edges(project_id, target_qn);
CREATE INDEX IF NOT EXISTS idx_edges_type ON edges(project_id, edge_type);

-- ADRs: Architecture Decision Records
CREATE TABLE IF NOT EXISTS adrs (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    project_id  INTEGER NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    title       TEXT    NOT NULL,
    content     TEXT    NOT NULL DEFAULT '',
    status      TEXT    NOT NULL DEFAULT 'proposed',
    created_at  TEXT    NOT NULL DEFAULT (datetime('now')),
    updated_at  TEXT    NOT NULL DEFAULT (datetime('now'))
);

CREATE INDEX IF NOT EXISTS idx_adrs_project ON adrs(project_id);

-- Memos: persistent memory entries
CREATE TABLE IF NOT EXISTS memos (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    project_id  INTEGER NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    type        TEXT    NOT NULL DEFAULT 'learning',
    title       TEXT    NOT NULL,
    content     TEXT    NOT NULL DEFAULT '',
    created_at  TEXT    NOT NULL DEFAULT (datetime('now')),
    updated_at  TEXT    NOT NULL DEFAULT (datetime('now'))
);

CREATE INDEX IF NOT EXISTS idx_memos_project ON memos(project_id);
CREATE INDEX IF NOT EXISTS idx_memos_type ON memos(project_id, type);

-- Traces: runtime trace spans
CREATE TABLE IF NOT EXISTS traces (
    id              INTEGER PRIMARY KEY AUTOINCREMENT,
    project_id      INTEGER NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    trace_id        TEXT    NOT NULL,
    span_id         TEXT    NOT NULL,
    parent_span_id  TEXT    DEFAULT '',
    service         TEXT    NOT NULL DEFAULT '',
    operation       TEXT    NOT NULL DEFAULT '',
    start_time      INTEGER NOT NULL DEFAULT 0,
    duration        INTEGER NOT NULL DEFAULT 0,
    status          TEXT    DEFAULT 'ok',
    tags            TEXT    DEFAULT '{}'
);

CREATE INDEX IF NOT EXISTS idx_traces_trace ON traces(project_id, trace_id);
CREATE INDEX IF NOT EXISTS idx_traces_service ON traces(project_id, service);

-- mcp_state: cross-session MCP server state (e.g. the last project bound by
-- open_project, restored after a server restart / panic so tool calls without
-- an explicit project keep working).
CREATE TABLE IF NOT EXISTS mcp_state (
    key   TEXT PRIMARY KEY,
    value TEXT NOT NULL
);

-- FTS5 virtual table for full-text search over nodes
CREATE VIRTUAL TABLE IF NOT EXISTS node_fts USING fts5(
    qualified_name,
    name,
    kind,
    file_path,
    signature,
    doc_comment,
    content='nodes',
    content_rowid='id',
    tokenize='unicode61'
);

-- Triggers to keep FTS in sync with nodes table
CREATE TRIGGER IF NOT EXISTS node_fts_insert AFTER INSERT ON nodes BEGIN
    INSERT INTO node_fts(rowid, qualified_name, name, kind, file_path, signature, doc_comment)
    VALUES (new.id, new.qualified_name, new.name, new.kind, new.file_path, new.signature, new.doc_comment);
END;

CREATE TRIGGER IF NOT EXISTS node_fts_delete AFTER DELETE ON nodes BEGIN
    INSERT INTO node_fts(node_fts, rowid, qualified_name, name, kind, file_path, signature, doc_comment)
    VALUES ('delete', old.id, old.qualified_name, old.name, old.kind, old.file_path, old.signature, old.doc_comment);
END;

CREATE TRIGGER IF NOT EXISTS node_fts_update AFTER UPDATE ON nodes BEGIN
    INSERT INTO node_fts(node_fts, rowid, qualified_name, name, kind, file_path, signature, doc_comment)
    VALUES ('delete', old.id, old.qualified_name, old.name, old.kind, old.file_path, old.signature, old.doc_comment);
    INSERT INTO node_fts(rowid, qualified_name, name, kind, file_path, signature, doc_comment)
    VALUES (new.id, new.qualified_name, new.name, new.kind, new.file_path, new.signature, new.doc_comment);
END;
`

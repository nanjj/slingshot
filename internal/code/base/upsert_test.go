package base

import (
	"os"
	"path/filepath"
	"testing"

	"gotest.tools/v3/assert"
)

// ─── Regression: LastInsertId() after ON CONFLICT DO UPDATE ─────────────────
//
// UpsertProject / SaveNode / SaveEdge run INSERT ... ON CONFLICT ... DO UPDATE.
// SQLite does not touch last_insert_rowid() when the DO UPDATE branch fires,
// so LastInsertId() on a fresh (or reused) pooled connection reports 0 or a
// stale rowid. For UpsertProject that 0 flows into every node's project_id and
// trips the FOREIGN KEY constraint during project re-indexing
// ("constraint failed: FOREIGN KEY constraint failed (787)").
//
// SetMaxIdleConns(0) forces the second call onto a brand-new connection whose
// last_insert_rowid() starts at 0, making the stale-0 path deterministic.

func TestUpsertProject_ExistingProjectReturnsRealID(t *testing.T) {
	store := openTestStore(t)
	defer store.Close()

	pid1, err := store.UpsertProject("dscli", "/tmp/dscli")
	assert.NilError(t, err)
	assert.Assert(t, pid1 > 0, "first upsert should return a real id, got %d", pid1)

	// Force the next statement onto a fresh connection (last_insert_rowid = 0).
	store.db.SetMaxIdleConns(0)

	pid2, err := store.UpsertProject("dscli", "/tmp/dscli")
	assert.NilError(t, err)
	assert.Assert(t, pid2 > 0, "re-upsert of an existing project must return a real id, got %d (bug: LastInsertId() was stale)", pid2)
	assert.Equal(t, pid1, pid2, "re-upsert must return the same project id")
}

func TestIndexProject_RebuildExistingProject(t *testing.T) {
	dir := t.TempDir()
	src := `package main

import "fmt"

// Greeter greets.
type Greeter struct{ Name string }

func (g Greeter) Greet() string { return "hi " + g.Name }

func main() {
	g := Greeter{Name: "world"}
	fmt.Println(g.Greet())
}
`
	assert.NilError(t, os.WriteFile(filepath.Join(dir, "main.go"), []byte(src), 0o644))

	store := openTestStore(t)
	defer store.Close()

	first, err := store.IndexProject(dir, "dscli", IndexModeFull)
	assert.NilError(t, err)
	assert.Assert(t, first.NodesStored > 0, "first index should store nodes")

	// Simulate the reported failure: re-index an already-known project on a
	// fresh connection (server restart / cold pool), exactly the setup that
	// produced "FOREIGN KEY constraint failed (787)" on dscli.
	store.db.SetMaxIdleConns(0)

	second, err := store.IndexProject(dir, "dscli", IndexModeFull)
	assert.NilError(t, err, "re-index of an existing project must succeed (regression: stale LastInsertId → project_id=0 → FK failure)")
	assert.Assert(t, second.NodesStored > 0, "re-index should store nodes")
}

func TestSaveNode_UpdateReturnsRealID(t *testing.T) {
	store := openTestStore(t)
	defer store.Close()

	pid, err := store.UpsertProject("dscli", "/tmp/dscli")
	assert.NilError(t, err)

	n := &Node{ProjectID: pid, QualifiedName: "main.Greet", Kind: "method", Name: "Greet", FilePath: "main.go"}
	id1, err := store.SaveNode(n)
	assert.NilError(t, err)
	assert.Assert(t, id1 > 0, "first save should return a real id, got %d", id1)

	store.db.SetMaxIdleConns(0)

	n.Line = 42 // force an UPDATE branch
	id2, err := store.SaveNode(n)
	assert.NilError(t, err)
	assert.Assert(t, id2 > 0, "update of an existing node must return a real id, got %d", id2)
	assert.Equal(t, id1, id2, "update must return the same node id")
}

func TestSaveEdge_UpdateReturnsRealID(t *testing.T) {
	store := openTestStore(t)
	defer store.Close()

	pid, err := store.UpsertProject("dscli", "/tmp/dscli")
	assert.NilError(t, err)

	e := &Edge{ProjectID: pid, SourceQN: "main.main", TargetQN: "main.Greet", EdgeType: "CALLS", Metadata: `{"callee":"Greet"}`}
	id1, err := store.SaveEdge(e)
	assert.NilError(t, err)
	assert.Assert(t, id1 > 0, "first save should return a real id, got %d", id1)

	store.db.SetMaxIdleConns(0)

	e.Metadata = `{"callee":"g.Greet"}` // force an UPDATE branch
	id2, err := store.SaveEdge(e)
	assert.NilError(t, err)
	assert.Assert(t, id2 > 0, "update of an existing edge must return a real id, got %d", id2)
	assert.Equal(t, id1, id2, "update must return the same edge id")
}

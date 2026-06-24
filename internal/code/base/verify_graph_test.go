package base

import (
	"os"
	"path/filepath"
	"testing"

	"gotest.tools/v3/assert"
)

func TestGraphEnhanced(t *testing.T) {
	dir := t.TempDir()
	src := `package main

import "fmt"

type Greeter struct {
	Name string
}

func (g *Greeter) Hello() string {
	return "Hello, " + g.Name + "!"
}

func main() {
	g := &Greeter{Name: "World"}
	fmt.Println(g.Hello())
}
`
	err := os.WriteFile(filepath.Join(dir, "main.go"), []byte(src), 0644)
	assert.NilError(t, err)

	dbPath := filepath.Join(t.TempDir(), "graph.db")
	store, err := OpenStore(dbPath)
	assert.NilError(t, err)
	defer store.Close()

	result, err := store.IndexProject(dir, "graphtest", IndexModeFull)
	assert.NilError(t, err)

	// ── Node assertions ──
	// Expect: file node, package node, function/method nodes
	assert.Assert(t, result.NodesStored >= 3,
		"expected ≥3 nodes (file + package + defs), got %d", result.NodesStored)

	// Verify a file node exists
	fileNodes, _, err := store.FindSymbols("", "graphtest", "file", 10, 0)
	assert.NilError(t, err)
	assert.Assert(t, len(fileNodes) > 0, "expected at least one file node")

	// Verify a package node exists
	pkgNodes, _, err := store.FindSymbols("", "graphtest", "package", 10, 0)
	assert.NilError(t, err)
	assert.Assert(t, len(pkgNodes) > 0, "expected at least one package node")

	// ── Edge assertions ──
	// Expect: DEFINES, CONTAINS, IMPORTS, CALLS, CONTAINS (method parent)
	assert.Assert(t, result.EdgesStored >= 4,
		"expected ≥4 edges, got %d", result.EdgesStored)

	// Count edges per type
	rows, err := store.DB().Query(`
		SELECT edge_type, COUNT(*) FROM edges e
		JOIN projects p ON p.id = e.project_id
		WHERE p.name = 'graphtest'
		GROUP BY edge_type
	`)
	assert.NilError(t, err)
	defer rows.Close()

	edgeTypes := map[string]int{}
	for rows.Next() {
		var typ string
		var cnt int
		err := rows.Scan(&typ, &cnt)
		assert.NilError(t, err)
		edgeTypes[typ] = cnt
	}

	t.Logf("Edge counts: DEFINES=%d CONTAINS=%d IMPORTS=%d CALLS=%d IMPLEMENTS=%d",
		edgeTypes["DEFINES"], edgeTypes["CONTAINS"], edgeTypes["IMPORTS"],
		edgeTypes["CALLS"], edgeTypes["IMPLEMENTS"])

	assert.Assert(t, edgeTypes["DEFINES"] >= 2,
		"expected ≥2 DEFINES edges, got %d", edgeTypes["DEFINES"])
	assert.Assert(t, edgeTypes["CONTAINS"] >= 1,
		"expected ≥1 CONTAINS edge (package→file), got %d", edgeTypes["CONTAINS"])
	assert.Assert(t, edgeTypes["IMPORTS"] >= 1,
		"expected ≥1 IMPORTS edge, got %d", edgeTypes["IMPORTS"])
	assert.Assert(t, edgeTypes["CALLS"] >= 1,
		"expected ≥1 CALLS edge, got %d", edgeTypes["CALLS"])
}

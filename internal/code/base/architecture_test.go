package base

import (
	"path/filepath"
	"testing"

	"gotest.tools/v3/assert"
)

// ─── extractPackage ──────────────────────────────────────────────────────────

func TestExtractPackage(t *testing.T) {
	tests := []struct {
		qn      string
		project string
		want    string
	}{
		{
			qn:      "proj.pkg1.pkg2.FuncName",
			project: "proj",
			want:    "pkg1.pkg2",
		},
		{
			qn:      "proj.pkg1.FuncName",
			project: "proj",
			want:    "pkg1",
		},
		{
			qn:      "proj.FuncName",
			project: "proj",
			want:    "FuncName", // no package = the last component alone
		},
		{
			qn:      "my-project.internal.code.base.store.OpenStore",
			project: "my-project",
			want:    "internal.code.base.store",
		},
		{
			qn:      "my-project.internal.code.base.store.Store.UpsertProject",
			project: "my-project",
			want:    "internal.code.base.store.Store",
		},
	}
	for _, tc := range tests {
		got := extractPackage(tc.qn, tc.project)
		assert.Equal(t, got, tc.want, "extractPackage(%q, %q)", tc.qn, tc.project)
	}
}

// ─── Hotspots ────────────────────────────────────────────────────────────────

func TestHotspots_NoData(t *testing.T) {
	store := openTestStore(t)
	defer store.Close()

	hotspots, err := store.Hotspots("unknown", 10)
	assert.NilError(t, err)
	assert.Assert(t, len(hotspots) == 0)
}

func TestHotspots_OrderedByFanIn(t *testing.T) {
	store := openTestStore(t)
	defer store.Close()

	pid := seedProject(t, store, "test-hotspots")

	// Insert nodes: 3 functions
	seedNode(t, store, pid, "proj.pkg.hot", "function", "hot", "pkg/hot.go", 10)
	seedNode(t, store, pid, "proj.pkg.warm", "function", "warm", "pkg/warm.go", 20)
	seedNode(t, store, pid, "proj.pkg.cold", "function", "cold", "pkg/cold.go", 30)

	// CALLS edges: hot gets 5 callers, warm gets 2, cold gets 0
	for i := 0; i < 5; i++ {
		caller := "proj.pkg.caller" + string(rune('A'+i))
		seedNode(t, store, pid, caller, "function", "caller", "pkg/caller.go", uint32(100+i))
		seedEdgeCall(t, store, pid, caller, "proj.pkg.hot")
	}
	for i := 0; i < 2; i++ {
		caller := "proj.pkg.caller" + string(rune('X'+i))
		seedNode(t, store, pid, caller, "function", "caller", "pkg/caller.go", uint32(200+i))
		seedEdgeCall(t, store, pid, caller, "proj.pkg.warm")
	}

	hotspots, err := store.Hotspots("test-hotspots", 10)
	assert.NilError(t, err)
	assert.Assert(t, len(hotspots) >= 2)

	// hot (5) > warm (2)
	assert.Assert(t, hotspots[0].FanIn >= hotspots[1].FanIn,
		"expected %d >= %d", hotspots[0].FanIn, hotspots[1].FanIn)

	// cold should not appear (fan-in = 0 → not returned because JOIN excludes it)
	foundCold := false
	for _, h := range hotspots {
		if h.QualifiedName == "proj.pkg.cold" {
			foundCold = true
		}
	}
	assert.Assert(t, !foundCold, "cold should not appear (zero fan-in)")
}

func TestHotspots_Limit(t *testing.T) {
	store := openTestStore(t)
	defer store.Close()

	pid := seedProject(t, store, "test-hotspots-limit")

	for i := 0; i < 10; i++ {
		fn := "proj.pkg.fn" + string(rune('0'+i))
		seedNode(t, store, pid, fn, "function", fn, "pkg/fn.go", uint32(i*10))
		// Each function is called once
		caller := "proj.pkg.caller" + string(rune('A'+i))
		seedNode(t, store, pid, caller, "function", "caller", "pkg/caller.go", uint32(999+i))
		seedEdgeCall(t, store, pid, caller, fn)
	}

	hotspots, err := store.Hotspots("test-hotspots-limit", 3)
	assert.NilError(t, err)
	assert.Assert(t, len(hotspots) <= 3,
		"limit=3 but got %d", len(hotspots))
}

// ─── PackageDeps ─────────────────────────────────────────────────────────────

func TestPackageDeps_NoData(t *testing.T) {
	store := openTestStore(t)
	defer store.Close()

	deps, err := store.PackageDeps("unknown")
	assert.NilError(t, err)
	assert.Assert(t, len(deps) == 0)
}

func TestPackageDeps_CrossPackage(t *testing.T) {
	store := openTestStore(t)
	defer store.Close()

	pid := seedProject(t, store, "proj")

	// Nodes in pkg-a and pkg-b
	seedNode(t, store, pid, "proj.pkg-a.FuncA1", "function", "FuncA1", "pkg-a/a.go", 10)
	seedNode(t, store, pid, "proj.pkg-a.FuncA2", "function", "FuncA2", "pkg-a/a.go", 20)
	seedNode(t, store, pid, "proj.pkg-b.FuncB1", "function", "FuncB1", "pkg-b/b.go", 30)
	seedNode(t, store, pid, "proj.pkg-b.FuncB2", "function", "FuncB2", "pkg-b/b.go", 40)

	// Cross-package calls: pkg-a → pkg-b (3 calls), pkg-b → pkg-a (1 call)
	seedEdgeCall(t, store, pid, "proj.pkg-a.FuncA1", "proj.pkg-b.FuncB1")
	seedEdgeCall(t, store, pid, "proj.pkg-a.FuncA1", "proj.pkg-b.FuncB2")
	seedEdgeCall(t, store, pid, "proj.pkg-a.FuncA2", "proj.pkg-b.FuncB1")
	seedEdgeCall(t, store, pid, "proj.pkg-b.FuncB1", "proj.pkg-a.FuncA1")

	deps, err := store.PackageDeps("proj")
	assert.NilError(t, err)

	// Should find 2 dependency pairs
	find := func(src, tgt string) *PackageDep {
		for _, d := range deps {
			if d.Source == src && d.Target == tgt {
				return &d
			}
		}
		return nil
	}

	ab := find("pkg-a", "pkg-b")
	assert.Assert(t, ab != nil, "pkg-a → pkg-b not found")
	assert.Equal(t, ab.Count, 3)

	ba := find("pkg-b", "pkg-a")
	assert.Assert(t, ba != nil, "pkg-b → pkg-a not found")
	assert.Equal(t, ba.Count, 1)

	// Self-dependencies (same package) should be excluded
	self := find("pkg-a", "pkg-a")
	assert.Assert(t, self == nil, "self-dependency should be excluded")
}

func TestPackageDeps_SortedByCount(t *testing.T) {
	store := openTestStore(t)
	defer store.Close()

	pid := seedProject(t, store, "proj")

	seedNode(t, store, pid, "proj.a.F1", "function", "F1", "a/a.go", 10)
	seedNode(t, store, pid, "proj.b.F2", "function", "F2", "b/b.go", 20)
	seedNode(t, store, pid, "proj.c.F3", "function", "F3", "c/c.go", 30)

	// a→c: 5 calls (highest)
	for i := 0; i < 5; i++ {
		seedEdgeCall(t, store, pid, "proj.a.F1", "proj.c.F3")
	}
	// b→c: 2 calls
	for i := 0; i < 2; i++ {
		seedEdgeCall(t, store, pid, "proj.b.F2", "proj.c.F3")
	}

	deps, err := store.PackageDeps("proj")
	assert.NilError(t, err)
	assert.Assert(t, len(deps) >= 2)

	// First entry should be the highest count
	assert.Assert(t, deps[0].Count >= deps[1].Count,
		"expected %d >= %d", deps[0].Count, deps[1].Count)
}

// ─── FileTree ────────────────────────────────────────────────────────────────

func TestFileTree_NoData(t *testing.T) {
	store := openTestStore(t)
	defer store.Close()

	tree, err := store.FileTree("unknown")
	assert.NilError(t, err)
	assert.Assert(t, len(tree) == 0)
}

func TestFileTree_GroupedByDir(t *testing.T) {
	store := openTestStore(t)
	defer store.Close()

	pid := seedProject(t, store, "test-filetree")

	// Create nodes across 3 directories
	seedNode(t, store, pid, "proj.a.F1", "function", "F1", "pkg/a/f1.go", 10)
	seedNode(t, store, pid, "proj.a.F2", "function", "F2", "pkg/a/f2.go", 10)
	seedNode(t, store, pid, "proj.b.F3", "function", "F3", "pkg/b/f3.go", 10)
	seedNode(t, store, pid, "proj.root.F4", "function", "F4", "root.go", 10) // top-level

	tree, err := store.FileTree("test-filetree")
	assert.NilError(t, err)

	find := func(dir string) *FileTreeEntry {
		for _, e := range tree {
			if e.Path == dir {
				return &e
			}
		}
		return nil
	}

	a := find("pkg/a")
	assert.Assert(t, a != nil, "pkg/a not found")
	assert.Equal(t, a.FileCount, 2, "pkg/a should have 2 files")
	assert.Equal(t, a.NodeCount, 2, "pkg/a should have 2 nodes")

	b := find("pkg/b")
	assert.Assert(t, b != nil, "pkg/b not found")
	assert.Equal(t, b.FileCount, 1, "pkg/b should have 1 file")
	assert.Equal(t, b.NodeCount, 1, "pkg/b should have 1 node")

	root := find("/")
	assert.Assert(t, root != nil, "root dir not found")
	assert.Assert(t, root.FileCount >= 1, "root should have at least 1 file")
	assert.Assert(t, root.NodeCount >= 1, "root should have at least 1 node")
}

// ─── helpers ─────────────────────────────────────────────────────────────────

func openTestStore(t *testing.T) *Store {
	t.Helper()
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")
	store, err := OpenStore(dbPath)
	assert.NilError(t, err)
	t.Cleanup(func() { store.Close() })
	return store
}

func seedProject(t *testing.T, s *Store, name string) int64 {
	t.Helper()
	pid, err := s.UpsertProject(name, "/tmp/"+name)
	assert.NilError(t, err)
	return pid
}

func seedNode(t *testing.T, s *Store, pid int64, qn, kind, name, path string, line uint32) {
	t.Helper()
	n := &Node{
		ProjectID:     pid,
		QualifiedName: qn,
		Kind:          kind,
		Name:          name,
		FilePath:      path,
		Line:          line,
		EndLine:       line + 5,
	}
	_, err := s.SaveNode(n)
	assert.NilError(t, err)
}

func seedEdgeCall(t *testing.T, s *Store, pid int64, src, tgt string) {
	t.Helper()
	e := &Edge{
		ProjectID: pid,
		SourceQN:  src,
		TargetQN:  tgt,
		EdgeType:  "CALLS",
	}
	_, err := s.SaveEdge(e)
	assert.NilError(t, err)
}

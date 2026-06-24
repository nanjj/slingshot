package base

import (
	"testing"

	"gotest.tools/v3/assert"
)

// ─── ImpactAnalysis ──────────────────────────────────────────────────────────

func TestImpactAnalysis_NoChangedFiles(t *testing.T) {
	store := openTestStore(t)
	defer store.Close()

	impacted, err := store.ImpactAnalysis("proj", nil, 3)
	assert.NilError(t, err)
	assert.Assert(t, len(impacted) == 0, "expected empty for no changed files")

	impacted, err = store.ImpactAnalysis("proj", []string{}, 3)
	assert.NilError(t, err)
	assert.Assert(t, len(impacted) == 0, "expected empty for empty changed files")
}

func TestImpactAnalysis_UnknownProject(t *testing.T) {
	store := openTestStore(t)
	defer store.Close()

	impacted, err := store.ImpactAnalysis("nonexistent", []string{"a.go"}, 3)
	assert.NilError(t, err)
	assert.Assert(t, len(impacted) == 0, "expected empty for unknown project")
}

func TestImpactAnalysis_DirectChanges(t *testing.T) {
	store := openTestStore(t)
	defer store.Close()

	pid := seedProject(t, store, "proj")

	seedNode(t, store, pid, "proj.pkg.FuncA", "function", "FuncA", "a.go", 10)
	seedNode(t, store, pid, "proj.pkg.FuncB", "function", "FuncB", "b.go", 20)

	impacted, err := store.ImpactAnalysis("proj", []string{"a.go"}, 3)
	assert.NilError(t, err)

	// Should find FuncA as directly changed
	foundA := false
	for _, s := range impacted {
		if s.QualifiedName == "proj.pkg.FuncA" {
			foundA = true
			assert.Assert(t, s.Changed, "FuncA should be marked Changed")
			assert.Equal(t, s.ChangeDepth, 0, "FuncA should be depth 0")
			assert.Equal(t, s.FilePath, "a.go")
		}
	}
	assert.Assert(t, foundA, "FuncA should be in impacted set")

	// FuncB should NOT be in impacted set (no CALLS edges)
	for _, s := range impacted {
		if s.QualifiedName == "proj.pkg.FuncB" {
			t.Errorf("FuncB should not be impacted (no call relationship)")
		}
	}
}

func TestImpactAnalysis_PropagationOutbound(t *testing.T) {
	store := openTestStore(t)
	defer store.Close()

	pid := seedProject(t, store, "proj")

	// FuncA calls FuncB
	seedNode(t, store, pid, "proj.pkg.FuncA", "function", "FuncA", "a.go", 10)
	seedNode(t, store, pid, "proj.pkg.FuncB", "function", "FuncB", "b.go", 20)
	seedEdgeCall(t, store, pid, "proj.pkg.FuncA", "proj.pkg.FuncB")

	impacted, err := store.ImpactAnalysis("proj", []string{"a.go"}, 3)
	assert.NilError(t, err)

	find := func(qn string) *ImpactedSymbol {
		for _, s := range impacted {
			if s.QualifiedName == qn {
				return &s
			}
		}
		return nil
	}

	a := find("proj.pkg.FuncA")
	assert.Assert(t, a != nil, "FuncA should be impacted")
	assert.Equal(t, a.ChangeDepth, 0)
	assert.Assert(t, a.Changed)

	// FuncB is called by FuncA → change in FuncA propagates to FuncB
	b := find("proj.pkg.FuncB")
	assert.Assert(t, b != nil, "FuncB should be impacted (called by FuncA)")
	assert.Equal(t, b.ChangeDepth, 1)
	assert.Assert(t, !b.Changed, "FuncB is not directly changed")
}

func TestImpactAnalysis_PropagationInbound(t *testing.T) {
	store := openTestStore(t)
	defer store.Close()

	pid := seedProject(t, store, "proj")

	// FuncA calls FuncB
	seedNode(t, store, pid, "proj.pkg.FuncA", "function", "FuncA", "a.go", 10)
	seedNode(t, store, pid, "proj.pkg.FuncB", "function", "FuncB", "b.go", 20)
	seedEdgeCall(t, store, pid, "proj.pkg.FuncA", "proj.pkg.FuncB")

	impacted, err := store.ImpactAnalysis("proj", []string{"b.go"}, 3)
	assert.NilError(t, err)

	find := func(qn string) *ImpactedSymbol {
		for _, s := range impacted {
			if s.QualifiedName == qn {
				return &s
			}
		}
		return nil
	}

	// FuncB directly changed
	b := find("proj.pkg.FuncB")
	assert.Assert(t, b != nil, "FuncB should be impacted")
	assert.Equal(t, b.ChangeDepth, 0)
	assert.Assert(t, b.Changed)

	// FuncA calls FuncB → change in FuncB propagates to FuncA
	a := find("proj.pkg.FuncA")
	assert.Assert(t, a != nil, "FuncA should be impacted (calls FuncB)")
	assert.Equal(t, a.ChangeDepth, 1)
	assert.Assert(t, !a.Changed, "FuncA is not directly changed")
}

func TestImpactAnalysis_DepthLimit(t *testing.T) {
	store := openTestStore(t)
	defer store.Close()

	pid := seedProject(t, store, "proj")

	// Chain: A → B → C → D (depth 3)
	seedNode(t, store, pid, "proj.pkg.FuncA", "function", "FuncA", "a.go", 10)
	seedNode(t, store, pid, "proj.pkg.FuncB", "function", "FuncB", "b.go", 20)
	seedNode(t, store, pid, "proj.pkg.FuncC", "function", "FuncC", "c.go", 30)
	seedNode(t, store, pid, "proj.pkg.FuncD", "function", "FuncD", "d.go", 30)
	seedEdgeCall(t, store, pid, "proj.pkg.FuncA", "proj.pkg.FuncB")
	seedEdgeCall(t, store, pid, "proj.pkg.FuncB", "proj.pkg.FuncC")
	seedEdgeCall(t, store, pid, "proj.pkg.FuncC", "proj.pkg.FuncD")

	// Depth=1: only direct + first hop
	impacted, err := store.ImpactAnalysis("proj", []string{"a.go"}, 1)
	assert.NilError(t, err)

	find := func(qn string) *ImpactedSymbol {
		for _, s := range impacted {
			if s.QualifiedName == qn {
				return &s
			}
		}
		return nil
	}

	assert.Assert(t, find("proj.pkg.FuncA") != nil, "FuncA should be impacted")
	assert.Assert(t, find("proj.pkg.FuncB") != nil, "FuncB should be impacted (depth=1)")
	assert.Assert(t, find("proj.pkg.FuncC") == nil, "FuncC should NOT be impacted (depth limit)")
	assert.Assert(t, find("proj.pkg.FuncD") == nil, "FuncD should NOT be impacted (depth limit)")
}

func TestImpactAnalysis_MultipleChangedFiles(t *testing.T) {
	store := openTestStore(t)
	defer store.Close()

	pid := seedProject(t, store, "proj")

	// FuncA in a.go calls FuncB in b.go, FuncC in c.go is independent
	seedNode(t, store, pid, "proj.pkg.FuncA", "function", "FuncA", "a.go", 10)
	seedNode(t, store, pid, "proj.pkg.FuncB", "function", "FuncB", "b.go", 20)
	seedNode(t, store, pid, "proj.pkg.FuncC", "function", "FuncC", "c.go", 30)
	seedEdgeCall(t, store, pid, "proj.pkg.FuncA", "proj.pkg.FuncB")

	// Two changed files
	impacted, err := store.ImpactAnalysis("proj", []string{"a.go", "c.go"}, 3)
	assert.NilError(t, err)

	find := func(qn string) *ImpactedSymbol {
		for _, s := range impacted {
			if s.QualifiedName == qn {
				return &s
			}
		}
		return nil
	}

	a := find("proj.pkg.FuncA")
	assert.Assert(t, a != nil, "FuncA should be impacted")
	assert.Assert(t, a.Changed, "FuncA directly changed")
	assert.Equal(t, a.ChangeDepth, 0)

	c := find("proj.pkg.FuncC")
	assert.Assert(t, c != nil, "FuncC should be impacted")
	assert.Assert(t, c.Changed, "FuncC directly changed")
	assert.Equal(t, c.ChangeDepth, 0)

	b := find("proj.pkg.FuncB")
	assert.Assert(t, b != nil, "FuncB should be impacted (called by FuncA)")
	assert.Equal(t, b.ChangeDepth, 1)
	assert.Assert(t, !b.Changed, "FuncB not directly changed")
}

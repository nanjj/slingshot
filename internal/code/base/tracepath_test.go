package base

import (
	"testing"

	"gotest.tools/v3/assert"
)

// ─── TracePath ───────────────────────────────────────────────────────────────

func TestTracePath_CallsMode(t *testing.T) {
	store := openTestStore(t)
	defer store.Close()
	pid := seedProject(t, store, "proj")

	seedNode(t, store, pid, "proj.pkg.FuncA", "function", "FuncA", "a.go", 10)
	seedNode(t, store, pid, "proj.pkg.FuncB", "function", "FuncB", "b.go", 20)
	seedEdgeCall(t, store, pid, "proj.pkg.FuncA", "proj.pkg.FuncB")

	hops, err := store.TracePath(TracePathRequest{
		FunctionName: "proj.pkg.FuncA",
		Project:      "proj",
		Direction:    "outbound",
		Depth:        3,
		Mode:         "calls",
	})
	assert.NilError(t, err)
	assert.Assert(t, len(hops) >= 1, "should find at least FuncA→FuncB")

	foundAB := false
	for _, h := range hops {
		if h.SourceQN == "proj.pkg.FuncA" && h.TargetQN == "proj.pkg.FuncB" {
			foundAB = true
			assert.Equal(t, h.EdgeType, "CALLS")
			assert.Equal(t, h.Depth, 1)
		}
	}
	assert.Assert(t, foundAB, "FuncA→FuncB CALLS edge not found")
}

func TestTracePath_RiskLabels(t *testing.T) {
	store := openTestStore(t)
	defer store.Close()
	pid := seedProject(t, store, "proj")

	// Chain A→B→C→D→E (depth 4)
	names := []string{"FuncA", "FuncB", "FuncC", "FuncD", "FuncE"}
	for i, name := range names {
		qn := "proj.pkg." + name
		file := string(rune('a'+i)) + ".go"
		seedNode(t, store, pid, qn, "function", name, file, uint32(i*10))
		if i > 0 {
			prev := "proj.pkg." + names[i-1]
			seedEdgeCall(t, store, pid, prev, qn)
		}
	}

	hops, err := store.TracePath(TracePathRequest{
		FunctionName: "proj.pkg.FuncA",
		Project:      "proj",
		Direction:    "outbound",
		Depth:        10,
		Mode:         "calls",
		RiskLabels:   true,
	})
	assert.NilError(t, err)
	assert.Assert(t, len(hops) >= 4)

	riskMap := make(map[int]string)
	for _, h := range hops {
		riskMap[h.Depth] = h.Risk
	}

	// Depth ≤ 2 → HIGH
	assert.Equal(t, riskMap[1], "HIGH")
	assert.Equal(t, riskMap[2], "HIGH")
	// Depth 3-4 → MEDIUM
	assert.Equal(t, riskMap[3], "MEDIUM")
	assert.Equal(t, riskMap[4], "MEDIUM")
}

func TestTracePath_IncludeTests(t *testing.T) {
	store := openTestStore(t)
	defer store.Close()
	pid := seedProject(t, store, "proj")

	seedNode(t, store, pid, "proj.pkg.FuncA", "function", "FuncA", "a.go", 10)
	seedNode(t, store, pid, "proj.pkg.FuncB", "function", "FuncB", "a_test.go", 20)
	seedEdgeCall(t, store, pid, "proj.pkg.FuncA", "proj.pkg.FuncB")

	// Without test files
	hops, err := store.TracePath(TracePathRequest{
		FunctionName: "proj.pkg.FuncA",
		Project:      "proj",
		Direction:    "outbound",
		Depth:        3,
		Mode:         "calls",
		IncludeTests: false,
	})
	assert.NilError(t, err)
	// FuncB (in test file) should be filtered out
	for _, h := range hops {
		if h.TargetQN == "proj.pkg.FuncB" {
			t.Errorf("FuncB should be filtered out (test file), but found in hop")
		}
	}

	// With test files
	hopsWith, err := store.TracePath(TracePathRequest{
		FunctionName: "proj.pkg.FuncA",
		Project:      "proj",
		Direction:    "outbound",
		Depth:        3,
		Mode:         "calls",
		IncludeTests: true,
	})
	assert.NilError(t, err)
	foundB := false
	for _, h := range hopsWith {
		if h.TargetQN == "proj.pkg.FuncB" {
			foundB = true
		}
	}
	assert.Assert(t, foundB, "FuncB should be included when IncludeTests=true")
}

func TestTracePath_EmptyResults(t *testing.T) {
	store := openTestStore(t)
	defer store.Close()

	// No nodes seeded
	hops, err := store.TracePath(TracePathRequest{
		FunctionName: "nonexistent",
		Project:      "proj",
		Direction:    "outbound",
		Depth:        3,
	})
	assert.NilError(t, err)
	assert.Assert(t, len(hops) == 0, "expected empty hops for nonexistent function")
}

func TestTracePath_Direction(t *testing.T) {
	store := openTestStore(t)
	defer store.Close()
	pid := seedProject(t, store, "proj")

	seedNode(t, store, pid, "proj.pkg.FuncA", "function", "FuncA", "a.go", 10)
	seedNode(t, store, pid, "proj.pkg.FuncB", "function", "FuncB", "b.go", 20)
	seedEdgeCall(t, store, pid, "proj.pkg.FuncA", "proj.pkg.FuncB")

	// Inbound: FuncB is called by FuncA
	inbound, err := store.TracePath(TracePathRequest{
		FunctionName: "proj.pkg.FuncB",
		Project:      "proj",
		Direction:    "inbound",
		Depth:        3,
	})
	assert.NilError(t, err)
	foundInbound := false
	for _, h := range inbound {
		if h.SourceQN == "proj.pkg.FuncA" && h.TargetQN == "proj.pkg.FuncB" {
			foundInbound = true
		}
	}
	assert.Assert(t, foundInbound, "should find inbound: FuncA calls FuncB")

	// Outbound: FuncA calls FuncB
	outbound, err := store.TracePath(TracePathRequest{
		FunctionName: "proj.pkg.FuncA",
		Project:      "proj",
		Direction:    "outbound",
		Depth:        3,
	})
	assert.NilError(t, err)
	foundOutbound := false
	for _, h := range outbound {
		if h.SourceQN == "proj.pkg.FuncA" && h.TargetQN == "proj.pkg.FuncB" {
			foundOutbound = true
		}
	}
	assert.Assert(t, foundOutbound, "should find outbound: FuncA calls FuncB")

	// Both: from FuncB should find FuncA and FuncB
	both, err := store.TracePath(TracePathRequest{
		FunctionName: "proj.pkg.FuncB",
		Project:      "proj",
		Direction:    "both",
		Depth:        3,
	})
	assert.NilError(t, err)
	assert.Assert(t, len(both) >= 1, "both direction should find at least 1 hop")
}

// Package base — tests for CALLS edge receiver resolution and
// receiver-stripped query matching (Archimedes feedback, mail #489).
//
// Background: method calls like h.dispatchToServer() were stored with the raw
// receiver variable text as the edge target, while the definition node is
// package-qualified (pkg.dispatchToServer) — 61% of CALLS edges dangled.
// Fix: index-time resolution (self calls, local var types) plus query-time
// receiver-stripped fallback matching.
package base

import (
	"os"
	"path/filepath"
	"testing"

	"gotest.tools/v3/assert"
)

// indexProjectFixture writes src to dir/main.go and indexes it as a project.
func indexProjectFixture(t *testing.T, src string) *Store {
	t.Helper()
	dir := t.TempDir()
	err := os.WriteFile(filepath.Join(dir, "main.go"), []byte(src), 0644)
	assert.NilError(t, err)

	dbPath := filepath.Join(t.TempDir(), "test.db")
	store, err := OpenStore(dbPath)
	assert.NilError(t, err)
	t.Cleanup(func() { store.Close() })

	result, err := store.IndexProject(dir, "test-project", IndexModeFull)
	assert.NilError(t, err)
	assert.Equal(t, result.Errors, 0)
	return store
}

// callsOnly filters edges to CALLS type (GetReferences returns all edge kinds).
func callsOnly(refs []Edge) []Edge {
	var calls []Edge
	for _, r := range refs {
		if r.EdgeType == "CALLS" {
			calls = append(calls, r)
		}
	}
	return calls
}

// ─── resolveMethodCall unit tests ────────────────────────────────────────────

func TestResolveMethodCall_SelfReceiver(t *testing.T) {
	ctx := callCtx{
		pkg:           "hub",
		recvVar:       "h",
		importAliases: map[string]bool{"fmt": true},
		typeNames:     map[string]bool{"Hub": true},
		varTypes:      map[string]string{},
	}
	resolved, recv, method, isPkg := resolveMethodCall("h.dispatchToServer", ctx)
	assert.Equal(t, resolved, "hub.dispatchToServer")
	assert.Equal(t, recv, "h")
	assert.Equal(t, method, "dispatchToServer")
	assert.Assert(t, !isPkg)
}

func TestResolveMethodCall_LocalVarType(t *testing.T) {
	ctx := callCtx{
		pkg:           "hub",
		importAliases: map[string]bool{},
		typeNames:     map[string]bool{"Hub": true},
		varTypes:      map[string]string{"h": "Hub"},
	}
	resolved, _, _, _ := resolveMethodCall("h.dispatchToServer", ctx)
	assert.Equal(t, resolved, "hub.dispatchToServer")
}

func TestResolveMethodCall_ImportAliasStaysQualified(t *testing.T) {
	ctx := callCtx{
		pkg:           "hub",
		importAliases: map[string]bool{"fmt": true},
		typeNames:     map[string]bool{},
		varTypes:      map[string]string{},
	}
	resolved, recv, method, isPkg := resolveMethodCall("fmt.Println", ctx)
	assert.Equal(t, resolved, "fmt.Println") // package function — untouched
	assert.Equal(t, recv, "")
	assert.Equal(t, method, "")
	assert.Assert(t, isPkg, "import-alias call must be flagged as package function")
}

func TestResolveMethodCall_UnknownReceiverKeptRaw(t *testing.T) {
	ctx := callCtx{
		pkg:           "hub",
		importAliases: map[string]bool{},
		typeNames:     map[string]bool{"Hub": true},
		varTypes:      map[string]string{},
	}
	resolved, recv, method, _ := resolveMethodCall("x.dispatchToServer", ctx)
	assert.Equal(t, resolved, "x.dispatchToServer") // cannot resolve — keep raw
	assert.Equal(t, recv, "x")
	assert.Equal(t, method, "dispatchToServer")
}

func TestResolveMethodCall_ChainedCallKeptRaw(t *testing.T) {
	ctx := callCtx{
		pkg:           "hub",
		recvVar:       "h",
		importAliases: map[string]bool{},
		typeNames:     map[string]bool{"Hub": true},
		varTypes:      map[string]string{},
	}
	resolved, _, _, _ := resolveMethodCall("getClient().Close", ctx)
	assert.Equal(t, resolved, "getClient().Close")
}

func TestResolveMethodCall_PlainFunction(t *testing.T) {
	ctx := callCtx{pkg: "hub"}
	resolved, recv, method, _ := resolveMethodCall("run", ctx)
	assert.Equal(t, resolved, "run") // handled by resolveCallTarget
	assert.Equal(t, recv, "")
	assert.Equal(t, method, "")
}

// ─── Index-time resolution (integration) ─────────────────────────────────────

// TestIndexProject_SelfMethodCallResolves verifies h.method() inside a method
// body whose receiver is h resolves to pkg.method.
func TestIndexProject_SelfMethodCallResolves(t *testing.T) {
	store := indexProjectFixture(t, `package hub

type Hub struct{}

func (h *Hub) dispatchToServer() {}

func (h *Hub) Run() {
	h.dispatchToServer()
}
`)
	refs, err := store.GetReferences("hub.dispatchToServer", "test-project", "inbound", 1)
	assert.NilError(t, err)
	calls := callsOnly(refs)
	assert.Assert(t, len(calls) == 1,
		"expected the self-call edge to resolve to hub.dispatchToServer, got %d CALLS refs", len(calls))
	assert.Equal(t, calls[0].SourceQN, "hub.Run")
}

// TestIndexProject_ParamTypeCallResolves verifies h.method() where h is a
// function parameter of a locally defined type resolves to pkg.method.
func TestIndexProject_ParamTypeCallResolves(t *testing.T) {
	store := indexProjectFixture(t, `package hub

type Hub struct{}

func (h *Hub) dispatchToServer() {}

func runHub(h *Hub) {
	h.dispatchToServer()
}
`)
	refs, err := store.GetReferences("hub.dispatchToServer", "test-project", "inbound", 1)
	assert.NilError(t, err)
	calls := callsOnly(refs)
	assert.Assert(t, len(calls) == 1,
		"expected param-type call to resolve, got %d CALLS refs", len(calls))
	assert.Equal(t, calls[0].SourceQN, "hub.runHub")
}

// TestIndexProject_ShortVarCallResolves verifies h := &Hub{}; h.method()
// resolves to pkg.method.
func TestIndexProject_ShortVarCallResolves(t *testing.T) {
	store := indexProjectFixture(t, `package hub

type Hub struct{}

func (h *Hub) dispatchToServer() {}

func run() {
	h := &Hub{}
	h.dispatchToServer()
}
`)
	refs, err := store.GetReferences("hub.dispatchToServer", "test-project", "inbound", 1)
	assert.NilError(t, err)
	calls := callsOnly(refs)
	assert.Assert(t, len(calls) == 1,
		"expected short-var call to resolve, got %d CALLS refs", len(calls))
}

// TestIndexProject_PackageFunctionCallStaysQualified verifies fmt.Println is
// NOT rewritten into a local package method.
func TestIndexProject_PackageFunctionCallStaysQualified(t *testing.T) {
	store := indexProjectFixture(t, `package hub

import "fmt"

type Hub struct{}

func (h *Hub) Println() {}

func (h *Hub) Run() {
	fmt.Println("hello")
}
`)
	// The import-alias call must stay qualified as fmt.Println.
	refs, err := store.GetReferences("fmt.Println", "test-project", "inbound", 1)
	assert.NilError(t, err)
	calls := callsOnly(refs)
	assert.Assert(t, len(calls) == 1,
		"expected fmt.Println call edge, got %d CALLS refs", len(calls))
	assert.Equal(t, calls[0].SourceQN, "hub.Run")
	// And it must NOT be matched as the local method.
	refs2, err := store.GetReferences("hub.Println", "test-project", "inbound", 1)
	assert.NilError(t, err)
	for _, r := range callsOnly(refs2) {
		assert.Assert(t, r.SourceQN != "hub.Run",
			"fmt.Println must not resolve to hub.Println")
	}
}

// ─── Query-time receiver-stripped fallback ───────────────────────────────────

// TestFindReferences_ReceiverStrippedMatch verifies that an unresolvable
// method call (x.other — receiver declared elsewhere) is still found when
// querying the package-qualified definition, via the LIKE '%.method' fallback.
func TestFindReferences_ReceiverStrippedMatch(t *testing.T) {
	store := indexProjectFixture(t, `package hub

type Hub struct{}

func (h *Hub) other() {}

func run(x *External) {
	x.other()
}
`)
	// The definition node exists (kind method) → stripped matching active.
	refs, err := store.GetReferences("hub.other", "test-project", "inbound", 1)
	assert.NilError(t, err)
	calls := callsOnly(refs)
	assert.Assert(t, len(calls) == 1,
		"expected receiver-stripped match for x.other, got %d CALLS refs", len(calls))
	assert.Equal(t, calls[0].SourceQN, "hub.run")
	// Metadata carries the raw receiver so consumers can disambiguate.
	assert.Assert(t, len(calls[0].Metadata) > 0 && calls[0].Metadata != "{}",
		"edge should carry metadata, got %q", calls[0].Metadata)
}

// TestTracePath_ReceiverStrippedMatch verifies trace_path inbound finds the
// same unresolvable call through the fallback.
func TestTracePath_ReceiverStrippedMatch(t *testing.T) {
	store := indexProjectFixture(t, `package hub

type Hub struct{}

func (h *Hub) other() {}

func run(x *External) {
	x.other()
}
`)
	hops, err := store.TracePath(TracePathRequest{
		FunctionName: "hub.other",
		Project:      "test-project",
		Direction:    "inbound",
		Depth:        2,
	})
	assert.NilError(t, err)
	assert.Assert(t, len(hops) == 1,
		"expected trace_path inbound hop via stripped match, got %d", len(hops))
	assert.Equal(t, hops[0].SourceQN, "hub.run")
}

// TestGetReferences_NonMethodNoStripping verifies stripping is not applied to
// non-function/method nodes (e.g. variables), keeping queries precise.
func TestGetReferences_NonMethodNoStripping(t *testing.T) {
	store := indexProjectFixture(t, `package hub

var v = 1

type Hub struct{}

func run() {
	_ = v
}
`)
	// v is a variable node — stripped matching must not apply, so a query for
	// "x.v" (which would LIKE-match nothing anyway) stays exact.
	refs, err := store.GetReferences("x.v", "test-project", "inbound", 1)
	assert.NilError(t, err)
	assert.Assert(t, len(refs) == 0,
		"non-method QN must not use receiver-stripped matching, got %d refs", len(refs))
}

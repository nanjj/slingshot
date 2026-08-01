// Package mcp_test — open_project binding persistence across server restarts
// (Archimedes feedback, mail #489: after a panic/restart the binding was lost
// and project-less tool calls fell back to CWD resolution errors).
package mcp_test

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"gotest.tools/v3/assert"

	"github.com/nanjj/slingshot/internal/code/base"
	"github.com/nanjj/slingshot/internal/code/lsp"
	codemcp "github.com/nanjj/slingshot/internal/code/mcp"
)

// newServerForDB builds a fresh MCP server sharing the given database, with an
// unindexed workspace root (simulating a restart with no CWD binding).
func newServerForDB(t *testing.T, dbFile string) (*mcp.ClientSession, func()) {
	t.Helper()
	store, err := base.OpenStore(dbFile)
	assert.NilError(t, err)

	analyzer := lsp.NewAnalyzer()
	impl := &mcp.Implementation{Name: "code-test", Version: "0.0.1-test"}
	srv := mcp.NewServer(impl, nil)

	// Workspace root is a fresh empty dir — NOT the indexed project, so the
	// only way to get a binding is the persisted last_project.
	codeSrv := codemcp.NewServer(store, analyzer, &codemcp.Options{
		ProjectRoot: t.TempDir(),
		DBPath:      dbFile,
	})
	codeSrv.RegisterAll(srv)

	st, ct := mcp.NewInMemoryTransports()
	_, err = srv.Connect(context.Background(), st, nil)
	assert.NilError(t, err)
	client := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "0.0.1"}, nil)
	cs, err := client.Connect(context.Background(), ct, nil)
	assert.NilError(t, err)

	cleanup := func() {
		cs.Close()
		store.Close()
	}
	return cs, cleanup
}

// TestOpenProjectBinding_PersistsAcrossRestart verifies that after binding a
// project and restarting the server (new process-like server on the same DB),
// tool calls without a project argument still resolve to the bound project.
func TestOpenProjectBinding_PersistsAcrossRestart(t *testing.T) {
	dbFile := filepath.Join(t.TempDir(), "code.db")

	// ── First server: index a project and bind it via open_project ──
	cs1, cleanup1 := newServerForDB(t, dbFile)
	projectDir := mkProject(t)
	var idxRes struct {
		ProjectName string `json:"projectName"`
	}
	callToolUnmarshal(t, cs1, "index_repository", map[string]any{
		"repoPath": projectDir,
		"mode":     "fast",
	}, &idxRes)

	var openRes map[string]any
	callToolUnmarshal(t, cs1, "open_project", map[string]any{
		"project": idxRes.ProjectName,
	}, &openRes)
	assert.Equal(t, openRes["indexed"], true)
	cleanup1()

	// ── Second server on the same DB (simulated restart) ──
	cs2, cleanup2 := newServerForDB(t, dbFile)
	defer cleanup2()

	// No project argument — must resolve via persisted binding.
	result, err := cs2.CallTool(context.Background(), &mcp.CallToolParams{
		Name: "search_code",
		Arguments: map[string]any{
			"pattern": "Greeter",
		},
	})
	assert.NilError(t, err)
	assert.Assert(t, !result.IsError,
		"search_code without project must work after restart via persisted binding: %s", textContent(result))
	assert.Assert(t, len(result.Content) > 0)
}

// TestLastProject_StoreRoundTrip verifies the store-level persistence
// primitives survive close/reopen.
func TestLastProject_StoreRoundTrip(t *testing.T) {
	dbFile := filepath.Join(t.TempDir(), "state.db")
	store, err := base.OpenStore(dbFile)
	assert.NilError(t, err)

	projectDir := mkProject(t)
	_, err = store.IndexProject(projectDir, "state-project", base.IndexModeFull)
	assert.NilError(t, err)
	err = store.SaveLastProject("state-project")
	assert.NilError(t, err)
	store.Close()

	store2, err := base.OpenStore(dbFile)
	assert.NilError(t, err)
	defer store2.Close()
	assert.Equal(t, store2.LoadLastProject(), "state-project")
}

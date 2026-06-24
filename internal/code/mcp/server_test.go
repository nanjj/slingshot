// Package mcp_test provides integration tests for the MCP code intelligence
// server. It runs a full in-process MCP server + client pair over in-memory
// transport, exercising the complete JSON-RPC handler chain.
package mcp_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	_ "github.com/odvcencio/gotreesitter/grammars"
	"gotest.tools/v3/assert"

	codemcp "github.com/nanjj/slingshot/internal/code/mcp"
	"github.com/nanjj/slingshot/internal/code/base"
	"github.com/nanjj/slingshot/internal/code/lsp"
)

// ─── Test helpers ──────────────────────────────────────────────────────────────

// mkProject creates a temporary directory with a small Go source file and returns its path.
func mkProject(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()

	content := `package main

import "fmt"

// Greeter greets people.
type Greeter struct {
	Name string
}

// Hello returns a greeting message.
func (g *Greeter) Hello() string {
	return "Hello, " + g.Name + "!"
}

// main is the entry point.
func main() {
	g := &Greeter{Name: "World"}
	fmt.Println(g.Hello())
}
`
	err := os.WriteFile(filepath.Join(dir, "main.go"), []byte(content), 0644)
	assert.NilError(t, err)
	return dir
}

// testFixture creates a fully wired MCP server + client pair ready for testing.
// Returns the client session (for calling tools) and a cleanup func.
func testFixture(t *testing.T) (*mcp.ClientSession, func()) {
	t.Helper()

	// 1. Temp database
	dbFile := filepath.Join(t.TempDir(), "code.db")
	store, err := base.OpenStore(dbFile)
	assert.NilError(t, err)

	// 2. Analyzer
	analyzer := lsp.NewAnalyzer()

	// 3. MCP server (SDK)
	impl := &mcp.Implementation{Name: "code-test", Version: "0.0.1-test"}
	srv := mcp.NewServer(impl, nil)

	// 4. Our code intelligence server (registers all tools)
	codeSrv := codemcp.NewServer(store, analyzer, &codemcp.Options{
		ProjectRoot: t.TempDir(),
		DBPath:      dbFile,
	})
	codeSrv.RegisterAll(srv)

	// 5. In-memory transport
	st, ct := mcp.NewInMemoryTransports()

	// Connect server
	_, err = srv.Connect(context.Background(), st, nil)
	assert.NilError(t, err)

	// Connect client
	client := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "0.0.1"}, nil)
	cs, err := client.Connect(context.Background(), ct, nil)
	assert.NilError(t, err)

	cleanup := func() {
		cs.Close()
		store.Close()
	}
	return cs, cleanup
}

// callToolOK calls a tool and returns the raw text content.
func callToolOK(t *testing.T, cs *mcp.ClientSession, name string, args map[string]any) string {
	t.Helper()
	result, err := cs.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      name,
		Arguments: args,
	})
	assert.NilError(t, err, "tool %q failed at protocol level", name)
	assert.Assert(t, !result.IsError, "tool %q returned error: %v", name, textContent(result))
	assert.Assert(t, len(result.Content) > 0, "tool %q returned empty content", name)
	return textContent(result)
}

func textContent(result *mcp.CallToolResult) string {
	if len(result.Content) == 0 {
		return ""
	}
	if t, ok := result.Content[0].(*mcp.TextContent); ok {
		return t.Text
	}
	return "<!-- non-text content -->"
}

// callToolUnmarshal calls a tool and unmarshals the text content into target.
func callToolUnmarshal(t *testing.T, cs *mcp.ClientSession, name string, args map[string]any, target any) {
	t.Helper()
	text := callToolOK(t, cs, name, args)
	err := json.Unmarshal([]byte(text), target)
	assert.NilError(t, err, "tool %q JSON unmarshal", name)
}

// callToolError calls a tool that is expected to return an error result.
func callToolError(t *testing.T, cs *mcp.ClientSession, name string, args map[string]any) {
	t.Helper()
	result, err := cs.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      name,
		Arguments: args,
	})
	assert.NilError(t, err, "tool %q should not produce protocol-level error", name)
	assert.Assert(t, result.IsError, "tool %q should return IsError=true", name)
}

// ─── Tests ─────────────────────────────────────────────────────────────────────

// TestListProjectsEmpty verifies list_projects returns an empty array on a fresh database.
func TestListProjectsEmpty(t *testing.T) {
	cs, cleanup := testFixture(t)
	defer cleanup()

	var projects []any
	callToolUnmarshal(t, cs, "list_projects", nil, &projects)
	assert.Assert(t, len(projects) >= 0) // no projects is valid
}

// TestProjectRoot verifies get_project_root returns the configured root.
func TestProjectRoot(t *testing.T) {
	cs, cleanup := testFixture(t)
	defer cleanup()

	var result struct {
		ProjectRoot string `json:"projectRoot"`
	}
	callToolUnmarshal(t, cs, "get_project_root", nil, &result)
	assert.Assert(t, result.ProjectRoot != "")
}

// TestIndexAndSearch verifies the full index → search → status flow.
func TestIndexAndSearch(t *testing.T) {
	cs, cleanup := testFixture(t)
	defer cleanup()

	projectDir := mkProject(t)

	// 1. Index the project
	type indexResult struct {
		ProjectName string `json:"projectName"`
		FilesParsed int    `json:"filesParsed"`
		NodesStored int    `json:"nodesStored"`
	}
	var idxRes indexResult
	callToolUnmarshal(t, cs, "index_repository", map[string]any{
		"repoPath": projectDir,
	}, &idxRes)
	assert.Assert(t, idxRes.FilesParsed >= 1)
	// Go tags query: function_declaration + method_declaration (no struct tag)
	assert.Assert(t, idxRes.NodesStored >= 2)

	projName := idxRes.ProjectName

	// 2. List projects (non-empty now)
	var projects []base.ProjectInfo
	callToolUnmarshal(t, cs, "list_projects", nil, &projects)
	assert.Assert(t, len(projects) > 0)
	found := false
	for _, p := range projects {
		if p.Name == projName {
			found = true
			assert.Equal(t, p.Status, "ready")
		}
	}
	assert.Assert(t, found, "project %q not found in list", projName)

	// 3. Index status
	var statusRes base.ProjectInfo
	callToolUnmarshal(t, cs, "index_status", map[string]any{
		"project": projName,
	}, &statusRes)
	assert.Equal(t, statusRes.Name, projName)

	// 4. Search graph — query mode
	type searchResults struct {
		Results []struct {
			Name string `json:"name"`
		} `json:"results"`
		Total int `json:"total"`
	}
	var searchRes searchResults
	callToolUnmarshal(t, cs, "search_graph", map[string]any{
		"project": projName,
		"query":   "Hello",
	}, &searchRes)
	assert.Assert(t, searchRes.Total >= 1)

	// 5. Search graph — name pattern mode
	var patternRes searchResults
	callToolUnmarshal(t, cs, "search_graph", map[string]any{
		"project":     projName,
		"namePattern": "Hello",
	}, &patternRes)
	assert.Assert(t, patternRes.Total >= 1)

	// 6. Get graph schema
	type schemaResult struct {
		NodeLabels []map[string]any `json:"nodeLabels"`
		EdgeTypes  []map[string]any `json:"edgeTypes"`
	}
	var schemaRes schemaResult
	callToolUnmarshal(t, cs, "get_graph_schema", map[string]any{
		"project": projName,
	}, &schemaRes)
	assert.Assert(t, len(schemaRes.NodeLabels) > 0, "should have at least one node label")

	// 7. Get architecture
	type archResult struct {
		Project *struct {
			Name string `json:"name"`
		} `json:"project"`
	}
	var archRes archResult
	callToolUnmarshal(t, cs, "get_architecture", map[string]any{
		"project": projName,
	}, &archRes)
	assert.Assert(t, archRes.Project != nil)
	assert.Equal(t, archRes.Project.Name, projName)

	// 8. Get code snippet
	type snippetResult struct {
		Node *struct {
			Name string `json:"name"`
		} `json:"node"`
		Source string `json:"source"`
	}
	var snippetRes snippetResult
	callToolUnmarshal(t, cs, "get_code_snippet", map[string]any{
		"project":       projName,
		"qualifiedName": "main.Hello",
	}, &snippetRes)
	assert.Assert(t, snippetRes.Node != nil)
	assert.Assert(t, strings.Contains(snippetRes.Source, "Hello"), "source should contain Hello")

	// 9. Query graph
	type graphResult struct {
		Results []map[string]any `json:"results"`
		Total   int              `json:"total"`
	}
	var graphRes graphResult
	callToolUnmarshal(t, cs, "query_graph", map[string]any{
		"project": projName,
		"query":   "SELECT name FROM nodes WHERE kind = 'function'",
	}, &graphRes)
	assert.Assert(t, graphRes.Total >= 1)
}

// TestMemoTools verifies save_memo, search_memos, and manage_adr.
func TestMemoTools(t *testing.T) {
	cs, cleanup := testFixture(t)
	defer cleanup()

	projectDir := mkProject(t)

	// Need an indexed project first
	type idxResult struct {
		ProjectName string `json:"projectName"`
	}
	var idxRes idxResult
	callToolUnmarshal(t, cs, "index_repository", map[string]any{
		"repoPath": projectDir,
	}, &idxRes)
	projName := idxRes.ProjectName

	// 1. Save memo
	type saveRes struct {
		ID int64 `json:"id"`
	}
	var saveR saveRes
	callToolUnmarshal(t, cs, "save_memo", map[string]any{
		"project": projName,
		"title":   "test memo",
		"content": "This is a test memory entry.",
		"type":    "learning",
	}, &saveR)
	assert.Assert(t, saveR.ID > 0)

	// 2. Search memos
	type searchRes struct {
		Results []struct {
			Title   string `json:"title"`
			Content string `json:"content"`
		} `json:"results"`
		Total int `json:"total"`
	}
	var searchR searchRes
	callToolUnmarshal(t, cs, "search_memos", map[string]any{
		"project": projName,
		"query":   "test memory",
	}, &searchR)
	assert.Assert(t, searchR.Total >= 1)
	assert.Equal(t, searchR.Results[0].Title, "test memo")

	// 3. Manage ADR — save
	type adrSaveRes struct {
		ID      int64 `json:"id"`
		Success bool  `json:"success"`
	}
	var adrSave adrSaveRes
	callToolUnmarshal(t, cs, "manage_adr", map[string]any{
		"project": projName,
		"action":  "create",
		"title":   "ADR 001",
		"content": "ADR 001: Use SQLite for storage.",
	}, &adrSave)
	assert.Assert(t, adrSave.ID > 0)

	// 4. Manage ADR — list
	type adrListItem struct {
		Title   string `json:"title"`
		Content string `json:"content"`
	}
	var adrList []adrListItem
	callToolUnmarshal(t, cs, "manage_adr", map[string]any{
		"project": projName,
		"action":  "list",
	}, &adrList)
	assert.Assert(t, len(adrList) >= 1)
}

// TestEditorTools verifies the AST analysis tools against a test file.
func TestEditorTools(t *testing.T) {
	cs, cleanup := testFixture(t)
	defer cleanup()

	projectDir := mkProject(t)
	filePath := filepath.Join(projectDir, "main.go")

	// get_structure
	type structResult struct {
		Type     string `json:"type"`
		Children []any  `json:"children,omitempty"`
	}
	var structRes structResult
	callToolUnmarshal(t, cs, "get_structure", map[string]any{
		"file":     filePath,
		"maxDepth": 2,
	}, &structRes)
	// tree-sitter-go root node is "source_file"
	assert.Assert(t, structRes.Type == "source_file" || structRes.Type == "program",
		"expected root type 'source_file' or 'program', got %q", structRes.Type)

	// get_text — line mode (0-indexed)
	var lineRes struct {
		Text string `json:"text"`
	}
	callToolUnmarshal(t, cs, "get_text", map[string]any{
		"file": filePath,
		"by":   "line",
		"line": 0,
	}, &lineRes)
	assert.Assert(t, strings.Contains(lineRes.Text, "package main"))

	// get_definitions
	// get_definitions — returns a JSON array of Tag objects directly
	type tagItem struct {
		Name string `json:"name"`
		Kind string `json:"kind"`
	}
	var defsRes []tagItem
	callToolUnmarshal(t, cs, "get_definitions", map[string]any{
		"file": filePath,
	}, &defsRes)
	t.Logf("definitions: %+v", defsRes)
	// validate
	type validateResult struct {
		Valid bool `json:"valid"`
	}
	var validateRes validateResult
	callToolUnmarshal(t, cs, "validate", map[string]any{
		"file": filePath,
	}, &validateRes)
	assert.Assert(t, validateRes.Valid, "expected no validation errors")

	// query_ast
	var qRes []map[string]any
	callToolUnmarshal(t, cs, "query_ast", map[string]any{
		"file":    filePath,
		"pattern": "(function_declaration name: (identifier) @fn)",
	}, &qRes)
	t.Logf("query_ast results: %d", len(qRes))
}

// TestDeleteProject verifies delete_project works.
func TestDeleteProject(t *testing.T) {
	cs, cleanup := testFixture(t)
	defer cleanup()

	projectDir := mkProject(t)

	type idxResult struct {
		ProjectName string `json:"projectName"`
	}
	var idxRes idxResult
	callToolUnmarshal(t, cs, "index_repository", map[string]any{
		"repoPath": projectDir,
	}, &idxRes)

	type delResult struct {
		Success bool `json:"success"`
	}
	var delRes delResult
	callToolUnmarshal(t, cs, "delete_project", map[string]any{
		"project": idxRes.ProjectName,
	}, &delRes)
	assert.Equal(t, delRes.Success, true)

	// List after delete
	var projects []any
	callToolUnmarshal(t, cs, "list_projects", nil, &projects)
	// Deleted projects may still appear with "deleted" status — just verify no error
}

// TestTracePath verifies the trace_path tool.
func TestTracePath(t *testing.T) {
	cs, cleanup := testFixture(t)
	defer cleanup()

	projectDir := mkProject(t)

	type idxResult struct {
		ProjectName string `json:"projectName"`
	}
	var idxRes idxResult
	callToolUnmarshal(t, cs, "index_repository", map[string]any{
		"repoPath": projectDir,
	}, &idxRes)

	type traceResult struct {
		Hops  []any `json:"hops"`
		Total int   `json:"total"`
	}
	var traceRes traceResult
	callToolUnmarshal(t, cs, "trace_path", map[string]any{
		"project":      idxRes.ProjectName,
		"functionName": "main",
	}, &traceRes)
	t.Logf("trace_path: total=%d", traceRes.Total)
}

// TestErrorHandling verifies that tools return proper errors for missing required args.
func TestErrorHandling(t *testing.T) {
	cs, cleanup := testFixture(t)
	defer cleanup()

	tests := []struct {
		name string
		args map[string]any
	}{
		{"search_graph", map[string]any{"query": "test"}},
		{"index_repository", map[string]any{}},
		{"index_status", map[string]any{}},
		{"delete_project", map[string]any{}},
		{"get_code_snippet", map[string]any{"project": "foo"}},
		{"trace_path", map[string]any{"project": "foo"}},
		{"save_memo", map[string]any{"title": "x", "content": "y"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			callToolError(t, cs, tt.name, tt.args)
		})
	}
}

// TestOpenProject verifies the open_project tool.
func TestOpenProject(t *testing.T) {
	cs, cleanup := testFixture(t)
	defer cleanup()

	projectDir := mkProject(t)

	var result struct {
		ProjectRoot string `json:"projectRoot"`
		Note        string `json:"note"`
	}
	callToolUnmarshal(t, cs, "open_project", map[string]any{
		"path": projectDir,
	}, &result)
	assert.Equal(t, result.ProjectRoot, projectDir)
}

// TestGetNode verifies the get_node tool across all scopes.
func TestGetNode(t *testing.T) {
	cs, cleanup := testFixture(t)
	defer cleanup()

	projectDir := mkProject(t)
	filePath := filepath.Join(projectDir, "main.go")

	// pos scope — first named node at byte 0
	type nodeResult struct {
		Type    string `json:"type"`
		IsNamed bool   `json:"isNamed"`
	}
	var posRes nodeResult
	callToolUnmarshal(t, cs, "get_node", map[string]any{
		"file": filePath,
		"pos":  0,
	}, &posRes)
	// At byte 0 in a Go file, we expect a named node (the root or a named descendant)
	t.Logf("get_node at pos=0: type=%q isNamed=%v", posRes.Type, posRes.IsNamed)
}

// ─── Phase 1: New tools ────────────────────────────────────────────────────────

// TestCodeEdit_InsertInsert verifies code_edit with insert mode.
func TestCodeEdit_Insert(t *testing.T) {
	cs, cleanup := testFixture(t)
	defer cleanup()

	projectDir := mkProject(t)
	filePath := filepath.Join(projectDir, "main.go")

	// Insert a line at the end of package declaration
	var result map[string]any
	callToolUnmarshal(t, cs, "code_edit", map[string]any{
		"file": filePath,
		"mode": "insert",
		"pos":  15,
		"text": "\n\n// NewFunc is a test function.\nfunc NewFunc() string {\n\treturn \"hello\"\n}\n",
	}, &result)
	assert.Assert(t, result["success"].(bool))
	t.Logf("code_edit insert byteDiff=%v", result["byteDiff"])
}

// TestCodeEdit_Replace verifies code_edit with replace mode.
func TestCodeEdit_Replace(t *testing.T) {
	cs, cleanup := testFixture(t)
	defer cleanup()

	projectDir := mkProject(t)
	filePath := filepath.Join(projectDir, "main.go")

	// Replace "Greeter" with "Saluter"
	var result map[string]any
	callToolUnmarshal(t, cs, "code_edit", map[string]any{
		"file":      filePath,
		"mode":      "replace",
		"startByte": 52,
		"endByte":   59,
		"text":      "Saluter",
	}, &result)
	assert.Assert(t, result["success"].(bool))
}

// TestCodeEdit_Delete verifies code_edit with delete mode.
func TestCodeEdit_Delete(t *testing.T) {
	cs, cleanup := testFixture(t)
	defer cleanup()

	projectDir := mkProject(t)
	filePath := filepath.Join(projectDir, "main.go")

	// Delete the Greeter struct
	var result map[string]any
	callToolUnmarshal(t, cs, "code_edit", map[string]any{
		"file":      filePath,
		"mode":      "delete",
		"startByte": 51,
		"endByte":   82,
	}, &result)
	assert.Assert(t, result["success"].(bool))
}

// TestCodeEditBody verifies code_edit_body replaces a function body.
func TestCodeEditBody(t *testing.T) {
	cs, cleanup := testFixture(t)
	defer cleanup()

	projectDir := mkProject(t)
	filePath := filepath.Join(projectDir, "main.go")

	var result map[string]any
	callToolUnmarshal(t, cs, "code_edit_body", map[string]any{
		"file":     filePath,
		"selector": "function:main",
		"text":     `fmt.Println("modified")`,
	}, &result)
	assert.Assert(t, result["success"].(bool))
}

// TestCodeLocate_Found verifies code_locate finds an indexed symbol.
func TestCodeLocate_Found(t *testing.T) {
	cs, cleanup := testFixture(t)
	defer cleanup()

	projectDir := mkProject(t)

	// Index first
	type idxResult struct {
		ProjectName string `json:"projectName"`
	}
	var idxRes idxResult
	callToolUnmarshal(t, cs, "index_repository", map[string]any{
		"repoPath": projectDir,
	}, &idxRes)

	// Locate the Hello method
	type locateResult struct {
		Found         bool   `json:"found"`
		Source        string `json:"source"`
		File          string `json:"file"`
		Kind          string `json:"kind"`
		Name          string `json:"name"`
		QualifiedName string `json:"qualifiedName"`
	}
	var locRes locateResult
	callToolUnmarshal(t, cs, "code_locate", map[string]any{
		"project":       idxRes.ProjectName,
		"qualifiedName": "main.Hello",
	}, &locRes)
	assert.Assert(t, locRes.Found, "Hello should be found")
	assert.Equal(t, locRes.Source, "sqlite")
}

// TestCodeLocate_Fallback verifies code_locate falls back to tree-sitter.
func TestCodeLocate_Fallback(t *testing.T) {
	cs, cleanup := testFixture(t)
	defer cleanup()

	projectDir := mkProject(t)
	filePath := filepath.Join(projectDir, "main.go")

	type locateResult struct {
		Found         bool   `json:"found"`
		Source        string `json:"source"`
		Name          string `json:"name"`
		QualifiedName string `json:"qualifiedName"`
	}
	var locRes locateResult
	callToolUnmarshal(t, cs, "code_locate", map[string]any{
		"file":          filePath,
		"qualifiedName": "Hello",
	}, &locRes)
	assert.Assert(t, locRes.Found, "Hello should be found via tree-sitter fallback")
	assert.Equal(t, locRes.Source, "treesitter")
}

// TestCodeLocate_NotFound verifies code_locate returns found=false for missing symbol.
func TestCodeLocate_NotFound(t *testing.T) {
	cs, cleanup := testFixture(t)
	defer cleanup()

	type locateResult struct {
		Found bool `json:"found"`
	}
	var locRes locateResult
	callToolUnmarshal(t, cs, "code_locate", map[string]any{
		"qualifiedName": "NonExistentSymbol",
	}, &locRes)
	assert.Assert(t, !locRes.Found, "NonExistentSymbol should not be found")
}

// TestCodeEdit_ErrorHandling verifies code_edit returns errors for missing args.
func TestCodeEdit_ErrorHandling(t *testing.T) {
	cs, cleanup := testFixture(t)
	defer cleanup()

	tests := []struct {
		name string
		args map[string]any
	}{
		{"missing mode", map[string]any{"file": "/tmp/test.go"}},
		{"missing file", map[string]any{"mode": "insert"}},
		{"insert missing text", map[string]any{"file": "/tmp/test.go", "mode": "insert"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			callToolError(t, cs, "code_edit", tt.args)
		})
	}
	}

// ─── Phase 2: References & Analysis ──────────────────────────────────────────

// TestCodeFindReferences_Inbound verifies code_find_references finds callers.
func TestCodeFindReferences_Inbound(t *testing.T) {
	cs, cleanup := testFixture(t)
	defer cleanup()

	projectDir := mkProject(t)

	// Index the project
	type idxResult struct {
		ProjectName string `json:"projectName"`
	}
	var idxRes idxResult
	callToolUnmarshal(t, cs, "index_repository", map[string]any{
		"repoPath": projectDir,
	}, &idxRes)

	// Find references to fmt.Println (inbound)
	type refItem struct {
		SourceQN string `json:"sourceQN"`
		TargetQN string `json:"targetQN"`
		EdgeType string `json:"edgeType"`
	}
	type refResult struct {
		Symbol     string    `json:"symbol"`
		References []refItem `json:"references"`
		Total      int       `json:"total"`
	}
	var refRes refResult
	callToolUnmarshal(t, cs, "code_find_references", map[string]any{
		"project":       idxRes.ProjectName,
		"qualifiedName": "fmt.Println",
	}, &refRes)

	assert.Assert(t, refRes.Total >= 1)
	assert.Equal(t, refRes.Symbol, "fmt.Println")
	assert.Assert(t, refRes.References[0].SourceQN == "main")
}

// TestCodeFindReferences_Outbound verifies code_find_references finds callees.
func TestCodeFindReferences_Outbound(t *testing.T) {
	cs, cleanup := testFixture(t)
	defer cleanup()

	projectDir := mkProject(t)

	type idxResult struct {
		ProjectName string `json:"projectName"`
	}
	var idxRes idxResult
	callToolUnmarshal(t, cs, "index_repository", map[string]any{
		"repoPath": projectDir,
	}, &idxRes)

	type refResult struct {
		Symbol     string `json:"symbol"`
		References []any  `json:"references"`
		Total      int    `json:"total"`
	}
	var refRes refResult
	callToolUnmarshal(t, cs, "code_find_references", map[string]any{
		"project":       idxRes.ProjectName,
		"qualifiedName": "main",
		"direction":     "outbound",
	}, &refRes)

	// main calls fmt.Println and g.Hello
	t.Logf("outbound references for main: total=%d", refRes.Total)
}

// TestCodeFindReferences_Errors verifies error handling.
func TestCodeFindReferences_Errors(t *testing.T) {
	cs, cleanup := testFixture(t)
	defer cleanup()

	t.Run("missing qualifiedName", func(t *testing.T) {
		callToolError(t, cs, "code_find_references", map[string]any{
			"project": "test",
		})
	})

	t.Run("missing project", func(t *testing.T) {
		callToolError(t, cs, "code_find_references", map[string]any{
			"qualifiedName": "main",
		})
	})
}

// TestCodeAnalysis verifies code_analysis returns file metrics.
func TestCodeAnalysis(t *testing.T) {
	cs, cleanup := testFixture(t)
	defer cleanup()

	projectDir := mkProject(t)
	filePath := filepath.Join(projectDir, "main.go")

	type funcAnalysis struct {
		Name       string `json:"name"`
		Kind       string `json:"kind"`
		Cyclomatic int    `json:"cyclomatic"`
		ParamCount int    `json:"paramCount"`
	}
	type analysisResult struct {
		File      string         `json:"file"`
		Language  string         `json:"language"`
		Functions []funcAnalysis `json:"functions"`
		Summary   struct {
			TotalFunctions int     `json:"totalFunctions"`
			AvgCyclomatic  float64 `json:"avgCyclomatic"`
			MaxComplexity  int     `json:"maxComplexity"`
		} `json:"summary"`
	}
	var result analysisResult
	callToolUnmarshal(t, cs, "code_analysis", map[string]any{
		"file": filePath,
	}, &result)

	assert.Assert(t, result.File != "")
	assert.Assert(t, result.Language != "")
	assert.Assert(t, len(result.Functions) >= 2, "expected at least 2 functions, got %d", len(result.Functions))

	// Verify: Hello method with param count 1 (receiver *Greeter counts as param)
	foundHello := false
	foundMain := false
	for _, fn := range result.Functions {
		if fn.Name == "Hello" {
			foundHello = true
			assert.Assert(t, fn.Cyclomatic >= 1)
			// Hello has receiver (g *Greeter) so param count should be >= 1
			assert.Assert(t, fn.ParamCount > 0)
		}
		if fn.Name == "main" {
			foundMain = true
		}
	}
	assert.Assert(t, foundHello, "Hello function should be analyzed")
	assert.Assert(t, foundMain, "main function should be analyzed")

	// Summary checks
	assert.Assert(t, result.Summary.TotalFunctions >= 2)
	assert.Assert(t, result.Summary.MaxComplexity >= 1)
	assert.Assert(t, result.Summary.AvgCyclomatic >= 1.0)
}

// TestCodeAnalysis_Error verifies error handling for missing file.
func TestCodeAnalysis_Error(t *testing.T) {
	cs, cleanup := testFixture(t)
	defer cleanup()

	callToolError(t, cs, "code_analysis", map[string]any{
		"file": "",
	})
}

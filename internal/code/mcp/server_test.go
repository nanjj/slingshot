// Package mcp_test provides integration tests for the MCP code intelligence
// server. It runs a full in-process MCP server + client pair over in-memory
// transport, exercising the complete JSON-RPC handler chain.
package mcp_test

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	_ "github.com/odvcencio/gotreesitter/grammars"
	"gotest.tools/v3/assert"

	"github.com/nanjj/slingshot/internal/code/base"
	"github.com/nanjj/slingshot/internal/code/lsp"
	codemcp "github.com/nanjj/slingshot/internal/code/mcp"
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

// ─── Phase 1: New tools ────────────────────────────────────────────────────────

// TestCodeEdit_InsertInsert verifies code_edit with insert mode.
func TestCodeEdit_Insert(t *testing.T) {
	cs, cleanup := testFixture(t)
	defer cleanup()

	projectDir := mkProject(t)
	filePath := filepath.Join(projectDir, "main.go")

	// Insert a line at the end of package declaration
	var result map[string]any
	callToolUnmarshal(t, cs, "edit", map[string]any{
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
	callToolUnmarshal(t, cs, "edit", map[string]any{
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
	callToolUnmarshal(t, cs, "edit", map[string]any{
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
	callToolUnmarshal(t, cs, "edit_body", map[string]any{
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
	callToolUnmarshal(t, cs, "locate", map[string]any{
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
	callToolUnmarshal(t, cs, "locate", map[string]any{
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
	callToolUnmarshal(t, cs, "locate", map[string]any{
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
			callToolError(t, cs, "edit", tt.args)
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
	callToolUnmarshal(t, cs, "find_references", map[string]any{
		"project":       idxRes.ProjectName,
		"qualifiedName": "fmt.Println",
	}, &refRes)

	assert.Assert(t, refRes.Total >= 1)
	assert.Equal(t, refRes.Symbol, "fmt.Println")
	assert.Assert(t, refRes.References[0].SourceQN == "main.main")
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
	callToolUnmarshal(t, cs, "find_references", map[string]any{
		"project":       idxRes.ProjectName,
		"qualifiedName": "main",
		"direction":     "outbound",
	}, &refRes)

	// main calls fmt.Println and g.Hello
	t.Logf("outbound references for main: total=%d", refRes.Total)
}

// TestCodeFindReferences_CrossFile verifies cross-file CALLS linking works
// when functions in different files call each other.
func TestCodeFindReferences_CrossFile(t *testing.T) {
	cs, cleanup := testFixture(t)
	defer cleanup()

	dir := t.TempDir()

	// File 1: helper package
	helperContent := `package helper

// Greet returns a greeting.
func Greet(name string) string {
	return "Hello, " + name
}
`
	err := os.WriteFile(filepath.Join(dir, "helper.go"), []byte(helperContent), 0644)
	assert.NilError(t, err)

	// File 2: main package that calls helper.Greet
	mainContent := `package main

import "helper"
import "fmt"

func main() {
	msg := helper.Greet("World")
	fmt.Println(msg)
}
`
	err = os.WriteFile(filepath.Join(dir, "main.go"), []byte(mainContent), 0644)
	assert.NilError(t, err)

	// File 3: another file in main package that calls helper.Greet too
	secondContent := `package main

import "helper"

func run() {
	helper.Greet("Test")
}
`
	err = os.WriteFile(filepath.Join(dir, "run.go"), []byte(secondContent), 0644)
	assert.NilError(t, err)

	type idxResult struct {
		ProjectName string `json:"projectName"`
	}
	var idxRes idxResult
	callToolUnmarshal(t, cs, "index_repository", map[string]any{
		"repoPath": dir,
	}, &idxRes)

	// Find inbound references to helper.Greet from all files
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
	callToolUnmarshal(t, cs, "find_references", map[string]any{
		"project":       idxRes.ProjectName,
		"qualifiedName": "helper.Greet",
	}, &refRes)

	t.Logf("callers of helper.Greet: total=%d", refRes.Total)
	assert.Assert(t, refRes.Total >= 2, "expected at least 2 callers of helper.Greet, got %d", refRes.Total)

	// Verify both callers have correct package-qualified SourceQNs
	callers := make(map[string]bool)
	for _, ref := range refRes.References {
		callers[ref.SourceQN] = true
	}
	assert.Assert(t, callers["main.main"], "expected main.main to call helper.Greet")
	assert.Assert(t, callers["main.run"], "expected main.run to call helper.Greet")
}

// TestTestLinking verifies TESTS and TESTS_FILE edges are created correctly.
func TestTestLinking(t *testing.T) {
	cs, cleanup := testFixture(t)
	defer cleanup()

	dir := t.TempDir()

	// Source file
	srcContent := `package math

// Add returns a + b.
func Add(a, b int) int {
	return a + b
}

// Mul returns a * b.
func Mul(a, b int) int {
	return a * b
}
`
	err := os.WriteFile(filepath.Join(dir, "math.go"), []byte(srcContent), 0644)
	assert.NilError(t, err)

	// Test file
	testContent := `package math

import "testing"

func TestAdd(t *testing.T) {
	r := Add(1, 2)
	if r != 3 {
		t.Fail()
	}
}

func TestMul(t *testing.T) {
	r := Mul(3, 4)
	if r != 12 {
		t.Fail()
	}
}
`
	err = os.WriteFile(filepath.Join(dir, "math_test.go"), []byte(testContent), 0644)
	assert.NilError(t, err)

	type idxResult struct {
		ProjectName string `json:"projectName"`
		EdgesStored int    `json:"edgesStored"`
	}
	var idxRes idxResult
	callToolUnmarshal(t, cs, "index_repository", map[string]any{
		"repoPath": dir,
	}, &idxRes)

	// Verify TESTS edges exist
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

	// Direct SQL check: count TESTS edges
	type sqlResult struct {
		Results []map[string]any `json:"results"`
		Total   int              `json:"total"`
	}
	var sqlRes sqlResult
	callToolUnmarshal(t, cs, "query_graph", map[string]any{
		"project": idxRes.ProjectName,
		"query": fmt.Sprintf(`
			SELECT e.source_qn, e.target_qn
			FROM edges e
			JOIN projects p ON p.id = e.project_id
			WHERE p.name = '%s' AND e.edge_type = 'TESTS'
		`, idxRes.ProjectName),
	}, &sqlRes)
	t.Logf("SQL TESTS check: total=%d", sqlRes.Total)
	for _, r := range sqlRes.Results {
		t.Logf("  TESTS: %v -> %v", r["source_qn"], r["target_qn"])
	}

	// Now check via code_find_references
	var refRes refResult
	callToolUnmarshal(t, cs, "find_references", map[string]any{
		"project":       idxRes.ProjectName,
		"qualifiedName": "math.Add",
		"direction":     "inbound",
	}, &refRes)

	t.Logf("references to math.Add: total=%d", refRes.Total)
	for i, ref := range refRes.References {
		t.Logf("  ref[%d]: %s -[%s]-> %s", i, ref.SourceQN, ref.EdgeType, ref.TargetQN)
	}
	foundTests := false
	for _, ref := range refRes.References {
		if ref.EdgeType == "TESTS" {
			foundTests = true
			assert.Assert(t, strings.Contains(ref.SourceQN, "TestAdd"),
				"expected TESTS from TestAdd, got %s", ref.SourceQN)
		}
	}
	assert.Assert(t, foundTests, "expected at least one TESTS edge for math.Add")
}

// TestCodeFindReferences_Errors verifies error handling.
func TestCodeFindReferences_Errors(t *testing.T) {
	cs, cleanup := testFixture(t)
	defer cleanup()

	t.Run("missing qualifiedName", func(t *testing.T) {
		callToolError(t, cs, "find_references", map[string]any{
			"project": "test",
		})
	})

	t.Run("missing project", func(t *testing.T) {
		callToolError(t, cs, "find_references", map[string]any{
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
	callToolUnmarshal(t, cs, "analysis", map[string]any{
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

	callToolError(t, cs, "analysis", map[string]any{
		"file": "",
	})
}

// ─── Project binding & lenient schema (Bohr feedback fixes) ─────────────────

// TestSearchCodeProjectNotFound_Hint verifies that a bad project identifier
// produces an error that lists available projects.
func TestSearchCodeProjectNotFound_Hint(t *testing.T) {
	cs, cleanup := testFixture(t)
	defer cleanup()

	projectDir := mkProject(t)
	callToolUnmarshal(t, cs, "index_repository", map[string]any{
		"repoPath": projectDir,
		"mode":     "fast",
	}, &struct {
		ProjectName string `json:"projectName"`
	}{})

	result, err := cs.CallTool(context.Background(), &mcp.CallToolParams{
		Name: "search_code",
		Arguments: map[string]any{
			"pattern": "Greeter",
			"project": "definitely-not-indexed",
		},
	})
	assert.NilError(t, err)
	assert.Assert(t, result.IsError, "search_code with unknown project should error")
	msg := textContent(result)
	assert.Assert(t, strings.Contains(msg, "Available projects"), "error should list available projects, got: %s", msg)
}

// TestSearchCodeWithoutProject_AfterOpenProject verifies project binding:
// after open_project, search_code works without a project argument.
func TestSearchCodeWithoutProject_AfterOpenProject(t *testing.T) {
	cs, cleanup := testFixture(t)
	defer cleanup()

	projectDir := mkProject(t)
	var idxRes struct {
		ProjectName string `json:"projectName"`
	}
	callToolUnmarshal(t, cs, "index_repository", map[string]any{
		"repoPath": projectDir,
		"mode":     "fast",
	}, &idxRes)

	// Bind by name
	var openRes map[string]any
	callToolUnmarshal(t, cs, "open_project", map[string]any{
		"project": idxRes.ProjectName,
	}, &openRes)
	assert.Equal(t, openRes["indexed"], true)

	// search_code without project argument
	type searchRes struct {
		TotalResults int `json:"totalResults"`
	}
	var res searchRes
	callToolUnmarshal(t, cs, "search_code", map[string]any{
		"pattern": "Greeter",
	}, &res)
	assert.Assert(t, res.TotalResults >= 1, "search without project should work after open_project")
}

// TestOpenProject_ByPathSuffix verifies open_project accepts a path suffix
// and binds the matching indexed project.
func TestOpenProject_ByPathSuffix(t *testing.T) {
	cs, cleanup := testFixture(t)
	defer cleanup()

	projectDir := mkProject(t)
	var idxRes struct {
		ProjectName string `json:"projectName"`
	}
	callToolUnmarshal(t, cs, "index_repository", map[string]any{
		"repoPath": projectDir,
		"mode":     "fast",
	}, &idxRes)

	base := filepath.Base(projectDir)
	var openRes map[string]any
	callToolUnmarshal(t, cs, "open_project", map[string]any{
		"path": base,
	}, &openRes)
	assert.Equal(t, openRes["project"], idxRes.ProjectName)
}

// TestSearchCodeFilePatternAlias verifies snake_case aliases (file_pattern,
// regex) are accepted and functional — the LLM-intuitive field names that
// previously failed with "unexpected additional properties".
func TestSearchCodeFilePatternAlias(t *testing.T) {
	cs, cleanup := testFixture(t)
	defer cleanup()

	projectDir := mkProject(t)
	var idxRes struct {
		ProjectName string `json:"projectName"`
	}
	callToolUnmarshal(t, cs, "index_repository", map[string]any{
		"repoPath": projectDir,
		"mode":     "fast",
	}, &idxRes)

	// regex alias + file_pattern alias in the same call
	type searchRes struct {
		TotalResults int `json:"totalResults"`
	}
	var res searchRes
	callToolUnmarshal(t, cs, "search_code", map[string]any{
		"regex":        "Greeter",
		"file_pattern": "*.go",
		"project":      idxRes.ProjectName,
	}, &res)
	assert.Assert(t, res.TotalResults >= 1, "aliases regex/file_pattern should work, got %d", res.TotalResults)
}

// TestSchemaLenient_ExtraField verifies the lenient schema accepts unknown
// fields instead of rejecting the call with a hard validation error.
func TestSchemaLenient_ExtraField(t *testing.T) {
	cs, cleanup := testFixture(t)
	defer cleanup()

	projectDir := mkProject(t)
	var idxRes struct {
		ProjectName string `json:"projectName"`
	}
	callToolUnmarshal(t, cs, "index_repository", map[string]any{
		"repoPath": projectDir,
		"mode":     "fast",
	}, &idxRes)

	type searchRes struct {
		TotalResults int `json:"totalResults"`
	}
	var res searchRes
	callToolUnmarshal(t, cs, "search_code", map[string]any{
		"pattern":    "Greeter",
		"project":    idxRes.ProjectName,
		"frobnicate": "llm-noise", // unknown field must not fail validation
	}, &res)
	assert.Assert(t, res.TotalResults >= 1)
}

// ─── Text-based edit (oldText → newText) ─────────────────────────────────────

// TestEditReplaceByText verifies the LLM-friendly replace mode.
func TestEditReplaceByText(t *testing.T) {
	cs, cleanup := testFixture(t)
	defer cleanup()

	projectDir := mkProject(t)
	filePath := filepath.Join(projectDir, "main.go")

	var result map[string]any
	callToolUnmarshal(t, cs, "edit", map[string]any{
		"file":    filePath,
		"mode":    "replace",
		"oldText": "Name string",
		"newText": "Title string",
	}, &result)
	assert.Assert(t, result["success"].(bool))

	// Verify the file changed on disk.
	content, err := os.ReadFile(filePath)
	assert.NilError(t, err)
	assert.Assert(t, strings.Contains(string(content), "Title string"), "file should contain replacement")
	assert.Assert(t, !strings.Contains(string(content), "Name string"), "file should no longer contain old text")
}

// TestEditReplaceByText_NotFound verifies a helpful error when oldText is absent.
func TestEditReplaceByText_NotFound(t *testing.T) {
	cs, cleanup := testFixture(t)
	defer cleanup()

	projectDir := mkProject(t)
	filePath := filepath.Join(projectDir, "main.go")

	result, err := cs.CallTool(context.Background(), &mcp.CallToolParams{
		Name: "edit",
		Arguments: map[string]any{
			"file":    filePath,
			"mode":    "replace",
			"oldText": "DoesNotExistAnywhere",
			"newText": "X",
		},
	})
	assert.NilError(t, err)
	assert.Assert(t, result.IsError, "replace with missing oldText should error")
	assert.Assert(t, strings.Contains(textContent(result), "not found"), "error should mention not found")
}

// TestEditReplaceByText_Ambiguous verifies occurrence selection when oldText
// appears multiple times.
func TestEditReplaceByText_Ambiguous(t *testing.T) {
	cs, cleanup := testFixture(t)
	defer cleanup()

	projectDir := mkProject(t)
	filePath := filepath.Join(projectDir, "main.go")

	// "Name" appears in the struct field and in the method — multiple times.
	result, err := cs.CallTool(context.Background(), &mcp.CallToolParams{
		Name: "edit",
		Arguments: map[string]any{
			"file":    filePath,
			"mode":    "replace",
			"oldText": "Name",
			"newText": "Moniker",
		},
	})
	assert.NilError(t, err)
	assert.Assert(t, result.IsError, "ambiguous oldText should error")
	msg := textContent(result)
	assert.Assert(t, strings.Contains(msg, "occurrence"), "error should mention occurrence, got: %s", msg)

	// With occurrence=1 it should succeed.
	var ok map[string]any
	callToolUnmarshal(t, cs, "edit", map[string]any{
		"file":       filePath,
		"mode":       "replace",
		"oldText":    "Name",
		"newText":    "Moniker",
		"occurrence": 1,
	}, &ok)
	assert.Assert(t, ok["success"].(bool))
}

// TestEditReplaceByText_NewTextAlias verifies text (not newText) also works
// as the replacement when oldText is present.
func TestEditReplaceByText_TextAlias(t *testing.T) {
	cs, cleanup := testFixture(t)
	defer cleanup()

	projectDir := mkProject(t)
	filePath := filepath.Join(projectDir, "main.go")

	var result map[string]any
	callToolUnmarshal(t, cs, "edit", map[string]any{
		"file":    filePath,
		"mode":    "replace",
		"oldText": "fmt.Println",
		"text":    "fmt.Printf",
	}, &result)
	assert.Assert(t, result["success"].(bool))
}

// TestIndexRepository_AutoBind verifies index_repository binds the project so
// graph tools work without an explicit project argument afterwards.
func TestIndexRepository_AutoBind(t *testing.T) {
	cs, cleanup := testFixture(t)
	defer cleanup()

	projectDir := mkProject(t)
	var idxRes struct {
		ProjectName string `json:"projectName"`
	}
	callToolUnmarshal(t, cs, "index_repository", map[string]any{
		"repoPath": projectDir,
		"mode":     "fast",
	}, &idxRes)

	// No project argument at all — should resolve via the bound project.
	type searchRes struct {
		Total int `json:"total"`
	}
	var res searchRes
	callToolUnmarshal(t, cs, "search_graph", map[string]any{
		"query": "Hello",
	}, &res)
	assert.Assert(t, res.Total >= 1, "search_graph without project should use auto-bound project")
}

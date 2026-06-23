package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/nanjj/slingshot/internal/editor"
)

// ─── helpers ────────────────────────────────────────────────────────────────

func newTestServer(t *testing.T) *editorServer {
	t.Helper()
	mgr, err := editor.NewEditorManager(t.TempDir())
	if err != nil {
		t.Fatalf("NewEditorManager: %v", err)
	}
	return &editorServer{mgr: mgr, opts: &serveOptions{}}
}

// mustOpen is a test helper that opens a scratch document, failing on error.
func mustOpen(t *testing.T, es *editorServer, uri, source, lang string) {
	t.Helper()
	args := openDocumentArgs{URI: uri, Source: source, Language: lang}
	res := es.openDocument(args)
	if res.IsError {
		t.Fatalf("openDocument(%q) unexpected error: %s", uri, extractText(res))
	}
}

// extractText extracts the text from the first text content in a CallToolResult.
func extractText(res *mcp.CallToolResult) string {
	if len(res.Content) == 0 {
		return ""
	}
	if tc, ok := res.Content[0].(*mcp.TextContent); ok {
		return tc.Text
	}
	return ""
}

// unmarshalJSONText unmarshals the JSON body of a successful result into v.
func unmarshalJSONText(t *testing.T, res *mcp.CallToolResult, v any) {
	t.Helper()
	if res.IsError {
		t.Fatalf("unexpected error result: %s", extractText(res))
	}
	body := extractText(res)
	if body == "" {
		t.Fatal("empty result body")
	}
	if err := json.Unmarshal([]byte(body), v); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}
}

// extractBoolMap extracts a map[string]bool from a result.
func extractBoolMap(t *testing.T, res *mcp.CallToolResult) map[string]bool {
	t.Helper()
	var m map[string]bool
	unmarshalJSONText(t, res, &m)
	return m
}

// ─── 1. editor_open_document ───────────────────────────────────────────────

func TestOpenDocumentHandler_scratchWithLanguage(t *testing.T) {
	es := newTestServer(t)
	args := openDocumentArgs{
		URI:      "scratch:///test.go",
		Source:   "package main\n",
		Language: "go",
	}
	res := es.openDocument(args)
	if res.IsError {
		t.Fatalf("unexpected error: %s", extractText(res))
	}
	m := extractBoolMap(t, res)
	if !m["success"] {
		t.Error("expected success=true")
	}
}

func TestOpenDocumentHandler_scratchAutoLanguage(t *testing.T) {
	es := newTestServer(t)
	args := openDocumentArgs{
		URI:    "scratch:///test.go",
		Source: "package main\n",
	}
	res := es.openDocument(args)
	if res.IsError {
		t.Fatalf("unexpected error: %s", extractText(res))
	}
}

func TestOpenDocumentHandler_unsupportedLanguage(t *testing.T) {
	es := newTestServer(t)
	args := openDocumentArgs{
		URI:      "scratch:///test.xyz",
		Source:   "content",
		Language: "unsupported-lang",
	}
	res := es.openDocument(args)
	if !res.IsError {
		t.Fatal("expected error for unsupported language")
	}
}

func TestOpenDocumentHandler_newFile(t *testing.T) {
	es := newTestServer(t)
	filePath := filepath.Join(t.TempDir(), "new.go")
	args := openDocumentArgs{
		URI:      "file://" + filePath,
		Source:   "package new\n",
		Language: "go",
	}
	res := es.openDocument(args)
	if res.IsError {
		t.Fatalf("unexpected error: %s", extractText(res))
	}
}

func TestOpenDocumentHandler_duplicateReplaces(t *testing.T) {
	es := newTestServer(t)
	mustOpen(t, es, "scratch:///dup.go", "package a\n", "go")

	// reopen with different source
	args := openDocumentArgs{
		URI:      "scratch:///dup.go",
		Source:   "package b\n",
		Language: "go",
	}
	res := es.openDocument(args)
	if res.IsError {
		t.Fatalf("reopen failed: %s", extractText(res))
	}
}

// ─── 2. editor_close_document ──────────────────────────────────────────────

func TestCloseDocumentHandler_existing(t *testing.T) {
	es := newTestServer(t)
	mustOpen(t, es, "scratch:///close.go", "package main\n", "go")

	args := closeDocumentArgs{URI: "scratch:///close.go"}
	res := es.closeDocument(args)
	if res.IsError {
		t.Fatalf("unexpected error: %s", extractText(res))
	}
}

func TestCloseDocumentHandler_notFound(t *testing.T) {
	es := newTestServer(t)
	args := closeDocumentArgs{URI: "scratch:///nonexistent.go"}
	res := es.closeDocument(args)
	if !res.IsError {
		t.Fatal("expected error for closing non-existent document")
	}
}

func TestCloseDocumentHandler_doubleClose(t *testing.T) {
	es := newTestServer(t)
	mustOpen(t, es, "scratch:///double.go", "package main\n", "go")
	es.closeDocument(closeDocumentArgs{URI: "scratch:///double.go"})

	// second close should fail
	res := es.closeDocument(closeDocumentArgs{URI: "scratch:///double.go"})
	if !res.IsError {
		t.Fatal("expected error for double close")
	}
}

// ─── 3. editor_get_structure ───────────────────────────────────────────────

func TestGetStructureHandler_basic(t *testing.T) {
	es := newTestServer(t)
	mustOpen(t, es, "scratch:///struct.go", "package main\nfunc main() {}\n", "go")

	args := getStructureArgs{URI: "scratch:///struct.go"}
	res := es.getStructure(args)
	if res.IsError {
		t.Fatalf("unexpected error: %s", extractText(res))
	}
	var info map[string]any
	unmarshalJSONText(t, res, &info)
	if info["type"] != "source_file" {
		t.Errorf("expected type=source_file, got %v", info["type"])
	}
}

func TestGetStructureHandler_notOpened(t *testing.T) {
	// A scratch URI with .go extension auto-opens successfully
	// (empty doc with Go language detected from extension).
	es := newTestServer(t)
	args := getStructureArgs{URI: "scratch:///autogo.go"}
	res := es.getStructure(args)
	if res.IsError {
		t.Fatalf("unexpected error for scratch URI with .go extension: %s", extractText(res))
	}
}

func TestGetStructureHandler_withMaxDepth(t *testing.T) {
	es := newTestServer(t)
	mustOpen(t, es, "scratch:///deep.go", "package main\nfunc main() {}\n", "go")

	// maxDepth=0 should default to -1 (full tree) in the handler
	args := getStructureArgs{URI: "scratch:///deep.go", MaxDepth: 0}
	res := es.getStructure(args)
	if res.IsError {
		t.Fatalf("unexpected error: %s", extractText(res))
	}
}

// ─── 4. editor_get_node (unified) ─────────────────────────────────────────

func TestGetNodeHandler_scopePos(t *testing.T) {
	es := newTestServer(t)
	mustOpen(t, es, "scratch:///node.go", "package main\n", "go")

	args := editorGetNodeArgs{URI: "scratch:///node.go", Pos: 0}
	res := es.getNode(args)
	if res.IsError {
		t.Fatalf("unexpected error: %s", extractText(res))
	}
}

func TestGetNodeHandler_scopePos_outOfRange(t *testing.T) {
	es := newTestServer(t)
	mustOpen(t, es, "scratch:///node.go", "package main\n", "go")

	args := editorGetNodeArgs{URI: "scratch:///node.go", Pos: 9999}
	res := es.getNode(args)
	if !res.IsError {
		t.Fatal("expected error for out-of-range position")
	}
}

func TestGetNodeHandler_scopePoint(t *testing.T) {
	es := newTestServer(t)
	mustOpen(t, es, "scratch:///point.go", "package main\n", "go")

	args := editorGetNodeArgs{URI: "scratch:///point.go", Scope: "point", Row: 0, Col: 0}
	res := es.getNode(args)
	if res.IsError {
		t.Fatalf("unexpected error: %s", extractText(res))
	}
}

func TestGetNodeHandler_scopePoint_invalid(t *testing.T) {
	es := newTestServer(t)
	mustOpen(t, es, "scratch:///point.go", "package main\n", "go")

	args := editorGetNodeArgs{URI: "scratch:///point.go", Scope: "point", Row: 999, Col: 0}
	res := es.getNode(args)
	if !res.IsError {
		t.Fatal("expected error for invalid point")
	}
}

func TestGetNodeHandler_scopeRange(t *testing.T) {
	es := newTestServer(t)
	mustOpen(t, es, "scratch:///range.go", "package main\n", "go")

	args := editorGetNodeArgs{URI: "scratch:///range.go", Scope: "range", StartByte: 0, EndByte: 1}
	res := es.getNode(args)
	if res.IsError {
		t.Fatalf("unexpected error: %s", extractText(res))
	}
}

func TestGetNodeHandler_scopeRange_invalid(t *testing.T) {
	es := newTestServer(t)
	mustOpen(t, es, "scratch:///range.go", "package main\n", "go")

	args := editorGetNodeArgs{URI: "scratch:///range.go", Scope: "range", StartByte: 0, EndByte: 9999}
	res := es.getNode(args)
	if !res.IsError {
		t.Fatal("expected error for invalid range")
	}
}

func TestGetNodeHandler_scopeDescendants(t *testing.T) {
	es := newTestServer(t)
	mustOpen(t, es, "scratch:///desc.go", "package main\nfunc main() {}\n", "go")

	args := editorGetNodeArgs{URI: "scratch:///desc.go", Scope: "descendants", Pos: 0}
	res := es.getNode(args)
	if res.IsError {
		t.Fatalf("unexpected error: %s", extractText(res))
	}
	var infos []map[string]any
	unmarshalJSONText(t, res, &infos)
	if len(infos) == 0 {
		t.Error("expected at least one descendant")
	}
}

func TestGetNodeHandler_scopeDescendants_outOfRange(t *testing.T) {
	es := newTestServer(t)
	mustOpen(t, es, "scratch:///desc.go", "package main\n", "go")

	args := editorGetNodeArgs{URI: "scratch:///desc.go", Scope: "descendants", Pos: 9999}
	res := es.getNode(args)
	if !res.IsError {
		t.Fatal("expected error for out-of-range position")
	}
}

// ─── 5. editor_get_text (unified) ─────────────────────────────────────────

func TestGetTextHandler_byRange(t *testing.T) {
	es := newTestServer(t)
	mustOpen(t, es, "scratch:///text.go", "package main\nfunc main() {}\n", "go")

	args := editorGetTextArgs{URI: "scratch:///text.go", StartByte: 0, EndByte: 7}
	res := es.getText(args)
	if res.IsError {
		t.Fatalf("unexpected error: %s", extractText(res))
	}
	var out map[string]string
	unmarshalJSONText(t, res, &out)
	if out["text"] != "package" {
		t.Errorf("expected text='package', got %q", out["text"])
	}
}

func TestGetTextHandler_byRange_invalid(t *testing.T) {
	es := newTestServer(t)
	mustOpen(t, es, "scratch:///text.go", "package main\n", "go")

	args := editorGetTextArgs{URI: "scratch:///text.go", StartByte: 0, EndByte: 9999}
	res := es.getText(args)
	if !res.IsError {
		t.Fatal("expected error for invalid range")
	}
}

func TestGetTextHandler_byLine(t *testing.T) {
	es := newTestServer(t)
	mustOpen(t, es, "scratch:///line.go",
		"line zero\nline one\nline two\n", "go")

	args := editorGetTextArgs{URI: "scratch:///line.go", By: "line", Line: 1}
	res := es.getText(args)
	if res.IsError {
		t.Fatalf("unexpected error: %s", extractText(res))
	}
	var out map[string]string
	unmarshalJSONText(t, res, &out)
	if out["text"] != "line one" {
		t.Errorf("expected text='line one', got %q", out["text"])
	}
}

func TestGetTextHandler_byLine_invalid(t *testing.T) {
	es := newTestServer(t)
	mustOpen(t, es, "scratch:///line.go", "package main\n", "go")

	args := editorGetTextArgs{URI: "scratch:///line.go", By: "line", Line: 999}
	res := es.getText(args)
	if !res.IsError {
		t.Fatal("expected error for invalid line")
	}
}

// ─── 6. editor_query ───────────────────────────────────────────────────────

func TestQueryHandler_valid(t *testing.T) {
	es := newTestServer(t)
	mustOpen(t, es, "scratch:///query.go",
		"package main\nfunc main() {}\n", "go")

	args := queryArgs{
		URI:     "scratch:///query.go",
		Pattern: "(function_declaration name: (identifier) @name)",
	}
	res := es.query(args)
	if res.IsError {
		t.Fatalf("unexpected error: %s", extractText(res))
	}
	var results []map[string]any
	unmarshalJSONText(t, res, &results)
	if len(results) == 0 {
		t.Error("expected at least one query result")
	}
}

func TestQueryHandler_nonMatchingPattern(t *testing.T) {
	es := newTestServer(t)
	mustOpen(t, es, "scratch:///query.go",
		"package main\nfunc main() {}\n", "go")

	args := queryArgs{
		URI:     "scratch:///query.go",
		Pattern: `(comment) @comment`,
	}
	res := es.query(args)
	if res.IsError {
		t.Fatalf("unexpected error for non-matching pattern: %s", extractText(res))
	}
	var results []map[string]any
	unmarshalJSONText(t, res, &results)
	if len(results) != 0 {
		t.Errorf("expected 0 results for non-matching pattern, got %d", len(results))
	}
}

// ─── 7. editor_insert (unified) ────────────────────────────────────────────

func TestInsertHandler_positionPos(t *testing.T) {
	es := newTestServer(t)
	mustOpen(t, es, "scratch:///insert.go", "package main\n", "go")

	args := editorInsertArgs{
		URI:  "scratch:///insert.go",
		Pos:  0,
		Text: "// comment\n",
	}
	res := es.insert(args)
	if res.IsError {
		t.Fatalf("unexpected error: %s", extractText(res))
	}
	var out editResultResponse
	unmarshalJSONText(t, res, &out)
	if !out.Success {
		t.Error("expected success=true")
	}
}

func TestInsertHandler_positionPos_outOfBounds(t *testing.T) {
	es := newTestServer(t)
	mustOpen(t, es, "scratch:///insert.go", "package main\n", "go")

	args := editorInsertArgs{
		URI:  "scratch:///insert.go",
		Pos:  9999,
		Text: "// comment\n",
	}
	res := es.insert(args)
	if !res.IsError {
		t.Fatal("expected error for out-of-bounds insert")
	}
}

func TestInsertHandler_positionPoint(t *testing.T) {
	es := newTestServer(t)
	mustOpen(t, es, "scratch:///iap.go", "package main\n", "go")

	args := editorInsertArgs{
		URI:      "scratch:///iap.go",
		Position: "point",
		Row:      0,
		Col:      0,
		Text:     "// comment\n",
	}
	res := es.insert(args)
	if res.IsError {
		t.Fatalf("unexpected error: %s", extractText(res))
	}
	var out editResultResponse
	unmarshalJSONText(t, res, &out)
	if !out.Success {
		t.Error("expected success=true")
	}
}

func TestInsertHandler_positionPoint_emptyDoc(t *testing.T) {
	es := newTestServer(t)
	mustOpen(t, es, "scratch:///iap.go", "", "go")

	args := editorInsertArgs{
		URI:      "scratch:///iap.go",
		Position: "point",
		Row:      0,
		Col:      0,
		Text:     "// comment\n",
	}
	res := es.insert(args)
	if res.IsError {
		t.Fatalf("insert at (0,0) on empty doc should succeed: %s", extractText(res))
	}
	var out editResultResponse
	unmarshalJSONText(t, res, &out)
	if !out.Success {
		t.Error("expected success=true")
	}
}

func TestInsertHandler_positionBefore(t *testing.T) {
	es := newTestServer(t)
	mustOpen(t, es, "scratch:///ib.go",
		"package main\nfunc main() {}\n", "go")

	sel := editor.NodeSelector{
		Path: []editor.PathStep{
			{Type: "function_declaration"},
		},
	}
	args := editorInsertArgs{
		URI:      "scratch:///ib.go",
		Position: "before",
		Selector: sel,
		Text:     "// before\n",
	}
	res := es.insert(args)
	if res.IsError {
		t.Fatalf("unexpected error: %s", extractText(res))
	}
}

func TestInsertHandler_positionBefore_emptySelector(t *testing.T) {
	es := newTestServer(t)
	mustOpen(t, es, "scratch:///ib.go", "package main\n", "go")

	args := editorInsertArgs{
		URI:      "scratch:///ib.go",
		Position: "before",
		Selector: editor.NodeSelector{}, // empty — should error
		Text:     "// before\n",
	}
	res := es.insert(args)
	if !res.IsError {
		t.Fatal("expected error for empty selector")
	}
}

func TestInsertHandler_positionAfter(t *testing.T) {
	es := newTestServer(t)
	mustOpen(t, es, "scratch:///ia.go",
		"package main\nfunc main() {}\n", "go")

	sel := editor.NodeSelector{
		Path: []editor.PathStep{
			{Type: "function_declaration"},
		},
	}
	args := editorInsertArgs{
		URI:      "scratch:///ia.go",
		Position: "after",
		Selector: sel,
		Text:     "// after\n",
	}
	res := es.insert(args)
	if res.IsError {
		t.Fatalf("unexpected error: %s", extractText(res))
	}
}

func TestInsertHandler_positionAfter_emptySelector(t *testing.T) {
	es := newTestServer(t)
	mustOpen(t, es, "scratch:///ia.go", "package main\n", "go")

	args := editorInsertArgs{
		URI:      "scratch:///ia.go",
		Position: "after",
		Selector: editor.NodeSelector{},
		Text:     "// after\n",
	}
	res := es.insert(args)
	if !res.IsError {
		t.Fatal("expected error for empty selector")
	}
}

// ─── 8. editor_replace (unified) ───────────────────────────────────────────

func TestReplaceHandler_targetRange(t *testing.T) {
	es := newTestServer(t)
	mustOpen(t, es, "scratch:///replace.go",
		"package main\nfunc main() {}\n", "go")

	args := editorReplaceArgs{
		URI:       "scratch:///replace.go",
		StartByte: 0,
		EndByte:   13,
		Text:      "// replaced\n",
	}
	res := es.replace(args)
	if res.IsError {
		t.Fatalf("unexpected error: %s", extractText(res))
	}
	var out editResultResponse
	unmarshalJSONText(t, res, &out)
	if !out.Success {
		t.Error("expected success=true")
	}
}

func TestReplaceHandler_targetRange_outOfBounds(t *testing.T) {
	es := newTestServer(t)
	mustOpen(t, es, "scratch:///replace.go", "package main\n", "go")

	args := editorReplaceArgs{
		URI:       "scratch:///replace.go",
		StartByte: 0,
		EndByte:   9999,
		Text:      "// replaced\n",
	}
	res := es.replace(args)
	if !res.IsError {
		t.Fatal("expected error for out-of-bounds replace")
	}
}

func TestReplaceHandler_targetNode(t *testing.T) {
	es := newTestServer(t)
	mustOpen(t, es, "scratch:///rn.go",
		"package main\nfunc main() {}\n", "go")

	sel := editor.NodeSelector{
		Path: []editor.PathStep{
			{Type: "function_declaration"},
		},
	}
	args := editorReplaceArgs{
		URI:      "scratch:///rn.go",
		Target:   "node",
		Selector: sel,
		Text:     "func replaced() {}",
	}
	res := es.replace(args)
	if res.IsError {
		t.Fatalf("unexpected error: %s", extractText(res))
	}
}

func TestReplaceHandler_targetNode_badSelector(t *testing.T) {
	es := newTestServer(t)
	mustOpen(t, es, "scratch:///rn.go", "package main\n", "go")

	sel := editor.NodeSelector{
		Path: []editor.PathStep{
			{Type: "nonexistent_type"},
		},
	}
	args := editorReplaceArgs{
		URI:      "scratch:///rn.go",
		Target:   "node",
		Selector: sel,
		Text:     "func replaced() {}",
	}
	res := es.replace(args)
	if !res.IsError {
		t.Fatal("expected error for bad selector")
	}
}

// ─── 9. editor_delete (unified) ────────────────────────────────────────────

func TestDeleteHandler_targetRange(t *testing.T) {
	es := newTestServer(t)
	mustOpen(t, es, "scratch:///del.go",
		"package main\nfunc main() {}\n", "go")

	args := editorDeleteArgs{
		URI:       "scratch:///del.go",
		StartByte: 0,
		EndByte:   13,
	}
	res := es.delete(args)
	if res.IsError {
		t.Fatalf("unexpected error: %s", extractText(res))
	}
	var out editResultResponse
	unmarshalJSONText(t, res, &out)
	if !out.Success {
		t.Error("expected success=true")
	}
}

func TestDeleteHandler_targetRange_outOfBounds(t *testing.T) {
	es := newTestServer(t)
	mustOpen(t, es, "scratch:///del.go", "package main\n", "go")

	args := editorDeleteArgs{
		URI:       "scratch:///del.go",
		StartByte: 0,
		EndByte:   9999,
	}
	res := es.delete(args)
	if !res.IsError {
		t.Fatal("expected error for out-of-bounds delete")
	}
}

func TestDeleteHandler_targetNode(t *testing.T) {
	es := newTestServer(t)
	mustOpen(t, es, "scratch:///dn.go",
		"package main\nfunc main() {}\n", "go")

	sel := editor.NodeSelector{
		Path: []editor.PathStep{
			{Type: "function_declaration"},
		},
	}
	args := editorDeleteArgs{
		URI:      "scratch:///dn.go",
		Target:   "node",
		Selector: sel,
	}
	res := es.delete(args)
	if res.IsError {
		t.Fatalf("unexpected error: %s", extractText(res))
	}
}

func TestDeleteHandler_targetNode_badSelector(t *testing.T) {
	es := newTestServer(t)
	mustOpen(t, es, "scratch:///dn.go", "package main\n", "go")

	sel := editor.NodeSelector{
		Path: []editor.PathStep{
			{Type: "nonexistent_type"},
		},
	}
	args := editorDeleteArgs{
		URI:      "scratch:///dn.go",
		Target:   "node",
		Selector: sel,
	}
	res := es.delete(args)
	if !res.IsError {
		t.Fatal("expected error for bad selector")
	}
}

// ─── 10. editor_save (unified) ─────────────────────────────────────────────

func TestSaveHandler_scratchFails(t *testing.T) {
	es := newTestServer(t)
	mustOpen(t, es, "scratch:///save.go", "package main\n", "go")

	args := editorSaveArgs{URI: "scratch:///save.go"}
	res := es.save(args)
	if !res.IsError {
		t.Fatal("expected error for saving scratch:// URI")
	}
}

func TestSaveHandler_newFile(t *testing.T) {
	es := newTestServer(t)
	dir := t.TempDir()
	filePath := filepath.Join(dir, "saved.go")
	uri := "file://" + filePath
	mustOpen(t, es, uri, "package main\n", "go")

	args := editorSaveArgs{URI: uri}
	res := es.save(args)
	if res.IsError {
		t.Fatalf("unexpected error: %s", extractText(res))
	}
	var out saveResultResponse
	unmarshalJSONText(t, res, &out)
	if !out.Success {
		t.Error("expected success=true")
	}
	if out.Bytes <= 0 {
		t.Errorf("expected bytes > 0, got %d", out.Bytes)
	}

	// Verify the file was actually created.
	content, err := os.ReadFile(filePath)
	if err != nil {
		t.Fatalf("reading saved file: %v", err)
	}
	if string(content) != "package main\n" {
		t.Errorf("file content: got %q, want %q", string(content), "package main\n")
	}
}

func TestSaveHandler_notFound(t *testing.T) {
	es := newTestServer(t)
	args := editorSaveArgs{URI: "file:///nonexistent.go"}
	res := es.save(args)
	if !res.IsError {
		t.Fatal("expected error for saving non-existent document")
	}
}

func TestSaveHandler_force(t *testing.T) {
	es := newTestServer(t)
	dir := t.TempDir()
	filePath := filepath.Join(dir, "forced.go")
	uri := "file://" + filePath
	mustOpen(t, es, uri, "package main\n", "go")

	args := editorSaveArgs{URI: uri, Force: true}
	res := es.save(args)
	if res.IsError {
		t.Fatalf("unexpected error: %s", extractText(res))
	}
	var out saveResultResponse
	unmarshalJSONText(t, res, &out)
	if !out.Success {
		t.Error("expected success=true")
	}
}

// ─── 11. editor_validate (unified) ─────────────────────────────────────────

func TestValidateHandler_basic(t *testing.T) {
	es := newTestServer(t)
	mustOpen(t, es, "scratch:///val.go",
		"package main\nfunc main() {}\n", "go")

	args := editorValidateArgs{URI: "scratch:///val.go"}
	res := es.validate(args)
	if res.IsError {
		t.Fatalf("unexpected error: %s", extractText(res))
	}
	var out validateResponse
	unmarshalJSONText(t, res, &out)
	if !out.Valid {
		t.Error("expected valid=true for correct code")
	}
}

func TestValidateHandler_syntaxError(t *testing.T) {
	es := newTestServer(t)
	mustOpen(t, es, "scratch:///valerr.go",
		"package main\nfunc main() {\nunclosed\n", "go")

	args := editorValidateArgs{URI: "scratch:///valerr.go"}
	res := es.validate(args)
	if res == nil {
		t.Fatal("validate returned nil")
	}
	// Validate may return IsError or include errors inline — both OK
}

func TestValidateHandler_includeDirty(t *testing.T) {
	es := newTestServer(t)
	mustOpen(t, es, "scratch:///dirty.go", "package main\n", "go")

	// Make an edit to mark as dirty
	es.insert(editorInsertArgs{URI: "scratch:///dirty.go", Pos: 0, Text: "// comment\n"})

	args := editorValidateArgs{URI: "scratch:///dirty.go", IncludeDirty: true}
	res := es.validate(args)
	if res.IsError {
		t.Fatalf("unexpected error: %s", extractText(res))
	}
	var out validateResponse
	unmarshalJSONText(t, res, &out)
	if out.Dirty == nil {
		t.Fatal("expected dirty field to be set when includeDirty=true")
	}
	if !*out.Dirty {
		t.Error("expected dirty=true after edit")
	}
	if len(out.DirtyURIs) == 0 {
		t.Error("expected non-empty DirtyURIs")
	}
}

func TestValidateHandler_includeDirty_afterOpen(t *testing.T) {
	es := newTestServer(t)
	mustOpen(t, es, "scratch:///clean.go", "package main\n", "go")

	args := editorValidateArgs{URI: "scratch:///clean.go", IncludeDirty: true}
	res := es.validate(args)
	if res.IsError {
		t.Fatalf("unexpected error: %s", extractText(res))
	}
	var out validateResponse
	unmarshalJSONText(t, res, &out)
	if out.Dirty == nil {
		t.Fatal("expected dirty field to be set when includeDirty=true")
	}
	if *out.Dirty {
		t.Error("expected dirty=false for new doc")
	}
}

func TestValidateHandler_listDirty(t *testing.T) {
	es := newTestServer(t)
	mustOpen(t, es, "scratch:///dirty1.go", "package main\n", "go")
	mustOpen(t, es, "scratch:///dirty2.go", "package main\n", "go")
	es.insert(editorInsertArgs{URI: "scratch:///dirty1.go", Pos: 0, Text: "// dirty\n"})

	args := editorValidateArgs{URI: "scratch:///dirty1.go", IncludeDirty: true}
	res := es.validate(args)
	if res.IsError {
		t.Fatalf("unexpected error: %s", extractText(res))
	}
	var out validateResponse
	unmarshalJSONText(t, res, &out)
	if len(out.DirtyURIs) != 1 {
		t.Errorf("expected 1 dirty URI, got %v", out.DirtyURIs)
	}
	if len(out.DirtyURIs) > 0 && out.DirtyURIs[0] != "scratch:///dirty1.go" {
		t.Errorf("expected dirty1.go, got %v", out.DirtyURIs[0])
	}
}

// ─── 12. editor_get_project_root ───────────────────────────────────────────

func TestGetProjectRootHandler(t *testing.T) {
	dir := t.TempDir()
	mgr, err := editor.NewEditorManager(dir)
	if err != nil {
		t.Fatalf("NewEditorManager: %v", err)
	}
	es := &editorServer{mgr: mgr, opts: &serveOptions{}}

	res := es.getProjectRoot()
	if res.IsError {
		t.Fatalf("unexpected error: %s", extractText(res))
	}
	var out map[string]string
	unmarshalJSONText(t, res, &out)
	if out["projectRoot"] != dir {
		t.Errorf("expected projectRoot=%q, got %q", dir, out["projectRoot"])
	}
}

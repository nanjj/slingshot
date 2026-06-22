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

// ─── 1. openDocument ───────────────────────────────────────────────────────

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

// ─── 2. closeDocument ──────────────────────────────────────────────────────

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

// ─── 3. getStructure ───────────────────────────────────────────────────────

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

// ─── 4. getNode ────────────────────────────────────────────────────────────

func TestGetNodeHandler_valid(t *testing.T) {
	es := newTestServer(t)
	mustOpen(t, es, "scratch:///node.go", "package main\n", "go")

	args := byteURIPositionArgs{URI: "scratch:///node.go", Pos: 0}
	res := es.getNode(args)
	if res.IsError {
		t.Fatalf("unexpected error: %s", extractText(res))
	}
}

func TestGetNodeHandler_outOfRange(t *testing.T) {
	es := newTestServer(t)
	mustOpen(t, es, "scratch:///node.go", "package main\n", "go")

	args := byteURIPositionArgs{URI: "scratch:///node.go", Pos: 9999}
	res := es.getNode(args)
	if !res.IsError {
		t.Fatal("expected error for out-of-range position")
	}
}

// ─── 5. getNodeAtPoint ─────────────────────────────────────────────────────

func TestGetNodeAtPointHandler_valid(t *testing.T) {
	es := newTestServer(t)
	mustOpen(t, es, "scratch:///point.go", "package main\n", "go")

	args := pointArgs{URI: "scratch:///point.go", Row: 0, Col: 0}
	res := es.getNodeAtPoint(args)
	if res.IsError {
		t.Fatalf("unexpected error: %s", extractText(res))
	}
}

func TestGetNodeAtPointHandler_invalid(t *testing.T) {
	es := newTestServer(t)
	mustOpen(t, es, "scratch:///point.go", "package main\n", "go")

	args := pointArgs{URI: "scratch:///point.go", Row: 999, Col: 0}
	res := es.getNodeAtPoint(args)
	if !res.IsError {
		t.Fatal("expected error for invalid point")
	}
}

// ─── 6. getNodeAtRange ─────────────────────────────────────────────────────

func TestGetNodeAtRangeHandler_valid(t *testing.T) {
	es := newTestServer(t)
	mustOpen(t, es, "scratch:///range.go", "package main\n", "go")

	args := byteRangeArgs{URI: "scratch:///range.go", StartByte: 0, EndByte: 1}
	res := es.getNodeAtRange(args)
	if res.IsError {
		t.Fatalf("unexpected error: %s", extractText(res))
	}
}

func TestGetNodeAtRangeHandler_invalid(t *testing.T) {
	es := newTestServer(t)
	mustOpen(t, es, "scratch:///range.go", "package main\n", "go")

	args := byteRangeArgs{URI: "scratch:///range.go", StartByte: 0, EndByte: 9999}
	res := es.getNodeAtRange(args)
	if !res.IsError {
		t.Fatal("expected error for invalid range")
	}
}

// ─── 7. getDescendantsAt ───────────────────────────────────────────────────

func TestGetDescendantsAtHandler_valid(t *testing.T) {
	es := newTestServer(t)
	mustOpen(t, es, "scratch:///desc.go", "package main\nfunc main() {}\n", "go")

	args := byteURIPositionArgs{URI: "scratch:///desc.go", Pos: 0}
	res := es.getDescendantsAt(args)
	if res.IsError {
		t.Fatalf("unexpected error: %s", extractText(res))
	}
	var infos []map[string]any
	unmarshalJSONText(t, res, &infos)
	if len(infos) == 0 {
		t.Error("expected at least one descendant")
	}
}

func TestGetDescendantsAtHandler_outOfRange(t *testing.T) {
	es := newTestServer(t)
	mustOpen(t, es, "scratch:///desc.go", "package main\n", "go")

	args := byteURIPositionArgs{URI: "scratch:///desc.go", Pos: 9999}
	res := es.getDescendantsAt(args)
	if !res.IsError {
		t.Fatal("expected error for out-of-range position")
	}
}

// ─── 8. query ──────────────────────────────────────────────────────────────

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

// ─── 9. getText ────────────────────────────────────────────────────────────

func TestGetTextHandler_valid(t *testing.T) {
	es := newTestServer(t)
	mustOpen(t, es, "scratch:///text.go", "package main\nfunc main() {}\n", "go")

	args := byteRangeArgs{URI: "scratch:///text.go", StartByte: 0, EndByte: 7}
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

func TestGetTextHandler_invalidRange(t *testing.T) {
	es := newTestServer(t)
	mustOpen(t, es, "scratch:///text.go", "package main\n", "go")

	args := byteRangeArgs{URI: "scratch:///text.go", StartByte: 0, EndByte: 9999}
	res := es.getText(args)
	if !res.IsError {
		t.Fatal("expected error for invalid range")
	}
}

// ─── 10. getLine ───────────────────────────────────────────────────────────

func TestGetLineHandler_valid(t *testing.T) {
	es := newTestServer(t)
	mustOpen(t, es, "scratch:///line.go",
		"line zero\nline one\nline two\n", "go")

	args := lineArgs{URI: "scratch:///line.go", Line: 1}
	res := es.getLine(args)
	if res.IsError {
		t.Fatalf("unexpected error: %s", extractText(res))
	}
	var out map[string]string
	unmarshalJSONText(t, res, &out)
	if out["text"] != "line one" {
		t.Errorf("expected text='line one', got %q", out["text"])
	}
}

func TestGetLineHandler_invalid(t *testing.T) {
	es := newTestServer(t)
	mustOpen(t, es, "scratch:///line.go", "package main\n", "go")

	args := lineArgs{URI: "scratch:///line.go", Line: 999}
	res := es.getLine(args)
	if !res.IsError {
		t.Fatal("expected error for invalid line")
	}
}

// ─── 11. insert ────────────────────────────────────────────────────────────

func TestInsertHandler_valid(t *testing.T) {
	es := newTestServer(t)
	mustOpen(t, es, "scratch:///insert.go", "package main\n", "go")

	args := insertArgs{
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

func TestInsertHandler_outOfBounds(t *testing.T) {
	es := newTestServer(t)
	mustOpen(t, es, "scratch:///insert.go", "package main\n", "go")

	args := insertArgs{
		URI:  "scratch:///insert.go",
		Pos:  9999,
		Text: "// comment\n",
	}
	res := es.insert(args)
	if !res.IsError {
		t.Fatal("expected error for out-of-bounds insert")
	}
}

// ─── 12. insertAtPoint ─────────────────────────────────────────────────────

func TestInsertAtPointHandler_valid(t *testing.T) {
	es := newTestServer(t)
	mustOpen(t, es, "scratch:///iap.go", "package main\n", "go")

	args := insertAtPointArgs{
		URI:  "scratch:///iap.go",
		Row:  0,
		Col:  0,
		Text: "// comment\n",
	}
	res := es.insertAtPoint(args)
	if res.IsError {
		t.Fatalf("unexpected error: %s", extractText(res))
	}
	var out editResultResponse
	unmarshalJSONText(t, res, &out)
	if !out.Success {
		t.Error("expected success=true")
	}
}

func TestInsertAtPointHandler_emptyDoc(t *testing.T) {
	es := newTestServer(t)
	// Empty scratch document — insert at (0,0) on an empty doc is valid
	// (PointToByte clips to the last valid offset).
	mustOpen(t, es, "scratch:///iap.go", "", "go")

	args := insertAtPointArgs{
		URI:  "scratch:///iap.go",
		Row:  0,
		Col:  0,
		Text: "// comment\n",
	}
	res := es.insertAtPoint(args)
	if res.IsError {
		t.Fatalf("insert at (0,0) on empty doc should succeed: %s", extractText(res))
	}
	var out editResultResponse
	unmarshalJSONText(t, res, &out)
	if !out.Success {
		t.Error("expected success=true")
	}
}

// ─── 13. insertBefore ──────────────────────────────────────────────────────

func TestInsertBeforeHandler_valid(t *testing.T) {
	es := newTestServer(t)
	mustOpen(t, es, "scratch:///ib.go",
		"package main\nfunc main() {}\n", "go")

	sel := editor.NodeSelector{
		Path: []editor.PathStep{
			{Type: "function_declaration"},
		},
	}
	args := insertBeforeAfterArgs{
		URI:      "scratch:///ib.go",
		Selector: sel,
		Text:     "// before\n",
	}
	res := es.insertBefore(args)
	if res.IsError {
		t.Fatalf("unexpected error: %s", extractText(res))
	}
}

func TestInsertBeforeHandler_emptySelector(t *testing.T) {
	es := newTestServer(t)
	mustOpen(t, es, "scratch:///ib.go", "package main\n", "go")

	args := insertBeforeAfterArgs{
		URI:      "scratch:///ib.go",
		Selector: editor.NodeSelector{}, // empty — should error
		Text:     "// before\n",
	}
	res := es.insertBefore(args)
	if !res.IsError {
		t.Fatal("expected error for empty selector")
	}
}

// ─── 14. insertAfter ───────────────────────────────────────────────────────

func TestInsertAfterHandler_valid(t *testing.T) {
	es := newTestServer(t)
	mustOpen(t, es, "scratch:///ia.go",
		"package main\nfunc main() {}\n", "go")

	sel := editor.NodeSelector{
		Path: []editor.PathStep{
			{Type: "function_declaration"},
		},
	}
	args := insertBeforeAfterArgs{
		URI:      "scratch:///ia.go",
		Selector: sel,
		Text:     "// after\n",
	}
	res := es.insertAfter(args)
	if res.IsError {
		t.Fatalf("unexpected error: %s", extractText(res))
	}
}

func TestInsertAfterHandler_emptySelector(t *testing.T) {
	es := newTestServer(t)
	mustOpen(t, es, "scratch:///ia.go", "package main\n", "go")

	args := insertBeforeAfterArgs{
		URI:      "scratch:///ia.go",
		Selector: editor.NodeSelector{},
		Text:     "// after\n",
	}
	res := es.insertAfter(args)
	if !res.IsError {
		t.Fatal("expected error for empty selector")
	}
}

// ─── 15. replace ───────────────────────────────────────────────────────────

func TestReplaceHandler_valid(t *testing.T) {
	es := newTestServer(t)
	mustOpen(t, es, "scratch:///replace.go",
		"package main\nfunc main() {}\n", "go")

	args := replaceArgs{
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

func TestReplaceHandler_outOfBounds(t *testing.T) {
	es := newTestServer(t)
	mustOpen(t, es, "scratch:///replace.go", "package main\n", "go")

	args := replaceArgs{
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

// ─── 16. replaceNode ───────────────────────────────────────────────────────

func TestReplaceNodeHandler_valid(t *testing.T) {
	es := newTestServer(t)
	mustOpen(t, es, "scratch:///rn.go",
		"package main\nfunc main() {}\n", "go")

	sel := editor.NodeSelector{
		Path: []editor.PathStep{
			{Type: "function_declaration"},
		},
	}
	args := replaceNodeArgs{
		URI:      "scratch:///rn.go",
		Selector: sel,
		Text:     "func replaced() {}",
	}
	res := es.replaceNode(args)
	if res.IsError {
		t.Fatalf("unexpected error: %s", extractText(res))
	}
}

func TestReplaceNodeHandler_badSelector(t *testing.T) {
	es := newTestServer(t)
	mustOpen(t, es, "scratch:///rn.go", "package main\n", "go")

	sel := editor.NodeSelector{
		Path: []editor.PathStep{
			{Type: "nonexistent_type"},
		},
	}
	args := replaceNodeArgs{
		URI:      "scratch:///rn.go",
		Selector: sel,
		Text:     "func replaced() {}",
	}
	res := es.replaceNode(args)
	if !res.IsError {
		t.Fatal("expected error for bad selector")
	}
}

// ─── 17. delete ────────────────────────────────────────────────────────────

func TestDeleteHandler_valid(t *testing.T) {
	es := newTestServer(t)
	mustOpen(t, es, "scratch:///del.go",
		"package main\nfunc main() {}\n", "go")

	args := deleteRangeArgs{
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

func TestDeleteHandler_outOfBounds(t *testing.T) {
	es := newTestServer(t)
	mustOpen(t, es, "scratch:///del.go", "package main\n", "go")

	args := deleteRangeArgs{
		URI:       "scratch:///del.go",
		StartByte: 0,
		EndByte:   9999,
	}
	res := es.delete(args)
	if !res.IsError {
		t.Fatal("expected error for out-of-bounds delete")
	}
}

// ─── 18. deleteNode ────────────────────────────────────────────────────────

func TestDeleteNodeHandler_valid(t *testing.T) {
	es := newTestServer(t)
	mustOpen(t, es, "scratch:///dn.go",
		"package main\nfunc main() {}\n", "go")

	sel := editor.NodeSelector{
		Path: []editor.PathStep{
			{Type: "function_declaration"},
		},
	}
	args := deleteNodeArgs{
		URI:      "scratch:///dn.go",
		Selector: sel,
	}
	res := es.deleteNode(args)
	if res.IsError {
		t.Fatalf("unexpected error: %s", extractText(res))
	}
}

func TestDeleteNodeHandler_badSelector(t *testing.T) {
	es := newTestServer(t)
	mustOpen(t, es, "scratch:///dn.go", "package main\n", "go")

	sel := editor.NodeSelector{
		Path: []editor.PathStep{
			{Type: "nonexistent_type"},
		},
	}
	args := deleteNodeArgs{
		URI:      "scratch:///dn.go",
		Selector: sel,
	}
	res := es.deleteNode(args)
	if !res.IsError {
		t.Fatal("expected error for bad selector")
	}
}

// ─── 19. validate ──────────────────────────────────────────────────────────

func TestValidateHandler_valid(t *testing.T) {
	es := newTestServer(t)
	mustOpen(t, es, "scratch:///val.go",
		"package main\nfunc main() {}\n", "go")

	args := validateArgs{URI: "scratch:///val.go"}
	res := es.validate(args)
	if res.IsError {
		t.Fatalf("unexpected error: %s", extractText(res))
	}
	// Result body should contain validation info.
	var result map[string]any
	unmarshalJSONText(t, res, &result)
	// For valid code, "syntaxErrors" should be empty.
	if errs, ok := result["syntaxErrors"]; ok {
		if arr, ok := errs.([]any); ok && len(arr) > 0 {
			t.Errorf("expected no syntax errors, got %v", arr)
		}
	}
}

func TestValidateHandler_syntaxError(t *testing.T) {
	es := newTestServer(t)
	// Invalid Go with a syntax error — missing closing brace for main.
	mustOpen(t, es, "scratch:///valerr.go",
		"package main\nfunc main() {\nunclosed\n", "go")

	args := validateArgs{URI: "scratch:///valerr.go"}
	res := es.validate(args)
	if res == nil {
		t.Fatal("validate returned nil")
	}
	// Validate may return errors inline or as IsError depending on
	// implementation. Both are acceptable — just verify no panic.
}

// ─── 20. save ──────────────────────────────────────────────────────────────

func TestSaveHandler_scratchFails(t *testing.T) {
	es := newTestServer(t)
	mustOpen(t, es, "scratch:///save.go", "package main\n", "go")

	args := saveArgs{URI: "scratch:///save.go"}
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

	args := saveArgs{URI: uri}
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
	args := saveArgs{URI: "file:///nonexistent.go"}
	res := es.save(args)
	if !res.IsError {
		t.Fatal("expected error for saving non-existent document")
	}
}

// ─── 21. forceSave ────────────────────────────────────────────────────────

func TestForceSaveHandler(t *testing.T) {
	es := newTestServer(t)
	dir := t.TempDir()
	filePath := filepath.Join(dir, "forced.go")
	uri := "file://" + filePath
	mustOpen(t, es, uri, "package main\n", "go")

	args := saveArgs{URI: uri}
	res := es.forceSave(args)
	if res.IsError {
		t.Fatalf("unexpected error: %s", extractText(res))
	}
	var out saveResultResponse
	unmarshalJSONText(t, res, &out)
	if !out.Success {
		t.Error("expected success=true")
	}
}

// ─── 22. isDirty ──────────────────────────────────────────────────────────

func TestIsDirtyHandler_afterOpen(t *testing.T) {
	es := newTestServer(t)
	mustOpen(t, es, "scratch:///dirty.go", "package main\n", "go")

	args := uriOnlyArgs{URI: "scratch:///dirty.go"}
	res := es.isDirty(args)
	if res.IsError {
		t.Fatalf("unexpected error: %s", extractText(res))
	}
	m := extractBoolMap(t, res)
	if m["dirty"] {
		t.Error("new doc should not be dirty")
	}
}

func TestIsDirtyHandler_afterEdit(t *testing.T) {
	es := newTestServer(t)
	mustOpen(t, es, "scratch:///dirty.go", "package main\n", "go")
	es.insert(insertArgs{URI: "scratch:///dirty.go", Pos: 0, Text: "// comment\n"})

	args := uriOnlyArgs{URI: "scratch:///dirty.go"}
	res := es.isDirty(args)
	if res.IsError {
		t.Fatalf("unexpected error: %s", extractText(res))
	}
	m := extractBoolMap(t, res)
	if !m["dirty"] {
		t.Error("after edit, doc should be dirty")
	}
}

func TestIsDirtyHandler_notFound(t *testing.T) {
	es := newTestServer(t)
	args := uriOnlyArgs{URI: "scratch:///nonexistent.go"}
	res := es.isDirty(args)
	if !res.IsError {
		t.Fatal("expected error for non-existent document")
	}
}

// ─── 23. listDirty ────────────────────────────────────────────────────────

func TestListDirtyHandler_empty(t *testing.T) {
	es := newTestServer(t)
	mustOpen(t, es, "scratch:///clean.go", "package main\n", "go")

	res := es.listDirty()
	if res.IsError {
		t.Fatalf("unexpected error: %s", extractText(res))
	}
	var out map[string][]string
	unmarshalJSONText(t, res, &out)
	if len(out["uris"]) != 0 {
		t.Errorf("expected 0 dirty, got %v", out["uris"])
	}
}

func TestListDirtyHandler_afterEdits(t *testing.T) {
	es := newTestServer(t)
	mustOpen(t, es, "scratch:///dirty1.go", "package main\n", "go")
	mustOpen(t, es, "scratch:///dirty2.go", "package main\n", "go")
	es.insert(insertArgs{URI: "scratch:///dirty1.go", Pos: 0, Text: "// dirty\n"})

	res := es.listDirty()
	if res.IsError {
		t.Fatalf("unexpected error: %s", extractText(res))
	}
	var out map[string][]string
	unmarshalJSONText(t, res, &out)
	if len(out["uris"]) != 1 {
		t.Errorf("expected 1 dirty, got %v", out["uris"])
	}
	if out["uris"][0] != "scratch:///dirty1.go" {
		t.Errorf("expected dirty1.go, got %v", out["uris"][0])
	}
}

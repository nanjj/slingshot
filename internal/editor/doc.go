// Package editor implements a tree-sitter powered AI code editor core.
//
// It wraps tree-sitter's incremental parsing, syntax-aware editing, and
// structured code queries into a minimal API surface for AI tools.
//
// # Core types
//
//	Editor    entry point — manages document lifecycle and all operations
//	Document  parsed document — source + syntax tree + line index + metadata
//	LineIndex row↔column↔byte offset conversions
//	NodeSelector 4-position locator: Pos, Point, Range, or Path
//
// # Lifecycle (typical)
//
//	ed := editor.NewEditor(projectRoot)
//	ed.OpenDocument("file:///main.go", nil, "go") // loads disk, detects language
//	info, _ := ed.GetStructure("file:///main.go", -1, -1) // full AST as JSON
//	res, _ := ed.Insert("file:///main.go", 0, "package main\n")
//	ed.Save("file:///main.go")
//	ed.CloseDocument("file:///main.go")
//
// Editor is project-scoped singleton. Create one per project root.
// Auto-open: all View/Edit methods attempt auto-open if document not cached.
//
// # Error model
//
// Sentinel errors (ErrDocumentNotFound, ErrInvalidPosition, etc.) are
// returned directly. Checks and conflict results are returned as values
// (SaveResult.Conflict, EditResult.ParseErrors), not errors.
//
// # Dependencies
//
//	github.com/odvcencio/gotreesitter v0.20.2
//	  - Parser, Tree, Node, Language, Query
//	  - Rewriter (syntax-aware text manipulation)
//	  - InputEdit, BoundTree
//	github.com/odvcencio/gotreesitter/grammars
//	  - DetectLanguageByName, DetectLanguage, DetectLanguageByShebang
//
// # Design decisions
//
//   - Incremental parse with full fallback: ParseWith → on error → Parse.
//     Robust against edge cases in incremental re-parsing.
//   - LineIndex rebuilt on every edit (ApplyEdit). Simpler than
//     incremental line-index maintenance; fast enough for AI-editor scale.
//   - FieldName is not populated in NodeInfo. gotreesitter's Node API
//     does not expose child field names directly; Query captures remain
//     the recommended way to access field-named nodes.
//   - now() called directly (time.Now). Testing can inject a clock via
//     the Document struct if needed later.
//   - Atomic writes (tmpfile + rename). External modification detection
//     uses mtime comparison (not content hash).
//   - URI resolution respects projectRoot (from NewEditor). Relative
//     file:// URIs are resolved against the project root directory.
//     Non-file URIs (scratch://) bypass filesystem entirely.
//   - Auto-open: read/write methods open documents on demand if not
//     already cached. Explicit OpenDocument only needed for scratch://
//     URIs or to pass non-nil source for new files.
//
// # How to test
//
// Tests live in editor_test.go. Key patterns:
//
//	// 1. Standalone (no disk I/O) — create scratch documents
//	func TestGetStructure(t *testing.T) {
//	    ed := NewEditor("")
//	    err := ed.OpenDocument("scratch:///test.go", []byte("package main\n"), "go")
//	    require.NoError(t, err)
//	    info, err := ed.GetStructure("scratch:///test.go", -1, -1)
//	    assert.NoError(t, err)
//	    assert.Equal(t, "source_file", info.Type)
//	}
//
//	// 2. Disk-based — OpenDocument from actual files, test Save/Reload
//	//    Use t.TempDir() for isolated filesystem tests.
//
//	// 3. Error cases — test every sentinel error:
//	//    - OpenDocument with bogus language → ErrUnsupportedLanguage
//	//    - Read/write on unknown scratch URI (not opened) → ErrUnsupportedLanguage
//	//      (auto-open fails for scratch URIs)
//	//    - Edit with out-of-range position → ErrInvalidPosition
//	//    - Save on scratch:// URI → ErrNonFileURI
//	//    - OpenDocument(nil, nil, "") on non-existent file → ErrUnsupportedLanguage
//
//	// 4. Incremental parse — edit a document and verify the tree updates:
//	    res, _ := ed.Insert("scratch:///test.go", 0, "package main\n")
//	    assert.True(t, res.Success)
//	    info, _ := ed.GetStructure("scratch:///test.go", -1, -1)
//	    assert.Equal(t, "package main\n", info.Children[0].Text)
//
//	// 5. Conflict detection — modify file behind editor's back, then Save:
//	    // touch the file to change mtime, then call Save → Conflict=true
//
// # How to integrate as Layer 1 (MCP stdio server)
//
// The "slingshot editor" subcommand is a MCP stdio server presenting the
// editor API as JSON-RPC tools. Recommended command mapping:
//
//	MCP tool name          → editor method (all on Editor, uri first param)
//	────────────────────────────────────────────────────
//	open_document          → ed.OpenDocument(uri, source, language)
//	close_document         → ed.CloseDocument(uri)
//	get_structure          → ed.GetStructure(uri, maxDepth, maxChildren)
//	get_node               → ed.GetNode(uri, bytePos)
//	get_node_at_point      → ed.GetNodeAtPoint(uri, row, col)
//	query                  → ed.Query(uri, pattern)
//	get_text               → ed.GetText(uri, startByte, endByte)
//	get_line               → ed.GetLine(uri, line)
//	insert                 → ed.Insert(uri, pos, text)
//	insert_at_point        → ed.InsertAtPoint(uri, row, col, text)
//	replace                → ed.Replace(uri, startByte, endByte, text)
//	delete                 → ed.Delete(uri, startByte, endByte)
//	validate               → ed.Validate(uri)
//	save                   → ed.Save(uri)
//	save_as                → ed.SaveAs(uri, newPath)
//	force_save             → ed.ForceSave(uri)
//	reload                 → ed.Reload(uri)
//	is_dirty               → ed.IsDirty(uri)
//	dirty_documents        → ed.DirtyDocuments()
//
// MCP server skeleton:
//
//	// cmd/slingshot/editor_mcp.go
//	func runEditorMCP() {
//	    ed := editor.NewEditor("/path/to/project")
//	    mcp.NewServer("slingshot-editor", "1.0.0").
//	        Tool("open_document", openDocHandler(ed)).
//	        Tool("get_structure", getStructHandler(ed)).
//	        // ... one handler per tool above
//	        ServeStdio()
//	}
//
// Each handler pattern:
//
//	func openDocHandler(ed *editor.Editor) mcp.ToolHandler {
//	    return func(ctx context.Context, args json.RawMessage) (any, error) {
//	        var req struct { URI string; Source []byte; Language string }
//	        json.Unmarshal(args, &req)
//	        err := ed.OpenDocument(req.URI, req.Source, req.Language)
//	        if err != nil { return nil, err }
//	        return map[string]any{"ok": true}, nil
//	    }
//	}
//
// Concurrency: Editor methods are safe for concurrent use. Each Document
// has its own mutex. Editor map access uses sync.Map. The MCP server can
// handle requests in parallel.
//
// # Non-goals
//
//   - Not an LSP server. No diagnostics push, no completions.
//   - No dependency on fsnotify. External changes are detected on Save.
//   - No undo/redo. The AI owns the edit history externally.
package editor

// Package editor implements a tree-sitter powered AI code editor core.
//
// It wraps tree-sitter's incremental parsing, syntax-aware editing, and
// structured code queries into a minimal API surface for AI tools.
//
// This package lives under internal/code to centralize all document editing
// and write-through persistence operations alongside the code analysis
// facilities in internal/code/lsp (read-only) and internal/code/mcp (MCP handlers).
//
// # Key types
//
//	Editor    — document lifecycle, all edit operations, auto-save (write-through)
//	Document  — parsed document (source + syntax tree + line index + metadata)
//	LineIndex — row↔column↔byte offset conversions
//	NodeSelector — 4-position AST locator: Pos, Point, Range, Path
//
// # Write-through semantics
//
// Every edit operation (Insert, Replace, Delete, etc.) modifies the in-memory
// document incrementally, then immediately writes the result to disk via
// saveAfterEdit. This ensures AI edit operations are instantly persisted
// without requiring an explicit Save call.
//
// # Lifecycle (typical)
//
//	ed := editor.NewEditor(projectRoot)
//	ed.OpenDocument("file:///main.go", nil, "go") // loads disk, detects language
//	info, _ := ed.GetStructure("file:///main.go", -1, -1) // full AST as JSON
//	res, _ := ed.Insert("file:///main.go", 0, "package main\n")
//	// file saved to disk automatically
//	ed.CloseDocument("file:///main.go")
//
// # Error model
//
// Sentinel errors (ErrDocumentNotFound, ErrInvalidPosition, etc.) are
// returned directly. Conflict results are returned as values
// (SaveResult.Conflict), not errors.
//
// # Design decisions
//
//   - Incremental parse with full fallback: ParseWith → on error → Parse.
//   - LineIndex rebuilt on every edit (ApplyEdit). Simple and fast enough.
//   - Atomic writes (tmpfile + rename). External modification detection
//     uses mtime comparison (not content hash).
//   - URI resolution respects projectRoot. scratch:// URIs bypass filesystem.
//   - Auto-open: read/write methods open documents on demand if not cached.
package editor

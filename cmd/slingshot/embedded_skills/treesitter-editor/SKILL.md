---
name: treesitter-editor
description: tree-sitter AI Editor — 通过 MCP stdio 协议进行语法感知的代码查询与编辑。12 个 MCP 工具覆盖文档生命周期、AST 探索、结构化编辑、验证保存
keywords:
  - editor
  - mcp
  - code-editing
  - tree-sitter
  - ast
  - query
  - syntax
  - code-structure
  - lsp
  - source-code
  - mcp-server
author: Curie <curie@dscli.io>
---

# treesitter-editor — tree-sitter AI Code Editor

The **slingshot editor** is a tree-sitter powered AI code editing MCP server. It
exposes incremental parsing, syntax-aware navigation, and structured editing
operations via the Model Context Protocol (stdio transport).

**Architecture**: `EditorManager` manages per-project `Editor` instances, each
holding a `map[uri]*Document`. Documents wrap source text + tree-sitter `Tree` +
`LineIndex`. All operations are safe for concurrent use.

## Quick Reference

```bash
# Start the MCP server (one line)
slingshot editor serve --project-root /path/to/project --log-level info

# Or use environment variable
SLINGSHOT_PROJECT_ROOT=/path/to/project slingshot editor serve
```

Add this to your MCP client config:

```json
{
  "mcpServers": {
    "editor": {
      "command": "slingshot",
      "args": ["editor", "serve", "--project-root", "."],
      "transport": "stdio"
    }
  }
}
```

## Concepts

### URI System

Two URI schemes:

| Scheme | Example | Behavior |
|--------|---------|----------|
| `file://` | `file:///main.go` | Backed by real files. Relative paths resolve against project root. Save/Reload hit the filesystem. |
| `scratch://` | `scratch:///snippet.go` | Ephemeral in-memory documents. No disk I/O. Auto-opened on read if extension is recognized. |

**Auto-open**: All read/write methods auto-open documents from disk if not
already cached. Only call `editor_open_document` explicitly for:
- `scratch://` URIs (or to pass explicit source/language)
- New files not yet on disk
- Overriding auto-detected language

### Language Detection

Priority: explicit `language` parameter > file extension > shebang line (`#!`)
> falls back to `ErrUnsupportedLanguage`.

Supported languages: Go, Python, JavaScript, TypeScript, Rust, Ruby, Java, C,
C++, C#, HTML, CSS, JSON, YAML, Markdown, Bash, PHP, Swift, Kotlin, Lua, etc.

### NodeSelector (AST Path)

Four ways to identify a node:

| Method | Description | Example |
|--------|-------------|---------|
| **Pos** (`uint32`) | Byte offset from start of file | `{"pos": 15}` |
| **Point** (`[row, col]`) | Row and column (0-indexed) | `{"point": [2, 4]}` |
| **Range** (`[start, end]`) | Byte range | `{"range": [10, 25]}` |
| **Path** (`PathStep[]`) | Tree path traversal | `{"path": [{"type":"function_declaration"}, {"field":"body"}]}` |

PathStep fields:

| Field | Required | Description |
|-------|----------|-------------|
| `type` | conditional | Node type to match (e.g. `"function_declaration"`) |
| `field` | no | Field name (e.g. `"body"`, `"name"`, `"condition"`) |
| `childIndex` | no | 1-based child index |
| `namedOnly` | no | If true, skips anonymous nodes (default: false) |

## Tool Reference

### Document Lifecycle (2 tools)

| Tool | Description |
|------|-------------|
| `editor_open_document` | Open a document. Supports `file://` and `scratch://` URIs. If URI already open, it's closed first. Parameters: `uri` (required), `source` (optional), `language` (optional). |
| `editor_close_document` | Close a document, releasing tree-sitter resources. Unsaved changes are lost. Parameters: `uri` (required). |

### AST Exploration (4 tools)

These are your eyes into the syntax tree — use them to understand code structure
before editing.

| Tool | Description | Best For |
|------|-------------|----------|
| `editor_get_structure` | Full AST subtree as JSON. Supports `maxDepth` and `maxChildren` limits. | Getting the big picture of a function or file. |
| `editor_get_node` | Unified node access. Use `scope` parameter to select mode: `pos` (byte offset), `point` (row/col), `range` (byte range), `descendants` (ancestors at pos). | All AST node queries in one tool. |
| `editor_get_text` | Get source text. Use `by` parameter: `range` (default, `[startByte, endByte)`), `line` (0-indexed line). | Read specific section or a single line. |
| `editor_query` | Execute a tree-sitter S-expression query (.scm pattern). | Extract functions, variables, imports by pattern. |

> **Progressive disclosure for AST exploration**:
> 1. Start with `editor_get_structure` (low `maxDepth`) to see the top-level shape
> 2. Use `editor_get_node` with `scope=descendants` to understand context at a position
> 3. Use `editor_query` to extract specific patterns (function names, imports, etc.)
> 4. Use `editor_get_text` to read actual source text

### Edit Operations (3 tools)

All edits update the tree-sitter tree incrementally. Your edits are syntax-aware
— the AST stays valid after each operation.

| Tool | Parameters | Description |
|------|-----------|-------------|
| `editor_insert` | `position` (`pos`/`point`/`before`/`after`), position-specific params, `text` | Insert text at byte offset, row/col, or before/after an AST node. |
| `editor_replace` | `target` (`range`/`node`), target-specific params, `text` | Replace byte range or AST node content. |
| `editor_delete` | `target` (`range`/`node`), target-specific params | Delete byte range or AST node. |

> **When to use NodeSelector vs byte ranges**:
> - **NodeSelector**: Use when you know the AST structure — replace a function
>   body, delete a statement, insert before a specific declaration. More robust
>   because it works by tree position, not text position.
> - **Byte ranges**: Use when you know exact byte positions — replace a portion
>   of text, insert at a known offset, delete a specific range.

### Save & Validate (2 tools)

| Tool | Description |
|------|-------------|
| `editor_save` | Write to disk. By default detects external file modification (mtime check) and returns a conflict instead of overwriting. Set `force=true` to skip conflict detection. |
| `editor_validate` | Syntax check. Returns syntax errors, line ending style (LF/CRLF), trailing newline status. Set `includeDirty=true` to also get dirty status and list of all dirty documents. Read-only, no side effects. |

### Project Root (1 tool)

| Tool | Description |
|------|-------------|
| `editor_get_project_root` | Query the active project root path. Returns absolute path. |

### Tool Response Formats

**editor_open_document / editor_close_document** — `{"success": true}` or error.

**editor_get_structure** — `NodeInfo` tree:

```json
{
  "type": "source_file",
  "startByte": 0, "endByte": 48,
  "startPoint": [0, 0], "endPoint": [4, 0],
  "isNamed": true, "isError": false, "isMissing": false,
  "children": [
    {"type": "package_clause", "startByte": 0, "endByte": 12,
     "children": [
       {"type": "package", "startByte": 0, "endByte": 7,
        "isNamed": false, "text": "package"},
       {"type": "identifier", "startByte": 8, "endByte": 12,
        "isNamed": true, "text": "main"}
     ]}
  ]
}
```

**editor_get_node** — depends on `scope`:
- `pos`/`point`/`range`: single `NodeInfo` (no children)
- `descendants`: `[]NodeInfo` ordered innermost-to-outermost

**editor_query** — `[]QueryResult`:

```json
[
  {
    "pattern": 1,
    "captures": {
      "name": [{"type": "identifier", "text": "main", ...}],
      "func": [{"type": "function_declaration", ...}]
    }
  }
]
```

**editor_get_text** — `{"text": "..."}`, whether by range or line.

**editor_insert / editor_replace / editor_delete** — `EditResult`:

```json
{"success": true, "parseErrors": [], "byteDiff": 14}
```

**editor_validate** — combined validation + optional dirty info:

```json
{
  "valid": true,
  "syntaxErrors": [],
  "lineEnding": "LF",
  "trailingNewline": true,
  "dirty": false,
  "dirtyURIs": ["file:///main.go"]
}
```
When `includeDirty=false` (default), `dirty` and `dirtyURIs` are omitted.

**editor_save** — `SaveResult`:

```json
{"success": true, "bytes": 48, "conflict": false, "path": "/project/main.go"}
```

**editor_get_project_root** — `{"projectRoot": "/current/project"}`

## Workflow Patterns

### Pattern 1: Read → Understand → Edit → Validate → Save

```python
# 1. Open document (auto-opens on read, so get_structure is enough)
await client.call("editor_get_structure", {"uri": "file:///main.go", "maxDepth": 2})

# 2. Understand context at a specific position
await client.call("editor_get_node", {
    "uri": "file:///main.go", "scope": "descendants", "pos": 150
})

# 3. Query for specific patterns
await client.call("editor_query", {
    "uri": "file:///main.go",
    "pattern": "(function_declaration name: (identifier) @name)"
})

# 4. Read specific text by range
await client.call("editor_get_text", {
    "uri": "file:///main.go", "startByte": 0, "endByte": 100
})

# 5. Read a specific line
await client.call("editor_get_text", {
    "uri": "file:///main.go", "by": "line", "line": 10
})

# 6. Edit: insert a new function before an existing one
await client.call("editor_insert", {
    "uri": "file:///main.go",
    "position": "before",
    "selector": {"path": [{"type": "function_declaration", "field": "name",
                           "childIndex": 1}]},
    "text": "// helper function\nfunc helper() {}\n\n"
})

# 7. Validate syntax
await client.call("editor_validate", {"uri": "file:///main.go"})

# 8. Save
await client.call("editor_save", {"uri": "file:///main.go"})
```

### Pattern 2: Scratch Document — Test an Edit Before Applying

```python
# Open a scratch document with a snippet
await client.call("editor_open_document", {
    "uri": "scratch:///test.go",
    "source": "package main\nfunc main() { x := 1\n}\n",
    "language": "go"
})

# Explore its structure
struct = await client.call("editor_get_structure", {
    "uri": "scratch:///test.go", "maxDepth": 3
})

# Try a replacement (no disk I/O)
await client.call("editor_replace", {
    "uri": "scratch:///test.go",
    "target": "node",
    "selector": {"path": [{"type": "function_declaration"}]},
    "text": "func main() { x := 2\nprintln(x)\n}"
})

# Validate
await client.call("editor_validate", {"uri": "scratch:///test.go"})
```

### Pattern 3: Syntax Error Recovery

```python
# Edit blindly
await client.call("editor_insert", {
    "uri": "file:///buggy.go", "pos": 50,
    "text": "unclosed {"
})

# Check for errors
result = await client.call("editor_validate", {"uri": "file:///buggy.go"})
if result.get("syntaxErrors"):
    for err in result["syntaxErrors"]:
        # Find the error node and diagnose
        ctx = await client.call("editor_get_node", {
            "uri": "file:///buggy.go", "scope": "descendants", "pos": err.get("startByte", 0)
        })
```

### Pattern 4: Check Dirty Status Before Saving

```python
# Validate with dirty check
result = await client.call("editor_validate", {
    "uri": "file:///main.go", "includeDirty": True
})
if result.get("dirty"):
    # Has unsaved changes — save
    await client.call("editor_save", {"uri": "file:///main.go"})
```

## Common Node Selector Patterns

| Goal | Selector | Explanation |
|------|----------|-------------|
| First function | `{"path":[{"type":"function_declaration","childIndex":1}]}` | First child of type `function_declaration` |
| Second function | `{"path":[{"type":"function_declaration","childIndex":2}]}` | Second function declaration |
| Function body | `{"path":[{"type":"function_declaration"},{"field":"body"}]}` | `body` field of function |
| If condition | `{"path":[{"type":"if_statement"},{"field":"condition"}]}` | `condition` field of if |
| Function by name | `{"pos": 15}` or `{"point": [2, 4]}` | Any node at that position |
| Specific range | `{"range": [50, 120]}` | Smallest node covering bytes 50-119 |

## Common tree-sitter Query Patterns

```scheme
;; Find all function declarations
(function_declaration name: (identifier) @name)

;; Find all function calls
(call_expression function: (identifier) @func)

;; Find all comments
(comment) @comment

;; Find import paths
(import_path) @import

;; Find string literals
(string_literal) @string

;; Find variable declarations
(variable_declaration) @var

;; If with else
(if_statement
  condition: (_) @cond
  consequence: (_) @body
  alternative: (_) @else)
```

## Integration Guide

### MCP Client Integration

1. Add the server entry to your MCP config (see Quick Reference).
2. The server speaks stdio MCP protocol — compatible with any MCP client
   (Claude Desktop, Continue.dev, custom MCP hosts).
3. All logs go to stderr, JSON-RPC traffic on stdin/stdout.

### Architecture Notes

- **Each project gets its own Editor**: `EditorManager.SwitchTo(dir)` creates
  or retrieves an isolated editor per project root. Documents, dirty state,
  and AST trees are all per-project (managed internally — no user-visible push/pop).
- **Concurrency safe**: All Editor methods are safe for concurrent use. Each
  Document has its own mutex; the Editor's document map uses `sync.Map`.
- **Incremental parsing**: Every edit updates the tree-sitter tree via
  `Tree.Edit()` + `Parser.ParseWith()`. Falls back to full `Parse()` on
  error for robustness.
- **LineIndex**: Rebuilt on every edit (`ApplyEdit`). Simpler than incremental
  maintenance and fast enough for AI-editor scale.
- **Atomic writes**: `Save` uses tmpfile + rename. External modification
  detection uses mtime comparison.
- **No undo/redo**: The AI owns the edit history externally.

### API Stability

The tool set (12 tools) is stable. New tools may be added, but existing ones
will maintain backward compatibility. Response JSON schemas are stable.

## Environment

| Variable | Default | Description |
|----------|---------|-------------|
| `SLINGSHOT_PROJECT_ROOT` | `$PWD` | Project root for the initial editor instance |
| `--project-root` | `$PWD` | CLI flag, overrides env var |
| `--log-level` | `warn` | `debug`, `info`, `warn`, `error` |
| `--allow-external-paths` | `false` | Allow opening files outside project root |

## Dependencies

- **slingshot** (built-in, no extra installation)
- **gotreesitter** v0.20.2 (embedded in binary)

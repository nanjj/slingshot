# Phase 6 Plan — Closing the Gap with codebase-memory-mcp

## Current Status

**Slingshot Code Indexer**: ✅ All Phase 6 priorities completed
**CBM Indexer (same project)**: 2150 nodes, 9689 edges

## Gap Analysis (Updated 2026-06-24)

### Node types
| Node Kind | Slingshot | CBM | Status |
|-----------|-----------|-----|--------|
| function  | ✅ 843+   | 884 | ✅ |
| method    | ✅ 330+   | 348 | ✅ |
| file      | ✅ 171+   | 171 | ✅ |
| package   | ✅ 35+    | 35  | ✅ |
| class     | ✅ 179+   | 201 | ✅ P6b |
| route     | ❌        | 69  | Future |
| module    | ❌        | 171 | Future |
| variable  | ✅ 78+    | 78  | ✅ P6a |
| interface | ✅ 1+     | 1   | ✅ P6b |
| type      | ✅ 1+     | 1   | ✅ P6b |
| section   | ❌        | 157 | Future |

### Edge types
| Edge Type | Slingshot | CBM | Status |
|-----------|-----------|-----|--------|
| CALLS     | ✅ 1992+  | 2412 | ✅ |
| DEFINES   | ✅ 1757+  | 1841 | ✅ |
| CONTAINS  | ✅ 116+   | 171 | ✅ (includes struct→method) |
| IMPORTS   | ✅ 213+   | 217  | ✅ |
| IMPLEMENTS| ✅ 9+     | 9    | ✅ P6b |
| REFERENCES| ✅ (new)  | 1336 | ✅ P6a |
| TESTS     | ✅ 687+   | 843  | ✅ P6c |
| TESTS_FILE| ✅ 39+    | 40   | ✅ P6c |
| USAGE     | ❌        | N/A | N/A (CBM only) |
| HTTP_CALLS| ❌        | 207  | Future |
| WRITES    | ❌        | 756  | Future |

## ✅ Completed

### LSP Foundation (Zhang Heng)
- `ExtractSignature()`, `ExtractDocComment()`, `ExtractDeclName()` — full signature/doc extraction
- Package-qualified QNs (`main.Hello` not `Hello`)
- Node.Signature/Node.DocComment populated from AST

### CALLS SourceQN Fix (Zhang Heng)
- Fixed critical bug: CALLS SourceQN used `tag.Name` (raw) while nodes used `pkgQualified`
- Now consistent: `main.main` → `fmt.Println` edges link correctly
- Cross-file reference test validates multi-file project calls

### P6a: Variable REFERENCES (Curie + Zhang Heng)
- A1: Package-level var/const nodes via manual AST traversal ✅
- A2: REFERENCES edges from functions to variables ✅
- A4: Edge case tests (var/const-only files, full pipeline) ✅

### P6b: Class/Interface/Type Node Indexing (Curie) ✅
- `extractGoTypeDefs()`: manual AST traversal for Go type declarations
  - struct → kind "class", interface → "interface", type alias → "type"
  - Package-qualified QNs (`main.Greeter`) matching function/method convention
  - DocComment extraction from preceding comments
- **Fixed CONTAINS edges**: `extractGoMethodParent` now uses `parameter_list` (Go grammar) instead of non-existent `receiver` node type
- **Fixed `findReceiverType`**: returns type identifier (`Greeter`) not variable name (`g`)
- **Fixed IMPLEMENTS edges**: QNs now package-qualified (e.g., `main.FileWriter` → `main.Writer`)
- 8 new tests (6 unit + 2 integration), all 80+ tests pass
- Commit: 63d660e

### P6c: Test Relationship Tracking (Zhang Heng)
- Schema: TESTS/TESTS_FILE added to edge_type CHECK ✅
- Migration: Automatic schema migration for existing databases ✅
- LinkTests() post-processor: creates TESTS edges from CALLS edges ✅
- TESTS_FILE edges: test_file → source_file (foo_test.go → foo.go) ✅
- Bug fix: code_find_references dedup key includes EdgeType ✅
- MCP integration test validates TESTS edges via both SQL and API ✅

## Future
- P6d: Architecture Clusters (Leiden Community Detection)
- Stdlib stubs (Phase 3, deferred — 方案 C)
- Route/HTTP_CALLS tracking
- Semantic similarity search

## Decisions
- **2026-06-24**: P6a uses manual AST traversal (not Tagger API) since Go tags query doesn't emit var/const/struct tags
- **2026-06-24**: Use existing `REFERENCES` edge type (already in schema CHECK) instead of adding new edge type — zero schema migration needed
- **2026-06-24**: Package-level var/const only for A1; function-local vars deferred to later phase (scope complexity)
- **2026-06-24**: Reviewed CBM pass_definitions.c + pass_usages.c as reference — CBM uses registry-based resolution, slingshot will use in-memory varName→QN map for A2
- **2026-06-24**: CALLS SourceQN must match definition QN for cross-file traversal to work
- **2026-06-24**: TESTS/TESTS_FILE added to edge CHECK; schema migration handles existing DBs
- **2026-06-24**: LSP Phase 2/3 deferred (方案 C) — project-internal CALLS works, stdlib stubs later
- **2026-06-24**: `extractGoMethodParent`/`findReceiverType` had bug — Go grammar uses `parameter_list` not `receiver` for method receivers; returns variable name instead of type name. Fixed in P6b.

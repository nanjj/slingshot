# Phase 6 Plan — Closing the Gap with codebase-memory-mcp

## Current Status

**Slingshot Code Indexer**: 1208 nodes, 7241 edges
**CBM Indexer (same project)**: 2150 nodes, 9689 edges

## Gap Analysis

### Node types missing in Slingshot
| Node Kind | Slingshot | CBM |
|-----------|-----------|-----|
| function  | ✅ 823    | 884 |
| method    | ✅ 209    | 348 |
| file      | ✅ 156    | 171 |
| package   | ✅ 20     | 35  |
| **class** | ❌ 0      | 201 |
| **route** | ❌ 0      | 69  |
| **module**| ❌ 0      | 171 |
| **variable** | ❌ 0   | 78  |
| **interface** | ❌ 0  | 1   |
| **type**  | ❌ 0      | 1   |
| **section** | ❌ 0    | 157 |

### Edge types missing in Slingshot
| Edge Type | Slingshot | CBM |
|-----------|-----------|-----|
| CALLS     | ✅ 5867   | 2412 |
| DEFINES   | ✅ 1200   | 1841 |
| CONTAINS  | ✅ 156    | (145+26) |
| IMPORTS   | ✅ 17     | 217 |
| IMPLEMENTS| ✅ 1      | 9 |
| **USAGE** | ❌ 0      | 1336 |
| **SEMANTICALLY_RELATED** | ❌ 0 | 1840 |
| **SIMILAR_TO** | ❌ 0   | 347 |
| **TESTS** | ❌ 0      | 843 |
| **TESTS_FILE** | ❌ 0  | 40  |
| **HTTP_CALLS** | ❌ 0  | 207 |
| **WRITES** | ❌ 0     | 46  |
| **OVERRIDE** | ❌ 0   | 36  |
| **DEPENDS_ON** | ❌ 0  | 35  |
| **FILE_CHANGES_WITH** | ❌ 0 | 34 |
| **DEFINES_METHOD** | ❌ 0 | 273 |

### Feature gaps
| Feature | Slingshot | CBM |
|---------|-----------|-----|
| Code editing | ✅ edit | ❌ |
| AST query | ✅ query_ast | ❌ |
| Architecture clusters | ❌ | ✅ Leiden communities |
| Semantic similarity search | ❌ | ✅ SEMANTICALLY_RELATED |
| Graph enrichment (BM25) | ✅ search_code | ✅ |
| Pagination | ❌ | ✅ search_graph offset/limit |
| Test-aware tracing | ❌ | ✅ TESTS filtering |

## Phase 6 Priority Stack

### P6a: Variable REFERENCES + Local Scope (✅ Plan confirmed, ready to start)
**Key discovery (2026-06-24)**: Go grammar's tags query only emits `definition.function` and `definition.method`. No `definition.variable`, `definition.constant`, `definition.struct`, etc. Must use manual AST traversal.

**Schema**: `REFERENCES` edge type already exists in CHECK constraint. `code_find_references` MCP tool already registered — delegates to `store.GetReferences()`.

**Sub-phases**:

#### A1: Package-level var/const nodes
- Manual AST traversal for `var_declaration`/`const_declaration` in `indexFile()`
- Create `variable`/`constant` kind nodes
- Complexity computation skips (only relevant for function/method)

#### A2: REFERENCES edges from functions to variables
- Build `varName → QN` map from package-level var/const nodes per file
- In each function body walk, find identifiers matching known var names
- Filter out: call_expression targets (CALLS already covers this), keywords, package selectors
- Create `REFERENCES` edges: function → variable

#### A3: Enhance code_find_references tool
- Add optional `scope` parameter: `"local"` (REFERENCES only), `"global"` (all edges, default)
- Existing `GetReferences()` already works with any edge type — minimal changes needed

#### A4: ~20 tests
- Index test: var/const nodes indexed correctly
- Reference test: REFERENCES edges from functions to variables
- Find references test: scope="local" filters correctly

### P6b: Class/Interface/Type Node Indexing
- The Tagger API ALREADY provides `definition.class`, `definition.struct`, `definition.interface`, `definition.type` tags
- The `kindFromTag()` function already handles them
- **Root cause**: the Go grammar's tags query doesn't emit these in enough quantity
- Fix: manual AST extraction (similar to P6a approach) for struct/interface/type
- Add `IMPLEMENTS` edge properly (currently only 1 edge, CBM has 9)

### P6c: Test Relationship Tracking
- Write a post-indexing pass: MATCH test file + test function → link to tested function
- Add `TESTS` edge: test_function → function_under_test  
- Add `TESTS_FILE` edge: test_file → source_file
- Enable `include_tests=false` filtering in trace_path

### P6d: Architecture Clusters (Leiden Community Detection)
- Implement Leiden algorithm over weighted CALLS graph
- Expose clusters in get_architecture response
- CBM's implementation shows 12 clusters with 0.65-0.95 cohesion

## Execution Order
```
P6a (variable references) → P6b (class nodes) → P6c (test links) → P6d (clusters)
```
P6a and P6b can overlap since they touch different parts of the indexer.
P6c depends on P6b (needs proper function/class QNs for matching).
P6d is independent graphs computation.

## Test Target
- Existing: ~130 tests all green
- P6a target: +15-20 tests
- P6b target: +10-15 tests  
- P6c target: +10 tests
- P6d target: +15 tests
- **Final: ~170 tests green**

## Decisions
- **2026-06-24**: P6a uses manual AST traversal (not Tagger API) since Go tags query doesn't emit var/const/struct tags
- **2026-06-24**: Use existing `REFERENCES` edge type (already in schema CHECK) instead of adding new edge type — zero schema migration needed
- **2026-06-24**: Package-level var/const only for A1; function-local vars deferred to later phase (scope complexity)
- **2026-06-24**: Reviewed CBM pass_definitions.c + pass_usages.c as reference — CBM uses registry-based resolution, slingshot will use in-memory varName→QN map for A2
- **2026-06-24**: Waiting for Zhang Heng confirmation on using "REFERENCES" vs "USAGE" naming

## Progress Update — 2026-06-24 (17:06)

### ✅ Completed: LSP Foundation + CALLS SourceQN Fix (Zhang Heng)

**Primary Changes:**

1. **Signature Extraction** (`internal/code/lsp/lsp.go`):
   - `ExtractSignature()`, `ExtractDocComment()`, `ExtractDeclName()` — all with tests

2. **Indexer Enhancement** (`internal/code/base/indexer.go`):
   - Package-qualified QNs, Node.Signature/DocComment populated
   - **Fixed CALLS SourceQN bug**: `tag.Name` → `tagQNMap[tag.Name]` (e.g. `"Hello"` → `"main.Hello"`)
   - **Fixed REFERENCES SourceQN**: Same fix for variable reference extraction
   - Each CALLS edge now stores raw callee name in metadata JSON `{"callee":"fmt.Println"}`
   - Added `tagQNMap` (build during tag traversal, used for CALLS/REFERENCES SourceQN)

3. **Test fix** (`internal/code/mcp/server_test.go`):
   - `SourceQN == "main.main"` instead of `"main"`

### Impact
| Metric | Before | After |
|--------|--------|-------|
| Test suite | 457 tests pass | **457 tests pass** |
| CALLS SourceQN | `"Hello"` (raw) | `"main.Hello"` (qualified) |
| CALLS metadata | empty | `{"callee":"fmt.Println"}` |
| Graph traversal | Broken (QN mismatch) | **Works** (consistent QNs) |

### Division of Labor (Decided)
- **Zhang Heng → Phase 2b**: Post-index CALLS resolution — find dangling external calls, create placeholder nodes for stdlib/third-party packages
- **Curie → P6b**: Enhance Class/Interface/Type nodes with proper CONTAINS/IMPLEMENTS edges

## Next Steps
- Zhang Heng: `ResolveExternalCalls()` post-processor
- Curie: P6b — struct/class/interface hierarchy edges
- Phase 2: Resolve CALLS targets to definition nodes via import chain
  - Current: `fmt.Println` → CALLS target `fmt.Println` (no matching node)
  - Goal: Resolve through `import "fmt"` → find `fmt.Println` in stdlib stubs
- Phase 3: Add stdlib stub indexing (pre-generated data files)

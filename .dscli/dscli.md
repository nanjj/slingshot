# Slingshot code intelligence — 开发追踪

## 最新状态 (42df68b)

✅ **Phase 1+2 合并提交** (07d48d8) — 22 files, 3712 lines
✅ **Curie 集成测试** (7aefe4b) — 10/10 pass, SQL bug fix in graph.go
✅ **indexer_test.go + 4 bug fixes** (42df68b) — 32 tests, base 包从 0→32 tests

### 已修复 Bug
| Bug | 位置 | 影响 |
|-----|------|------|
| nodeText nil source | collectCyclomatic/checkRecursive/collectCalls | &&/|| 不计数、递归检测失败、调用提取为空 |
| collectCognitive 无限递归 | 对 if/for 递归自身 | 栈溢出 |
| countParams type mismatch | 用了 "parameters" 而非 "parameter_list" | Go 参数计数为 0 |
| checkRecursive 漏检方法调用 | field_identifier→selector_expression | Go 方法递归检测失败 |

### 测试覆盖
```
internal/code/base/  32 tests (was 0)
internal/code/mcp/   10 tests (unchanged)
internal/code/lsp/   0 tests  ← 仍需补
```

## 待办

- [ ] lsp 包测试（analyzer.go + lsp.go）
- [ ] 联调 — code serve CLI + codebase-memory-mcp MCP client
- [ ] 更多语言支持验证（Python/JS/Rust/Java 等）

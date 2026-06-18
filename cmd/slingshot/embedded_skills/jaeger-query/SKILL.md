---
name: jaeger-query
description: Jaeger Query HTTP API 技能 — 通过 slingshot jaeger 命令直接查询 Jaeger traces，无需 MCP Server，零依赖
keywords:
  - jaeger
  - tracing
  - query
  - api
  - distributed-tracing
  - opentelemetry
  - dapper
  - slingshot
  - progressive-disclosure
author: Curie <curie@dscli.io>
---

# jaeger-query — Jaeger Query HTTP API 技能

替代 Jaeger MCP Server（不可靠），直接调用 Jaeger Query HTTP API。
使用 `slingshot jaeger` 命令（内嵌在 slingshot CLI 中，无需 curl/jq/python3）。

## 命令

| 命令 | 功能 |
|------|------|
| `services` | 列出所有注册的服务 |
| `operations <service>` | 列出服务的所有操作名称 |
| `search <service> [--limit N] [--tags JSON]` | 搜索 traces |
| `trace <traceID>` | 获取完整的 trace 详情（原始 JSON） |
| `trace topology <traceID>` | 显示 trace 的服务拓扑（依赖关系图） |
| `trace critical-path <traceID>` | 显示 trace 的关键路径（最耗时链路） |
| `deps [--lookback DURATION]` | 获取全局服务依赖图 |

## 渐进式披露

对于复杂 trace（几十上百 span），原始 JSON 难以快速定位瓶颈。
`trace topology` 和 `trace critical-path` 提供两层抽象，逐级降低信息密度：

1. **拓扑视图** — 一眼看清哪些服务参与了请求、它们之间的依赖关系、各服务的 span 数/错误数/平均耗时
2. **关键路径** — 从根 span 到叶子，每层选耗时最长的子 span，形成端到端瓶颈链
3. **原始 trace** — 当上述视图不够时，再用 `trace <traceID>` 查原始 JSON 做深层次分析

这种设计避免一次返回大量原始数据，节省 token 和推理时间。

### 拓扑视图

```bash
slingshot jaeger trace topology <traceID>
slingshot jaeger trace topology <traceID> --json   # 机器可读格式
```

输出示例：
```
Services (3):
  frontend                 spans: 12    avg: 45ms   ops: [GET /api]
  backend                  spans: 8     avg: 120ms  ops: [processOrder, validate]
  database                 spans: 5     avg: 200ms  ops: [query]

Dependencies (2):
  frontend → backend   (8 spans)
  backend  → database  (5 spans)
```

### 关键路径

```bash
slingshot jaeger trace critical-path <traceID>
slingshot jaeger trace critical-path <traceID> --json
```

输出示例：
```
frontend.GET /api                                   [ 250ms]
└─ backend.processOrder                             [ 200ms]
    └─ database.query                               [ 150ms]

Total: 250ms
```

## 使用场景

### 查看服务
```bash
slingshot jaeger services
```

### 搜索 traces
```bash
slingshot jaeger search Dscli --limit 5
slingshot jaeger search Dscli --limit 10 --tags '{"error":"true"}'
```

### 深入分析
```bash
slingshot jaeger trace <traceID>
slingshot jaeger trace topology <traceID>
slingshot jaeger trace critical-path <traceID>
```

### 服务依赖拓扑
```bash
slingshot jaeger deps              # 默认 1h
slingshot jaeger deps --lookback 24h
```

## Jaeger API 注意事项

- **时间格式**: epoch 微秒 — `jaeger deps` 自动计算 `endEpoch` 和 `lookback`
- **`/api/dependencies`**: 使用 Go `time.Duration` 解析（1h, 24h, 3600s 等格式）
- **`/api/traces`**: 不带 start/end/lookback 即可返回最近 traces
- **`total` 字段**: Jaeger v2 始终返回 0（已知限制），实际数据在 `data[]` 中
- **tags**: `--tags` 标志自动 URL 编码

## 自定义 Jaeger 地址

```bash
# 环境变量方式
JAEGER_HOST=http://jaeger.example.com slingshot jaeger search Dscli

# 或 --host 标志
slingshot jaeger --host http://jaeger.example.com search Dscli
```

## 典型调试流程

1. 用户报告某个 tool call 慢
2. `slingshot jaeger operations Dscli` — 确认操作名
3. `slingshot jaeger search Dscli --limit 10` — 找最近的 traces
4. `slingshot jaeger trace topology <traceID>` — 看哪些服务参与、各服务耗时
5. `slingshot jaeger trace critical-path <traceID>` — 定位瓶颈链
6. `slingshot jaeger trace <traceID>` — 如有必要，查原始 JSON 做深层分析

## 依赖

- **slingshot**（内置，无需额外安装）
- Jaeger Query API（默认 `http://localhost:16686`）

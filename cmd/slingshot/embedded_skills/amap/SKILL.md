---
name: amap
description: 高德地图（Amap MCP）查询 — 通过 `slingshot amap` 子命令访问高德官方 MCP Server：POI 关键字搜索/周边搜索/POI 详情/地理编码/逆地理编码/驾车步行骑行公交路线规划/距离测量/IP 定位。Key 走本地 config 或 AMAP_KEY 环境变量。
keywords:
- amap
- 高德
- 地图
- poi
- geo
- 地理编码
- 路线规划
- driving
- transit
- 距离
- location
- maps
- mcp
author: JUN JIE NAN <nanjunjie@gmail.com>
---

# amap — 高德地图查询

slingshot 内置 `amap` 子命令，直接调用高德地图官方 MCP Server
（`https://mcp.amap.com/mcp`），无需浏览器、无需 dscli MCP 框架
（dscli 当前只支持 stdio/SSE，不支持 Streamable HTTP）。
所有结果以 pretty JSON 输出，可直接管道解析。

## 前置条件（Key 配置）

```bash
slingshot config set amap.key <key>   # 写入本地 config（推荐）
export AMAP_KEY=<key>                 # 或环境变量，优先级更高
```

- Key 只存在于本地 config/env，不写入代码、不硬编码
- 未配置时报错，并提示上述配置命令

## 子命令

| 命令 | 用途 |
|------|------|
| `slingshot amap search <关键词> [城市]` | POI 关键字搜索（citylimit=true 限城市内） |
| `slingshot amap around <关键词> <经度,纬度> [半径]` | 周边搜索（默认 3000 米） |
| `slingshot amap detail <poiId>` | POI ID 详情 |
| `slingshot amap geo <地址>` | 地理编码：地址 → 经纬度 |
| `slingshot amap regeo <经度,纬度>` | 逆地理编码：经纬度 → 结构化地址 |
| `slingshot amap driving <起点> <终点>` | 驾车路线（起终点自动 geo 转经纬度） |
| `slingshot amap walking <起点> <终点>` | 步行路线（≤100km） |
| `slingshot amap bicycling <起点> <终点>` | 骑行路线（≤500km） |
| `slingshot amap transit <起点> <终点> <起点城市> <终点城市>` | 公交/地铁/火车路线（跨城必传城市） |
| `slingshot amap distance <origins> <dest> [1驾车\|0直线\|3步行]` | 距离测量（默认 0 直线） |
| `slingshot amap ip [ip]` | IP 定位（缺省为请求方 IP） |

## 用法示例

```bash
slingshot amap search "冰煮羊" 呼和浩特
slingshot amap around "酒店" "111.772234,40.853779" 2000
slingshot amap geo "呼和浩特东站"
slingshot amap regeo "111.772234,40.853779"
slingshot amap driving "呼和浩特东站" "泽成冰煮羊(公务员店)"
slingshot amap transit 北京南 呼和浩特 北京 呼和浩特
slingshot amap distance "111.77,40.85" "111.61,40.72" 1
```

## MCP 工具映射

| slingshot 子命令 | MCP 工具 |
|------|------|
| search | maps_text_search |
| around | maps_around_search |
| detail | maps_search_detail |
| geo | maps_geo |
| regeo | maps_regeocode |
| driving | maps_direction_driving |
| walking | maps_direction_walking |
| bicycling | maps_direction_bicycling |
| transit | maps_direction_transit_integrated |
| distance | maps_distance |
| ip | maps_ip_location |

## 坐标格式

高德坐标：`经度,纬度`（GCJ-02），如 `111.772234,40.853779`。
不要与 WGS-84（GPS 原始值）混用。

## 已知行为（2026-08-23 实测）

- `search` 返回的 POI 条目**不含 location 字段**（MCP 端行为）；需要坐标时改用
  `around`（按坐标搜）或分两步 `geo`（按名称搜地址）
- `ip` 不传参数时 MCP 端可能返回空结果；需要定位时传具体 IP
- 搜索 POI 已强制 `citylimit=true`，结果限城市内，避免全国混排
- 无状态 HTTP 调用，每次独立请求，不保留会话

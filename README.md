# slingshot

![img](./slingshot.png)

微信公众号文章发布工作流 CLI。Markdown → 公众号草稿一条龙。

## 安装

```bash
go install github.com/nanjj/slingshot/cmd/slingshot@latest
# 或 make build
```

## 用法

```bash
# 1. 配置凭据
slingshot config set wechat.appid APPID
slingshot config set wechat.secret SECRET

# 高德地图（可选）：POI 搜索 / 地理编码 / 路线规划
slingshot config set amap.key KEY   # 或 export AMAP_KEY
slingshot amap search "冰煮羊" 呼和浩特

# 2. 转换 Markdown → 微信 HTML（可选 --upload 自动上传图片）
slingshot draft convert article.md --upload

# 3. 创建草稿
slingshot draft add article.html
```

### 子命令

```
slingshot
├── amap     search|around|detail|geo|regeo|driving|walking|bicycling|transit|distance|ip
├── draft    list|add|update|remove|show|convert <file>
├── config   list|show|get|set|unset
├── meterial add|list|remove|show
└── skill    list|install
```

### amap（高德地图）

通过高德官方 MCP 服务（Streamable HTTP，JSON-RPC）查询地图数据：

- `amap search <关键词> [城市]` — POI 关键字搜索（citylimit=true）
- `amap around <关键词> <经度,纬度> [半径]` — 周边搜索（默认 3000m）
- `amap detail <poiId>` — POI ID 详情
- `amap geo <地址>` / `amap regeo <经度,纬度>` — （逆）地理编码
- `amap driving|walking|bicycling <起点> <终点>` — 路线规划（起终点自动地理编码）
- `amap transit <起点> <终点> <起城> <终城>` — 公交路线（跨城必传城市）
- `amap distance <origins> <dest> <type>` — 距离测量（1驾车/0直线/3步行）
- `amap ip [ip]` — IP 定位

坐标使用高德 GCJ-02 坐标系，格式 `经度,纬度`（如 `111.772234,40.853779`）。
Key 通过 `slingshot config set amap.key <key>` 或环境变量 `AMAP_KEY` 提供，结果以 JSON 输出。

### sidecar YAML

同名 YAML 文件覆盖/补充 front matter（`article.md` → `article.yaml`）：

```yaml
title: 标题
author: 作者
thumb_media_id: cover.png  # 本地路径自动上传
digest: 摘要...
```

优先级: sidecar YAML > front matter > HTML `<meta>` > 默认值

### 诊断模式

加 `--explain` 查看参数解析结果，不实际执行。

## 构建

| 命令 | 说明 |
|------|------|
| `make build` | 编译 |
| `make test` | 测试 |
| `make install` | 安装 |
| `make release` | 交叉编译 |

## 许可证

Apache 2.0 © 2025 JUN JIE NAN

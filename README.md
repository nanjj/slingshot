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
├── skill    list|install
└── tikz     <in-file> <out-file> [--engine xe|tectonic|auto]
```

### tikz（TikZ/LaTeX → png/jpg/svg/pdf）

把 TikZ 片段渲染成图片或 PDF，输出格式由输出文件扩展名决定。输入可以是一个完整的
`tikzpicture` / `circuitikz` / `tikzcd` / `forest` 环境，也可以只是环境内部的命令
（自动补 `tikzpicture` 外壳）；`\usetikzlibrary` 与显式 `\usepackage` 会被提升到导言区。

```bash
slingshot tikz fig.tikz fig.png     # latexmk -xelatex → mutool 栅格化 150dpi
slingshot tikz fig.tikz fig.svg
slingshot tikz fig.tikz fig.pdf
slingshot tikz fig.tikz fig.png --engine tectonic   # 显式指定引擎
slingshot tikz fig.tikz fig.png --engine auto       # 优先 xe，缺失时回退 tectonic
```

`--engine` 默认 `xe`（latexmk `-xelatex`，TeX Live 2026 新版语法）；`tectonic` 使用内置的
2021 年旧包（旧版 tkz-euclide / circuitikz，自动启用兼容翻译）；`auto` 优先 xe、不可用时回退
tectonic。显式指定时探测失败直接报错，不静默回退。`pdf`（pdfLaTeX）与 `lua`（LuaLaTeX）
是保留取值，尚未实现。含中文（CJK）的片段通过 xeCJK 排版，字体可用 `TIKZ_CJK_FONT` 覆盖
（默认 Noto Sans CJK SC）。

单条外部命令（latexmk / tectonic / mutool / gs）默认 60 秒超时，超时杀掉整个进程组——死循环的
TikZ（例如旧语法 `\tkzDrawCircle[circum](A,B,C)` 把逗号参数喂给 pgfkeys）不会再挂死终端。
可用 `TIKZ_TIMEOUT`（Go duration 语法，如 `3m`）放宽。

依赖（缺省时命令会给出安装提示）：

- **Arch**: `sudo pacman -S texlive-binextra texlive-bin texlive-core`
  （CJK 再加 `texlive-langcjk`；也可用 `texlive-langchinese`）
- **Debian/Ubuntu**: `sudo apt install latexmk texlive-xetex texlive-latex-base`
  （CJK 再加 `texlive-lang-chinese`）
- **macOS/Windows**: 安装 TeX Live 或 MiKTeX（MiKTeX 会自动按需装包）；
  tectonic 也可从 <https://tectonic-typesetting.github.io> 安装

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

#!/usr/bin/env python3
"""
gen-zh-cn.py — Generate zh_CN .po entries for msgids missing from zh_CN.po.

Usage:
  python3 gen-zh-cn.py                          # generate with empty msgstr
  python3 gen-zh-cn.py --with-translations       # use built-in translation map
  python3 gen-zh-cn.py --translations data.json  # use external JSON map

The en_US.po is the authoritative source of all msgids. This script compares
zh_CN.po against en_US.po and generates missing entries.

An empty msgstr ("") is tolerated by i18n.G() — the program falls back to the
English msgid. This allows gradual translation: register first, translate later.
"""

import re, os, sys, json, argparse

def escape_po(s):
    """Escape string for .po file format."""
    s = s.replace('\\', '\\\\')
    s = s.replace('"', '\\"')
    s = s.replace('\n', '\\n')
    return s

def format_po_entry(msgid, msgstr):
    """Format a .po entry."""
    has_newline = '\n' in msgid
    if has_newline:
        msgid_parts = msgid.split('\n')
        msgstr_parts = msgstr.split('\n')
        # Remove trailing empty element from split if string ends with \n
        if msgid.endswith('\n') and msgid_parts and msgid_parts[-1] == '':
            msgid_parts = msgid_parts[:-1]
        if msgstr.endswith('\n') and msgstr_parts and msgstr_parts[-1] == '':
            msgstr_parts = msgstr_parts[:-1]
        out = ['msgid ""']
        for i, part in enumerate(msgid_parts):
            escaped = escape_po(part)
            if i < len(msgid_parts) - 1 or msgid.endswith('\n'):
                out.append('"' + escaped + '\\n"')
            else:
                out.append('"' + escaped + '"')
        out.append('msgstr ""')
        for i, part in enumerate(msgstr_parts):
            escaped = escape_po(part)
            if i < len(msgstr_parts) - 1 or msgstr.endswith('\n'):
                out.append('"' + escaped + '\\n"')
            else:
                out.append('"' + escaped + '"')
        return '\n'.join(out)
    else:
        return 'msgid "' + escape_po(msgid) + '"\nmsgstr "' + escape_po(msgstr) + '"'

def unescape_po(s):
    """Unescape .po escape sequences: \\n -> newline, \\\\ -> backslash, \\\" -> quote."""
    s = s.replace('\\n', '\n')
    s = s.replace('\\\\', '\\')
    s = s.replace('\\"', '"')
    return s

def load_po_msgids(filepath):
    """Load all msgids from a .po file, unescaping \\n sequences."""
    with open(filepath) as f:
        content = f.read()
    msgids = set()
    lines = content.split('\n')
    i = 0
    while i < len(lines):
        line = lines[i]
        if line.startswith('msgid ""'):
            i += 1
            parts = []
            while i < len(lines) and lines[i].startswith('"'):
                part = lines[i].strip()[1:-1]
                parts.append(part)
                i += 1
            full = ''.join(parts)
            if full:
                msgids.add(unescape_po(full))
        elif line.startswith('msgid "'):
            text = line[7:-1]
            i += 1
            while i < len(lines) and lines[i].startswith('"'):
                text += lines[i].strip()[1:-1]
                i += 1
            if text:
                msgids.add(unescape_po(text))
        else:
            i += 1
    return msgids

# Built-in translation map
TRANSLATIONS = {
    # Jaeger
    "Query Jaeger tracing data via HTTP API": "通过 HTTP API 查询 Jaeger 追踪数据",
    "Query Jaeger tracing data via the Jaeger Query HTTP API.\n\nReplaces the unreliable Jaeger MCP Server with direct HTTP calls —\nno truncation, no timeouts, no extra daemon.\n\nSubcommands:\n  services              List all registered services\n  operations  <service> List operations for a service\n  search      <service> Search traces for a service\n  trace       <traceID> Get full trace details\n  deps                  Get service dependency graph\n\nEnvironment:\n  JAEGER_HOST  Jaeger Query URL (default: http://localhost:16686)": "通过 Jaeger Query HTTP API 查询追踪数据。\n\n替代不可靠的 Jaeger MCP Server，直接使用 HTTP 调用 —\n无截断、无超时、无需额外守护进程。\n\n子命令：\n  services              列出所有已注册服务\n  operations  <service> 列出服务的操作\n  search      <service> 搜索服务的追踪记录\n  trace       <traceID> 获取完整追踪详情\n  deps                  获取服务依赖关系图\n\n环境变量：\n  JAEGER_HOST  Jaeger Query URL（默认: http://localhost:16686）",
    "Jaeger Query API host (default: http://localhost:16686, or $JAEGER_HOST)": "Jaeger Query API 主机地址（默认: http://localhost:16686，或 $JAEGER_HOST）",
    "request to Jaeger failed: %w": "请求 Jaeger 失败: %w",
    "reading response body: %w": "读取响应体失败: %w",
    "Jaeger API returned %d: %s": "Jaeger API 返回 %d: %s",
    "List all services registered with Jaeger.": "列出已在 Jaeger 注册的所有服务。",
    "List all operation names registered by a given service.": "列出指定服务注册的所有操作名称。",
    "Get full trace details": "获取完整追踪详情",
    "Get service dependency graph": "获取服务依赖关系图",
    "Get the service dependency graph from Jaeger.\n\nExamples:\n  slingshot jaeger deps\n  slingshot jaeger deps --lookback 24h": "从 Jaeger 获取服务依赖关系图。\n\n示例：\n  slingshot jaeger deps\n  slingshot jaeger deps --lookback 24h",
    "List all registered services": "列出所有已注册服务",
    "List operations for a service": "列出服务的操作",
    "Search traces for a service": "搜索服务的追踪记录",
    "Search recent traces for a service, with optional limit and tag filtering.\n\nExamples:\n  slingshot jaeger search Dscli\n  slingshot jaeger search Dscli --limit 5\n  slingshot jaeger search Dscli --tags '{\"error\":\"true\"}'": "搜索服务的最新追踪记录，支持可选的限制和标签过滤。\n\n示例：\n  slingshot jaeger search Dscli\n  slingshot jaeger search Dscli --limit 5\n  slingshot jaeger search Dscli --tags '{\"error\":\"true\"}'",
    "Retrieve the full trace details including all spans, process info, tags, and logs.": "检索完整追踪详情，包括所有 span、进程信息、标签和日志。",
    "Max trace count": "最大追踪记录数",
    "Filter tags as JSON, e.g. '{\"error\":\"true\"}'": "标签过滤 JSON，例如 '{\"error\":\"true\"}'",
    "Lookback duration (e.g. 1h, 24h, 3600s)": "回溯时长（例如 1h、24h、3600s）",
    "invalid lookback duration %q: %v": "无效的回溯时长 %q: %v",
    "Analyse a trace's spans to extract the service dependency graph:\nwhich services communicated with which, their operations, span counts,\nerror counts, and average durations.": "分析追踪的 span 以提取服务依赖关系图：\n哪些服务与哪些服务通信、它们的操作、span 数量、\n错误数量和平均耗时。",
    "Identify the critical path (longest chain) through the trace spans.\n\nWalks from the root span down to the leaf, at each level picking the\nchild with the longest duration. The result is the chain of spans most\nresponsible for end-to-end latency — the first thing to optimise.": "识别追踪 span 中的关键路径（最长链）。\n\n从根 span 向下遍历到叶子节点，每层选择持续时间最长的\n子 span。结果是最影响端到端延迟的 span 链——\n这是需要优先优化的对象。",
    "No root spans found in trace": "追踪中未找到根 span",
    "No spans in trace": "追踪中没有 span",
    "Output as JSON": "以 JSON 格式输出",
    "Retrieve the full trace details including all spans, process info, tags, and logs.\n\nSubcommands provide progressive-disclosure views:\n  topology      Show the service-dependency graph extracted from the trace\n  critical-path Identify the span chain most responsible for latency": "检索完整追踪详情，包括所有 span、进程信息、标签和日志。\n\n子命令提供渐进式披露视图：\n  topology      展示从追踪中提取的服务依赖关系图\n  critical-path 识别最影响延迟的 span 链",
    "Show trace critical path": "显示追踪关键路径",
    "Show trace service topology": "显示追踪服务拓扑",
    "parsing trace response: %w": "解析追踪响应失败: %w",
    "trace %s not found": "追踪 %s 未找到",


    # Draft
    "Create a new draft from .org/.md/.html file": "从 .org/.md/.html 文件创建新草稿",
    "Create a new WeChat draft from a file. Supports .org, .md, and .html.\nIf a .org or .md file is provided, it is auto-converted to HTML first\n(with image uploads and thumbnail resolution).\n\nThe title is extracted from the <title> tag in the HTML, falling back\nto the filename without extension. Use --title to override.\n\nThe cover media_id is required by WeChat. Use --thumb to specify, or\ninclude <meta name=\"thumb_media_id\" content=\"...\"> in the HTML.": "从文件创建新的微信公众号草稿。支持 .org、.md 和 .html。\n如果提供 .org 或 .md 文件，自动先转换为 HTML（含图片上传和缩略图处理）。\n\n标题从 HTML 的 <title> 标签提取，失败时回退到文件名（不含扩展名）。\n使用 --title 覆盖。\n\n封面 media_id 是微信必填项。使用 --thumb 指定，或在 HTML 中包含\n<meta name=\"thumb_media_id\" content=\"...\">。",
    "Article title (overrides auto-detection from HTML)": "文章标题（覆盖 HTML 自动检测）",
    "Cover image media_id (required; local image file paths are auto-uploaded to WeChat material)": "封面图 media_id（必填；本地图片路径会自动上传到微信素材库）",
    "Article": "文章",
    "Article index in multi-article draft (0-based)": "多文章草稿中的文章索引（从 0 开始）",
    "Draft created: %s\\n": "草稿已创建: %s\\n",
    "Draft updated: %s\\n": "草稿已更新: %s\\n",
    "Draft removed: %s\\n": "草稿已删除: %s\\n",
    "No drafts found.": "未找到草稿。",
    "Drafts (%d total):": "草稿（共 %d 个）：",
    "Draft has no articles.": "草稿没有文章。",
    "Remove (delete) a WeChat draft by ID or 1-based index from \"list\".": "通过 ID 或列表中索引（从 1 开始）删除微信公众号草稿。",
    "Show detailed information about a WeChat draft by ID or 1-based index from \"list\".": "通过 ID 或列表中索引（从 1 开始）显示微信公众号草稿的详细信息。",
    "Update an existing draft from .org/.md/.html file": "从 .org/.md/.html 文件更新已有草稿",
    "New article title (default: auto-detect from HTML)": "新文章标题（默认：从 HTML 自动检测）",
    "New cover image media_id (optional; also detected from <meta name=\"thumb_media_id\"> in HTML)": "新封面图 media_id（可选；也会从 HTML 中的 <meta name=\"thumb_media_id\"> 检测）",
    "cover media_id is required: use --thumb flag or add <meta name=\"thumb_media_id\" content=\"...\"> to HTML": "封面 media_id 是必填的：使用 --thumb 标志或在 HTML 中添加 <meta name=\"thumb_media_id\" content=\"...\">",
    "Update an existing WeChat draft with new content. Supports .org, .md,\nand .html files. If a .org or .md file is provided, it is auto-converted\nto HTML first (with image uploads and thumbnail resolution).\n\nThe draft is identified by (in priority order):\n  1. Sidecar YAML — if <file>.yaml/.yml exists with a \"media_id\" field\n  2. 1st draft — if no sidecar is found, defaults to the first draft in the list\n\nUsage:\n  slingshot draft update <file>               # auto-detect from sidecar YAML or first draft\n\nThe --index flag specifies which article in a multi-article draft to update\n(default 0, the first article). Use --thumb to update the cover image.": "更新已有的微信公众号草稿内容。支持 .org、.md 和 .html 文件。\n如果提供 .org 或 .md 文件，自动先转换为 HTML（含图片上传和缩略图处理）。\n\n草稿识别方式（按优先级）：\n  1. Sidecar YAML — 如果 <file>.yaml/.yml 存在且包含 \"media_id\" 字段\n  2. 第一个草稿 — 没有 sidecar 时默认使用列表中的第一个草稿\n\n用法：\n  slingshot draft update <file>               # 自动识别\n\n--index 标志指定更新多文章草稿中的第几篇（默认 0，即第一篇）。\n使用 --thumb 更新封面图。",
    "Warning: failed to save media_id to sidecar YAML: %v\\n": "警告: 保存 media_id 到 sidecar YAML 失败: %v\\n",

    # Convert
    "Convert Markdown or Org to WeChat public account HTML": "将 Markdown 或 Org 转换为微信公众号 HTML",
    "Auto-converting %s to WeChat HTML...\\n": "正在自动将 %s 转换为微信 HTML...\\n",
    "Converted SVG to PNG: %s -> %s\\n": "已转换 SVG 为 PNG: %s -> %s\\n",
    "Warning: SVG->PNG conversion failed for %s: %v; uploading original\\n": "警告: %s 的 SVG->PNG 转换失败: %v；正在上传原始文件\\n",
    "emacs not found: %w": "未找到 Emacs: %w",
    "emacs org-to-html conversion failed: %w\nstderr: %s": "Emacs org-to-html 转换失败: %w\nstderr: %s",
    "emacs org-to-html conversion timed out (30s)": "Emacs org-to-html 转换超时（30 秒）",
    "emacs did not produce expected HTML file: %s": "Emacs 未生成预期的 HTML 文件: %s",
    "Org-to-HTML conversion requires GNU Emacs (>= 26.1) with Org mode. ": "Org 到 HTML 转换需要 GNU Emacs（>= 26.1）并启用了 Org mode。",

    # Site
    "Add a new deployment site with key-value configuration pairs.\n\nThe first positional argument is the site name. Subsequent arguments are\nkey-value pairs for site configuration.\n\nRequired keys:\n  dir   Local site directory\n\nOptional keys:\n  rsync Rsync deployment command\n  type  Site type: page (default) or zine (auto-build before rsync, public/ output)\n\nExample:\n  slingshot site add mysite dir ~/mysite rsync 'rsync -avz --delete ./ user@host:/path' type zine": "添加新的部署站点，使用键值对配置。\n\n第一个位置参数是站点名称，后续参数是键值对配置。\n\n必填键：\n  dir   本地站点目录\n\n可选键：\n  rsync Rsync 部署命令\n  type  站点类型：page（默认）或 zine（rsync 前自动构建，输出 public/）\n\n示例：\n  slingshot site add mysite dir ~/mysite rsync 'rsync -avz --delete ./ user@host:/path' type zine",
    "Manage static site deployment targets with type-specific workflows.": "管理具有类型特定工作流的静态站点部署目标。",
    "Manage static site pages.": "管理静态站点页面。",
    "Add new pages from HTML files": "从 HTML 文件添加新页面",
    "Add new pages to a deployment site from HTML files.": "从 HTML 文件向部署站点添加新页面。",
    "Add new pages to a deployment site from HTML files.\n\nEach page name is derived from its HTML filename (without extension).\nA new subdirectory named <page-name> is created under the site's directory.\n\nImages referenced in the HTML (<img src=\"...\">) are copied from their\nsource locations into the page directory.\n\nThe site's index.html is regenerated once after all pages are processed.\n\nUse --rsync to automatically deploy the site after adding the pages.": "从 HTML 文件向部署站点添加新页面。\n\n每个页面名称从其 HTML 文件名衍生（不含扩展名）。\n在站点目录下创建名为 <page-name> 的子目录。\n\nHTML 中引用的图片（<img src=\"...\">）会从源位置复制到页面目录。\n\n所有页面处理完毕后，站点的 index.html 会重新生成。\n\n使用 --rsync 可在添加页面后自动部署站点。",
    "Update existing pages from HTML files": "从 HTML 文件更新已有页面",
    "Update existing pages in a deployment site with new content.": "用新内容更新部署站点中的已有页面。",
    "Update existing pages in a deployment site with new content.\n\nEach page name is derived from its HTML filename (without extension).\nThe page directory named <page-name> must already exist in the site directory.\n\nImages referenced in the HTML (<img src=\"...\">) are copied from their\nsource locations into the page directory.\n\nThe site's index.html is regenerated once after all pages are processed.\n\nUse --rsync to automatically deploy the site after the update.": "用新内容更新部署站点中的已有页面。\n\n每个页面名称从其 HTML 文件名衍生（不含扩展名）。\n名为 <page-name> 的页面目录必须已在站点目录中存在。\n\nHTML 中引用的图片（<img src=\"...\">）会从源位置复制到页面目录。\n\n所有页面处理完毕后，站点的 index.html 会重新生成。\n\n使用 --rsync 可在更新后自动部署站点。",
    "Remove a page": "删除页面",
    "Remove a page and its assets from a site.": "从站点中删除页面及其资源。",
    "List all pages in a deployment site.": "列出部署站点中的所有页面。",
    "List pages in a site": "列出站点页面",
    "Manage site pages": "管理站点页面",
    "No pages in site %q.": "站点 %q 中没有页面。",
    "Pages in %s (%d total):": "%s 中的页面（共 %d 个）：",
    "page %q not found in site %q": "站点 %2$s 中未找到页面 %1$q",
    "expected site and page arguments": "需要站点和页面参数",
    "invalid page name %q: use only letters, numbers, hyphens, and underscores": "无效的页面名称 %q：只能使用字母、数字、连字符和下划线",
    "invalid page name derived from filename": "从文件名衍生的页面名称无效",
    "Deploy site via rsync after adding/updating pages": "添加/更新页面后通过 rsync 部署站点",
    "Deploying via rsync for site:": "正在通过 rsync 部署站点：",
    "expected site name": "需要站点名称",
    "Run the configured rsync command to deploy site content to remote.\n\nFor zine-type sites, automatically runs 'zine build' before rsync\nand executes rsync from the 'public/' output directory.\nFor page-type sites, rsync runs directly from the site directory.": "运行配置的 rsync 命令将站点内容部署到远程。\n\n对于 zine 类型站点，自动在 rsync 前运行 'zine build'\n并从 'public/' 输出目录执行 rsync。\n对于 page 类型站点，直接从站点目录 rsync。",
    "Update a single configuration field on an existing site.\n\nArguments:\n  <name>  Site name\n  <key>   Configuration key (e.g. dir, rsync)\n  <value> New value\n\nExample:\n  slingshot site update mysite rsync 'rsync -avz --delete ./ user@host:/path'": "更新现有站点的单个配置字段。\n\n参数：\n  <name>  站点名称\n  <key>   配置键（如 dir、rsync）\n  <value> 新值\n\n示例：\n  slingshot site update mysite rsync 'rsync -avz --delete ./ user@host:/path'",
    "Optimize site CSS for responsive display": "优化站点 CSS 以适配响应式显示",
    "Force upgrade even if CSS is already optimized": "即使 CSS 已优化也强制升级",
    "CSS is already optimized.": "CSS 已经过优化。",
    "Upgrade the site's style.css to the latest responsive version.\n\nChecks for the responsive sentinel marker in the existing CSS. If the CSS\nis already optimized, no changes are made unless --force is specified.\n\nThis command can be run at any time — before rsync, after adding pages,\nor as a standalone optimization step.": "将站点的 style.css 升级到最新的响应式版本。\n\n检查现有 CSS 中的响应式哨兵标记。如果 CSS 已优化，除非指定 --force，\n否则不做更改。\n\n此命令可随时运行——rsync 之前、添加页面之后、或作为独立的优化步骤。",
    "image(s) copied": "图片已复制",
    "no files processed": "没有文件被处理",
    "expected at least one file argument": "至少需要一个文件参数",
    "file not found: %s": "文件未找到: %s",
    "Added": "已添加",
    "Updated": "已更新",
    "Updated:": "已更新：",
    "Removed:": "已删除：",

    # Config
    "Manage slingshot configuration file (~/.config/slingshot/config.yml).": "管理 slingshot 配置文件（~/.config/slingshot/config.yml）。",
    "Display the complete configuration in YAML format.": "以 YAML 格式显示完整配置。",
    "Show full configuration": "显示完整配置",
    "reading config": "读取配置",
    "Unset (delete) a config key": "删除配置键",
    "Unset (delete) a configuration key and save.": "删除配置键并保存。",
    "unset %s\\n": "已删除 %s\\n",

    # Material
    "Manage WeChat permanent materials": "管理微信永久素材",
    "Manage WeChat public account permanent materials.": "管理微信公众号永久素材。",
    "Upload a permanent material": "上传永久素材",
    "List permanent materials": "列出永久素材",
    "Remove a permanent material": "删除永久素材",
    "Remove (delete) a permanent material by its media ID.": "通过媒体 ID 删除永久素材。",
    "Show a permanent material's details": "显示永久素材详情",
    "Show detailed information about a permanent material by media ID.": "通过媒体 ID 显示永久素材的详细信息。",
    "No materials found.": "未找到素材。",
    "Materials (%s, %d total, showing %d):": "素材（%s，共 %d 个，显示 %d 个）：",
    "Material removed: %s\\n": "素材已删除: %s\\n",
    "Material type: image, video, voice": "素材类型：image、video、voice",
    "Material type: image, video, voice, news": "素材类型：image、video、voice、news",
    "Save to file (for image/voice material)": "保存到文件（用于图片/语音素材）",
    "Upload a file as a permanent material to WeChat material management.": "将文件作为永久素材上传到微信素材管理。",
    "invalid material type %q: must be one of: image, video, voice": "无效的素材类型 %q：必须是 image、video、voice 之一",
    "invalid material type %q: must be one of: image, video, voice, news": "无效的素材类型 %q：必须是 image、video、voice、news 之一",
    "Title (required for video material)": "标题（视频素材必填）",
    "--title is required for video material": "视频素材需要 --title 参数",
    "Introduction (optional, for video material)": "简介（可选，用于视频素材）",
    "Binary content, %d bytes. Use --output to save to file.\\n": "二进制内容，%d 字节。使用 --output 保存到文件。\\n",
    "Saved to %s (%d bytes)\\n": "已保存到 %s（%d 字节）\\n",
    "Uploaded %s -> media_id: %s\\n": "已上传 %s -> media_id: %s\\n",
    "Uploaded thumbnail -> media_id: %s\\n": "已上传缩略图 -> media_id: %s\\n",
    "Uploading %s as %s...\\n": "正在上传 %s（类型: %s）...\\n",
    "Uploading thumbnail %s...\\n": "正在上传缩略图 %s...\\n",
    "Using cached thumbnail %s -> %s\\n": "使用缓存缩略图 %s -> %s\\n",
    "Warning: thumbnail file %q not found, passing value as-is ": "警告: 缩略图文件 %q 未找到，直接传递原值 ",
    "Warning: thumbnail upload failed for %s: %v\\n": "警告: 缩略图 %s 上传失败: %v\\n",
    "Warning: checksum failed for thumbnail %s: %v\\n": "警告: 缩略图 %s 校验失败: %v\\n",
    "Warning: thumb_media_id %q looks like a local file.\\n": "警告: thumb_media_id %q 看起来是本地文件。\\n",
    "Cover media ID:": "封面媒体 ID：",
    "Download URL:": "下载 URL：",
    "Source URL:": "来源 URL：",

    # Skill
    "Manage built-in skills": "管理内置技能",
    "Manage built-in skills for AI agents.": "管理 AI 智能体的内置技能。",
    "List all built-in skills": "列出所有内置技能",
    "List all skills that are built into the slingshot binary.": "列出 slingshot 二进制文件中内置的所有技能。",
    "Install a built-in skill to the local skills directory": "安装内置技能到本地技能目录",
    "Install a built-in skill to the local skills directory.\n\nThe skill's SKILL.md is extracted from the binary and written to the\ndestination directory. By default the skill is installed under\n<project root>/.dscli/skills/<name>/, where <project root> is the nearest\nancestor of $PWD that contains a .git or .dscli directory (falling back\nto $PWD if none is found). Use --path to specify a custom directory.\n\nExamples:\n  slingshot skill install weixin\n  slingshot skill install weixin --path /home/user/.dscli/skills": "安装内置技能到本地技能目录。\n\n技能的 SKILL.md 从二进制文件中提取并写入目标目录。\n默认安装到 <project root>/.dscli/skills/<name>/，\n其中 <project root> 是 $PWD 最近的包含 .git 或 .dscli 目录的父目录。\n使用 --path 指定自定义目录。\n\n示例：\n  slingshot skill install weixin\n  slingshot skill install weixin --path /home/user/.dscli/skills",
    "Installation path (default: <project root>/.dscli/skills)": "安装路径（默认: <project root>/.dscli/skills）",
    "Installed skill %s to %s\\n": "已安装技能 %s 到 %s\\n",
    "No built-in skills found.": "未找到内置技能。",
    "expected a skill name argument": "需要技能名称参数",
    "skill %q has no SKILL.md": "技能 %q 没有 SKILL.md",
    "skill %q not found": "技能 %q 未找到",
    "Built-in skills:": "内置技能：",

    # Common
    "(no title)": "（无标题）",
    "Author:": "作者：",
    "Content preview:": "内容预览：",
    "Digest:": "摘要：",
    "ID:": "ID：",
    "Only fans comment:": "仅粉丝评论：",
    "Open comments:": "公开评论：",
    "Show cover:": "显示封面：",
    "Title:": "标题：",
    "URL": "URL",
    "URL:": "URL：",
    "Number of items to list (max 20)": "要列出的项目数（最多 20）",
    "Offset for pagination": "分页偏移量",
    "expected <file> argument": "需要 <file> 参数",
    "site %q has no directory configured": "站点 %q 未配置目录",
    "Using cached %s (by filename) -> %s\\n": "使用缓存 %s（按文件名）-> %s\\n",
    "Using existing %s\\n": "使用已有 %s\\n",
    "Warning: no cached CDN URL for %s; run 'convert --upload' first\\n": "警告: %s 没有缓存的 CDN URL；请先运行 'convert --upload'\\n",
}

def main():
    parser = argparse.ArgumentParser(description='Generate zh_CN .po entries')
    parser.add_argument('--with-translations', action='store_true',
                        help='Use built-in translation map (instead of empty msgstr)')
    parser.add_argument('--translations', type=str, default=None,
                        help='JSON file with translation map')
    args = parser.parse_args()

    repo = os.path.dirname(os.path.dirname(os.path.dirname(os.path.dirname(os.path.dirname(os.path.abspath(__file__))))))
    en_po = os.path.join(repo, 'internal/i18n/locales/en_US/slingshot.po')
    zh_po = os.path.join(repo, 'internal/i18n/locales/zh_CN/slingshot.po')

    # Load translation map
    tmap = {}
    if args.translations:
        with open(args.translations) as f:
            tmap = json.load(f)
    elif args.with_translations:
        tmap = TRANSLATIONS

    # Get all msgids from en_US (authoritative)
    en_ids = load_po_msgids(en_po)
    zh_ids = load_po_msgids(zh_po)

    missing = sorted(en_ids - zh_ids)
    if not missing:
        print("✓ No missing zh_CN entries found.")
        return

    print(f"Missing zh_CN entries: {len(missing)}", file=sys.stderr)

    # Generate .po entries
    output = []
    for msgid in missing:
        msgstr = tmap.get(msgid, "")
        output.append("")
        output.append(format_po_entry(msgid, msgstr))

    print("\n".join(output))

if __name__ == '__main__':
    main()

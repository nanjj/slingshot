# tikzassets — vendored LaTeX packages

这两份 `.sty` 是 slingshot `tikz` 子命令的 vendored 资产, 通过 `//go:embed
tikzassets/*.sty` 嵌进二进制, 只在 `tectonic` 后端编译前写入临时工作目录
(见 `tikz_assets.go`)。**verbatim upstream content, unmodified, do not edit
locally** —— 本目录只做原样复制, 任何改动都应在上游进行。

## 文件

| 文件 | 来源 | 版本 | 许可证 | SHA-256 |
|------|------|------|--------|---------|
| `figchild.sty` | TeX Live 2026 / CTAN (`tex/latex/figchild`) | v3.1.1, 2025/06/04 | LPPL 1.3c+ | `fc8aad2e1c9ffd2d0944b3cb8d8a5cde417094aac9878054881323386195c262` |
| `tikz-triminos.sty` | TeX Live 2026 / CTAN (`tex/latex/tikz-triminos`) | v0.1.0, 2025/01/20 | LPPL 1.3c+ | `465d62cc5d6bb4ec1d4ce687f1f9c822c2d938361102fd143e7e3abdbdb3bc9d` |

校验:

```
sha256sum cmd/slingshot/tikzassets/figchild.sty cmd/slingshot/tikzassets/tikz-triminos.sty
```

## 刷新策略

仅在确认上游版本后手动替换 (整文件覆盖), 然后更新上表的版本与 SHA-256,
并重跑 `TestFigchildLowercaseExceptions` / `TestTikzAssetsEmbedded` 等测试。
不要在本目录就地编辑。

## 为什么需要 vendoring

- **figchild**: tectonic 的 2021 bundle 内置的是 v1.1.1, 命令是 3 个必填参数的
  老 API (`\fcBell{a}{b}{c}`); 当前手册用法是裸命令或带可选 TikZ 选项
  (`\fcBell`, `\fcStar[scale=0.5]`), 在 bundle 版本上直接报 "Runaway argument"。
  把 v3.1.1 写进工作目录后 tectonic 优先从输入文件所在目录解析, 老 API 被覆盖。
- **tikz-triminos**: tectonic bundle 完全没有该包。同时 bundle 的 LaTeX 2021
  缺 `\fpeval` (l3kernel 2021-05 才加入), 由 `triminosFpevalShim` 提供别名。

`xe` (latexmk, TeX Live 2026) 后端使用系统 TeX Live 中的版本, 不走这条路径。

## 已知上游问题 (保持原样)

- `figchild.sty` 部分 `\newcommand` 的选项表以 `]]` 收尾 (bracket typo)。
  这是上游自带的写法, 与本仓库无关; vendored 内容保持原样, 不做本地修补。

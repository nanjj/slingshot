package main

import (
	"embed"
	"fmt"
	"os"
	"path/filepath"
	"slices"
)

// tikzAssets 嵌入 vendored 的 LaTeX 宏包 (cmd/slingshot/tikzassets)。
//
// 仅 tectonic profile 需要: bundle 的 figchild 是 v1.1.1 (2021, 3 必填参数的
// 老 API), 与当前 3.x 的裸命令/可选 TikZ 选项用法不兼容; bundle 也完全没有
// tikz-triminos。把新版 .sty 写进临时工作目录后, tectonic 从输入文件所在目录
// 解析 (与 input.tikz 同一机制, 已实测), 从而覆盖/补齐 bundle 版本。
// xe (latexmk) 用系统 TeX Live 的同名包, 不走这条路径。
//
//go:embed tikzassets/*.sty
var tikzAssets embed.FS

// vendoredPackageFiles 返回当前 profile 与包列表下需要写入工作目录的资产文件名。
// 只有 tectonic profile (legacyFigchild / legacyTriminos) 且确实命中了对应包时
// 才列出; 顺序固定 (figchild 在前), 与 tikzassets 目录的提交顺序一致。
func vendoredPackageFiles(p tikzProfile, pkgs []string) []string {
	var files []string
	if p.legacyFigchild && slices.Contains(pkgs, "figchild") {
		files = append(files, "figchild.sty")
	}
	if p.legacyTriminos && slices.Contains(pkgs, "tikz-triminos") {
		files = append(files, "tikz-triminos.sty")
	}
	return files
}

// writeVendoredPackages 把 vendoredPackageFiles 列出的资产写入 dir (编译工作目录)。
// dir 为输入文件所在目录, tectonic 优先从这里解析 \usepackage{<pkg>}。
func writeVendoredPackages(dir string, p tikzProfile, pkgs []string) error {
	for _, name := range vendoredPackageFiles(p, pkgs) {
		data, err := tikzAssets.ReadFile("tikzassets/" + name)
		if err != nil {
			return fmt.Errorf("reading embedded %s: %w", name, err)
		}
		if err := os.WriteFile(filepath.Join(dir, name), data, 0644); err != nil {
			return fmt.Errorf("writing %s: %w", name, err)
		}
	}
	return nil
}

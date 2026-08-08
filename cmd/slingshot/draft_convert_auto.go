package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/nanjj/slingshot/internal/i18n"
	"github.com/nanjj/slingshot/internal/mathrender"
	"github.com/nanjj/slingshot/internal/mdtowx"
)

// --- auto-convert for draft add/update ---

// ensureHTMLFile checks whether the given file is already HTML. If it's a
// source file (.org, .md) it ensures the corresponding .html exists and is
// up-to-date (newer than the source). If the .html is missing or stale, the
// source is auto-converted with the full upload pipeline.
//
// This lets callers use .org/.md files directly with "draft add" and
// "draft update" without a separate "draft convert --upload" step.
func ensureHTMLFile(cmd *cobra.Command, sourceFile string) (htmlPath string, err error) {
	ext := strings.ToLower(filepath.Ext(sourceFile))
	if ext == ".html" {
		return sourceFile, nil // already HTML, use as-is
	}

	// Compute expected .html path
	htmlPath = replaceExt(sourceFile, ".html")

	// Stat source to check existence
	srcInfo, err := os.Stat(sourceFile)
	if err != nil {
		return "", fmt.Errorf("stat source %q: %w", sourceFile, err)
	}

	// Check if .html exists and is fresh
	htmlInfo, htmlErr := os.Stat(htmlPath)
	if htmlErr == nil && !htmlInfo.ModTime().Before(srcInfo.ModTime()) {
		fmt.Fprintf(cmd.OutOrStdout(), i18n.G("Using existing %s\n"), htmlPath)
		return htmlPath, nil
	}

	// Auto-convert with full upload pipeline
	fmt.Fprintf(cmd.OutOrStdout(), i18n.G("Auto-converting %s to WeChat HTML...\n"), sourceFile)

	var result *mdtowx.Result
	switch ext {
	case ".org":
		result, err = mdtowx.ConvertOrgFile(sourceFile)
	default:
		result, err = mdtowx.ConvertFile(sourceFile)
	}
	if err != nil {
		return "", fmt.Errorf("conversion failed: %w", err)
	}

	// Render LaTeX formulas (auto mode: SVG via MathJax, PNG fallback).
	html := mdtowx.RemoveCJCSpace(result.HTML)
	html, err = mathrender.ProcessMath(html, filepath.Dir(sourceFile), mathrender.ModeAuto, cmd.ErrOrStderr())
	if err != nil {
		return "", fmt.Errorf("math processing failed: %w", err)
	}

	// Run the full upload pipeline (images + thumbnail)
	if err := runUploadPipeline(cmd, result, html, htmlPath, sourceFile); err != nil {
		return "", fmt.Errorf("upload pipeline failed: %w", err)
	}

	return htmlPath, nil
}

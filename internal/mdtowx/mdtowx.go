// Package mdtowx converts Markdown to WeChat-friendly HTML with inline styles.
//
// Strategy:
//  1. Parse: goldmark parses Markdown to AST
//  2. Style: AST walker adds style="..." attributes to supported node types
//  3. Render: goldmark's default HTML renderer + custom code block renderer
//     produce the final output with inline styles baked in
//
// Goldmark's default HTML renderer respects node attributes for most element
// types (heading, paragraph, blockquote, codespan, link, image, list, etc.)
// via RenderAttributes(). CodeBlock/FencedCodeBlock are the exceptions — they
// ignore attributes — so a dedicated custom renderer handles those.
package mdtowx

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"regexp"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/extension"
	extast "github.com/yuin/goldmark/extension/ast"
	"github.com/yuin/goldmark/renderer"
	"github.com/yuin/goldmark/text"
	"github.com/yuin/goldmark/util"
)

// --- Inline style definitions ---

// headingStyle returns the inline style for a heading level.
func headingStyle(level int) string {
	sizes := map[int]string{
		1: "font-size:1.8em;font-weight:bold;margin:1.2em 0 0.5em;line-height:1.4",
		2: "font-size:1.5em;font-weight:bold;margin:1.2em 0 0.5em;line-height:1.4",
		3: "font-size:1.3em;font-weight:bold;margin:1em 0 0.4em;line-height:1.4",
		4: "font-size:1.1em;font-weight:bold;margin:1em 0 0.4em;line-height:1.4",
	}
	if s, ok := sizes[level]; ok {
		return s
	}
	return sizes[4]
}

// style constants — each targets one AST node kind.
const (
	styleParagraph  = "margin:0.8em 0;line-height:1.8"
	styleBlockquote = "border-left:4px solid #d0d0d0;padding:10px 15px;" +
		"margin:1em 0;background:#f9f9f9"
	styleCodeBlockPre  = "background:#f5f5f5;padding:16px;border-radius:4px;overflow-x:auto"
	styleCodeBlockCode = "background:none;padding:0;" +
		"font-family:Consolas,'Courier New',monospace;font-size:0.9em"
	styleCodeSpan = "background:#f0f0f0;padding:2px 4px;border-radius:3px;" +
		"font-family:Consolas,'Courier New',monospace;font-size:0.9em"
	styleLink      = "color:#007bff;text-decoration:none"
	styleImage     = "max-width:100%;height:auto;display:block;margin:0.8em 0"
	styleTable     = "border-collapse:collapse;width:100%;margin:1em 0"
	styleTableCell = "border:1px solid #ddd;padding:8px;text-align:left"
	styleTH        = "background:#f5f5f5;font-weight:bold"
	styleList      = "margin:0.5em 0;padding-left:2em"
	styleListItem  = "margin:0.3em 0"
	styleHR        = "margin:1.5em 0;border:none;border-top:2px solid #eee"
	styleDel       = "text-decoration:line-through"
)

// --- AST walker: inject style attributes ---

// addInlineStyles walks the entire AST and sets style attributes on nodes that
// support them in goldmark's default HTML renderer.
func addInlineStyles(doc ast.Node) {
	_ = ast.Walk(doc, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		switch n.Kind() {
		case ast.KindHeading:
			n.SetAttributeString("style", headingStyle(n.(*ast.Heading).Level))

		case ast.KindParagraph:
			n.SetAttributeString("style", styleParagraph)

		case ast.KindBlockquote:
			n.SetAttributeString("style", styleBlockquote)

		case ast.KindCodeSpan:
			n.SetAttributeString("style", styleCodeSpan)

		case ast.KindLink, ast.KindAutoLink:
			n.SetAttributeString("style", styleLink)

		case ast.KindImage:
			n.SetAttributeString("style", styleImage)

		case ast.KindList:
			n.SetAttributeString("style", styleList)

		case ast.KindListItem:
			n.SetAttributeString("style", styleListItem)

		case ast.KindThematicBreak:
			n.SetAttributeString("style", styleHR)

		// Extension nodes (table, strikethrough)

		case extast.KindTable:
			n.SetAttributeString("style", styleTable)

		case extast.KindTableHeader:
			n.SetAttributeString("style", styleTH)

		case extast.KindTableCell:
			n.SetAttributeString("style", styleTableCell)

		case extast.KindStrikethrough:
			n.SetAttributeString("style", styleDel)
		}
		return ast.WalkContinue, nil
	})
}

// --- Custom code block renderer ---
//
// goldmark's html.Renderer does not inspect attributes for CodeBlock or
// FencedCodeBlock, so we provide our own renderer with inline styles baked in.

type codeBlockRenderer struct{}

func (r *codeBlockRenderer) RegisterFuncs(reg renderer.NodeRendererFuncRegisterer) {
	reg.Register(ast.KindCodeBlock, r.renderCodeBlock)
	reg.Register(ast.KindFencedCodeBlock, r.renderFencedCodeBlock)
}

func (r *codeBlockRenderer) renderCodeBlock(w util.BufWriter, source []byte, n ast.Node, entering bool) (ast.WalkStatus, error) {
	if entering {
		_, _ = w.WriteString(`<pre style="` + styleCodeBlockPre + `"><code style="` + styleCodeBlockCode + `">`)
		writeLines(w, source, n)
		return ast.WalkSkipChildren, nil
	}
	_, _ = w.WriteString("</code></pre>\n")
	return ast.WalkContinue, nil
}

func (r *codeBlockRenderer) renderFencedCodeBlock(w util.BufWriter, source []byte, n ast.Node, entering bool) (ast.WalkStatus, error) {
	if entering {
		_, _ = w.WriteString(`<pre style="` + styleCodeBlockPre + `"><code style="` + styleCodeBlockCode + `">`)
		writeLines(w, source, n)
		return ast.WalkSkipChildren, nil
	}
	_, _ = w.WriteString("</code></pre>\n")
	return ast.WalkContinue, nil
}

// writeLines writes the raw source segments of a code block.
func writeLines(w util.BufWriter, source []byte, n ast.Node) {
	lines := n.Lines()
	for i := 0; i < lines.Len(); i++ {
		line := lines.At(i)
		_, _ = w.Write(line.Value(source))
	}
}

// --- Goldmark instance ---

func newGoldmark() goldmark.Markdown {
	return goldmark.New(
		goldmark.WithExtensions(
			extension.Table,         // GFM tables
			extension.Strikethrough, // ~~strikethrough~~
		),
		goldmark.WithRendererOptions(
			// Custom code block renderer at LOW priority (100 < default's 1000).
			// In goldmark's reverse-iteration init, the lowest-priority renderer
			// registers LAST, so its registrations win over the default.
			renderer.WithNodeRenderers(
				util.Prioritized(&codeBlockRenderer{}, 100),
			),
			// Pass raw HTML through (needed for inline HTML in markdown).
			renderer.WithOption(renderer.OptionName("Unsafe"), true),
			// Use inline style (not align attribute) for table cell alignment.
			renderer.WithOption(
				renderer.OptionName("TableCellAlignMethod"),
				extension.TableCellAlignStyle,
			),
		),
	)
}

// --- Public API ---

// ConvertMarkdown converts Markdown source bytes to WeChat-friendly HTML.
//
// The returned HTML has all styles inline, making it suitable for pasting into
// the WeChat public platform editor.
func ConvertMarkdown(source []byte) ([]byte, error) {
	md := newGoldmark()

	// 1. Parse to AST.
	reader := text.NewReader(source)
	doc := md.Parser().Parse(reader)

	// 2. Inject inline style attributes.
	addInlineStyles(doc)

	// 3. Render to HTML.
	var buf bytes.Buffer
	if err := md.Renderer().Render(&buf, source, doc); err != nil {
		return nil, fmt.Errorf("rendering markdown: %w", err)
	}

	return buf.Bytes(), nil
}

// ConvertFile reads a Markdown file and returns WeChat-friendly HTML.
func ConvertFile(filePath string) ([]byte, error) {
	source, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("reading file %q: %w", filePath, err)
	}
	return ConvertMarkdown(source)
}

// ImageRef represents an image reference found in <img src="..."> in HTML.
type ImageRef struct {
	// Src is the original src attribute value as it appears in the HTML
	// (e.g., "images/photo.png" or "/absolute/path/img.jpg").
	Src string
	// AbsPath is the resolved absolute file path on disk.
	// Relative paths are resolved against the base directory.
	AbsPath string
}

var imgSrcRe = regexp.MustCompile(`<img[^>]+src="([^"]+)"`)

// isLocalPath checks if a path is a local file path (not a URL with scheme).
func isLocalPath(p string) bool {
	// Check for common URL schemes
	if len(p) > 8 && (p[:7] == "http://" || p[:8] == "https://") {
		return false
	}
	if len(p) > 3 && p[:4] == "ftp:" {
		return false
	}
	if len(p) > 3 && p[:4] == "data" {
		return false
	}
	return true
}

// ExtractImagePaths extracts local image references from HTML content.
//
// It parses <img src="..."> tags and returns only local file references
// (skipping http/https/data URIs). Relative paths are resolved against
// baseDir. Duplicates are preserved as-is (one entry per occurrence).
func ExtractImagePaths(html []byte, baseDir string) []ImageRef {
	matches := imgSrcRe.FindAllSubmatch(html, -1)
	var refs []ImageRef
	for _, m := range matches {
		src := string(m[1])
		if !isLocalPath(src) {
			continue
		}
		absPath := src
		if !filepath.IsAbs(src) {
			absPath = filepath.Join(baseDir, src)
		}
		absPath = filepath.Clean(absPath)
		refs = append(refs, ImageRef{Src: src, AbsPath: absPath})
	}
	return refs
}

// ReplaceImageURLs replaces image src attributes in HTML with new URLs.
//
// replacements maps from the original src value (as it appears in the HTML)
// to the new URL. Only src values present in the map are changed; unknown
// src values are left intact.
func ReplaceImageURLs(html []byte, replacements map[string]string) []byte {
	// Sort keys by length descending to avoid partial replacements
	// when one src is a prefix of another.
	var keys []string
	for k := range replacements {
		keys = append(keys, k)
	}
	for i := 0; i < len(keys); i++ {
		for j := i + 1; j < len(keys); j++ {
			if len(keys[j]) > len(keys[i]) {
				keys[i], keys[j] = keys[j], keys[i]
			}
		}
	}

	result := html
	for _, src := range keys {
		newURL := replacements[src]
		oldAttr := `src="` + src + `"`
		newAttr := `src="` + newURL + `"`
		result = bytes.ReplaceAll(result, []byte(oldAttr), []byte(newAttr))
	}
	return result
}

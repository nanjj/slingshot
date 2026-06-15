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
	"html"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/extension"
	extast "github.com/yuin/goldmark/extension/ast"
	"github.com/yuin/goldmark/renderer"
	"github.com/yuin/goldmark/text"
	"github.com/yuin/goldmark/util"
	"gopkg.in/yaml.v3"
)
// --- WeChat API limits ---

const (
	// MaxTitleLen is the maximum length (in runes) for an article title.
	MaxTitleLen = 64
	// MaxAuthorLen is the maximum length (in runes) for an article author.
	MaxAuthorLen = 8
)

// ValidateTitle checks that title does not exceed the WeChat API limit.
// Returns an error if the title is too long.
func ValidateTitle(title string) error {
	if n := len([]rune(title)); n > MaxTitleLen {
		return fmt.Errorf("title exceeds %d characters (got %d)", MaxTitleLen, n)
	}
	return nil
}

// SanitizeAuthor truncates author to the WeChat API limit.
// If the author exceeds MaxAuthorLen, all whitespace is stripped
// and the first MaxAuthorLen runes are kept.
func SanitizeAuthor(author string) string {
	if len([]rune(author)) <= MaxAuthorLen {
		return author
	}
	// Strip all whitespace
	noSpace := strings.Map(func(r rune) rune {
		if r == ' ' || r == '\t' || r == '\n' || r == '\r' {
			return -1 // drop
		}
		return r
	}, author)
	runes := []rune(noSpace)
	if len(runes) > MaxAuthorLen {
		runes = runes[:MaxAuthorLen]
	}
	return string(runes)
}

// --- Result type ---

// Result holds the converted HTML and metadata extracted from Markdown.
// Result holds the converted HTML and metadata extracted from Markdown.
type Result struct {
	// HTML is the body HTML with all styles inline, suitable for WeChat.
	HTML []byte
	// Title extracted from the YAML front matter (if present).
	Title string
	// Author extracted from the YAML front matter (if present).
	Author string
	// ThumbMediaID extracted from the YAML front matter (if present).
	ThumbMediaID string
	// Digest extracted from the YAML front matter (if present).
	Digest string
}

// frontMatter is the YAML structure expected at the top of a Markdown file.
// frontMatter is the YAML structure expected at the top of a Markdown file.
type frontMatter struct {
	Title        string `yaml:"title"`
	Author       string `yaml:"author"`
	ThumbMediaID string `yaml:"thumb_media_id"`
	Digest       string `yaml:"digest"`
}

// parseFrontMatter extracts YAML front matter from Markdown source.
// It returns the parsed metadata and the body content (with front matter stripped).
// If no valid front matter is found, returns empty metadata and the original source.
func parseFrontMatter(source []byte) (fm frontMatter, body []byte) {
	s := string(source)
	if !strings.HasPrefix(s, "---\n") && !strings.HasPrefix(s, "---\r\n") {
		return fm, source
	}

	// Find the closing ---
	rest := s[3:] // skip opening ---
	// Handle \r\n or \n after opening
	if strings.HasPrefix(rest, "\r\n") {
		rest = rest[2:]
	} else if strings.HasPrefix(rest, "\n") {
		rest = rest[1:]
	} else {
		return fm, source // malformed opening
	}

	endIdx := strings.Index(rest, "\n---")
	if endIdx < 0 {
		endIdx = strings.Index(rest, "\r\n---")
	}
	if endIdx < 0 {
		return fm, source // no closing --- found
	}

	yamlBlock := rest[:endIdx]
	bodyStart := endIdx + 4 // skip \n--- (handle either \n or \r\n)
	if len(rest) > bodyStart && rest[bodyStart-1] == '\r' {
		bodyStart = endIdx + 5 // skip \r\n---
	}

	// Parse YAML
	var parsed frontMatter
	if err := yaml.Unmarshal([]byte(yamlBlock), &parsed); err != nil {
		// Invalid YAML — skip front matter, treat source as plain markdown
		return fm, source
	}

	return parsed, []byte(rest[bodyStart:])
}

// --- Inline style definitions ---
//
// No font-family or color is specified for body text — WeChat's reader uses
// its own default fonts and handles theming (light/dark) automatically.
// This matches how manually crafted WeChat articles look.
// Functional colors are kept for code spans, links, and blockquotes.

// headingStyle returns the inline style for a heading level.
// Quaily groups: h1/h2 → centered h2; h3+ → left-aligned bold h3.
func headingStyle(level int) string {
	base := "line-height:1.5"
	switch {
	case level <= 2:
		return "text-align:center;" + base + ";font-size:140%;margin:80px 10px 40px 10px;font-weight:normal"
	default:
		return "text-align:left;" + base + ";font-size:120%;margin:40px 10px 20px 10px;font-weight:bold"
	}
}

// style constants — each targets one AST node kind.
// No font-family or body color is specified to match real WeChat articles.
const (
	styleParagraph  = "text-align:left;line-height:1.6;font-size:16px;margin:10px 10px"
	styleBlockquote = "text-align:left;color:rgb(91,91,91);line-height:1.5;font-size:16px" +
		";margin:20px 10px;padding:1px 0 1px 10px;background:rgba(158,158,158,0.1);border-left:3px solid rgb(158,158,158)"
	styleCodeSpan = "text-align:left;color:#ff3502;line-height:1.5;font-size:90%;" +
		"font-family:Operator Mono, Consolas, Monaco, Menlo, monospace;background:#f8f5ec;padding:3px 5px;border-radius:2px"
	styleLink      = "color:rgb(13,117,252);text-decoration:none"
	styleImage     = "max-width:100%;height:auto;display:block;margin:0.8em 0"
	styleTable     = "text-align:left;line-height:1.5;font-size:16px;border-collapse:collapse;margin:20px 0"
	styleTableHead = "text-align:left;line-height:1.5;font-size:16px;background:rgba(0,0,0,0.05)"
	styleTableCell = "text-align:left;line-height:1.5;font-size:80%;border:1px solid #dfdfdf;padding:4px 8px"
	styleHR        = "margin:1.5em 0;border:none;border-top:1px solid #eee"
	styleDel       = "text-decoration:line-through"

	// WeChat list styles — rendered as <ol>/<ul> + <li> with inline styles.
	// Native HTML list elements are used for better compatibility and nesting.
	styleListContainer = "text-align:left;line-height:1.5;font-size:16px;margin:10px 10px;margin-left:0;padding-left:1.5em;list-style:none"
	styleListItem      = "display:block;margin:0 8px"
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
			h := n.(*ast.Heading)
			origLevel := h.Level
			// Map heading level for WeChat: h1/h2 → h2 (centered), h3+ → h3 (left-aligned bold)
			switch {
			case origLevel <= 2:
				h.Level = 2
			default:
				h.Level = 3
			}
			n.SetAttributeString("style", headingStyle(origLevel))

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


		case ast.KindThematicBreak:
			n.SetAttributeString("style", styleHR)

		// Extension nodes (table, strikethrough)

		case extast.KindTable:
			n.SetAttributeString("style", styleTable)

		case extast.KindTableHeader:
			n.SetAttributeString("style", styleTableHead)

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

// WeChat code block format
//
// WeChat requires a specific HTML structure for code blocks:
//
//   <section class="code-snippet__fix code-snippet__LANG">
//     <pre class="code-snippet__LANG" data-lang="LANG">
//       <code><span class="code-snippet_outer">line1</span></code>
//       ...
//     </pre>
//   </section>
//
// The language suffix (LANG) comes from the fenced code block info string
// or defaults to "js". Each line gets its own <code><span> wrapper.

type codeBlockRenderer struct{}

func (r *codeBlockRenderer) RegisterFuncs(reg renderer.NodeRendererFuncRegisterer) {
	reg.Register(ast.KindCodeBlock, r.renderCodeBlock)
	reg.Register(ast.KindFencedCodeBlock, r.renderFencedCodeBlock)
}

// langSuffix sanitizes a language string for use as a CSS class suffix.
// The language is lowercased, and only safe ASCII CSS class characters
// (letters, digits, hyphens, underscores) are allowed. If the resulting
// string is empty, starts with a digit, or contains any unsafe characters,
// it falls back to "text".
// Iterating over bytes is correct here; any multi-byte rune will be rejected.
func langSuffix(lang string) string {

	lang = strings.ToLower(lang)
	if lang == "" {
		return "text"
	}
	if lang[0] >= '0' && lang[0] <= '9' {
		return "text"
	}
	for i := 0; i < len(lang); i++ {
		c := lang[i]
		if (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') || c == '-' || c == '_' {
			continue
		}
		return "text"
	}
	return lang
}


func (r *codeBlockRenderer) renderCodeBlock(w util.BufWriter, source []byte, n ast.Node, entering bool) (ast.WalkStatus, error) {
	if entering {
		writeWxCodeBlock(w, source, n, "")
		return ast.WalkSkipChildren, nil
	}
	_, _ = w.WriteString("</section>\n")
	return ast.WalkContinue, nil
}

func (r *codeBlockRenderer) renderFencedCodeBlock(w util.BufWriter, source []byte, n ast.Node, entering bool) (ast.WalkStatus, error) {
	if entering {
		fenced := n.(*ast.FencedCodeBlock)
		lang := string(fenced.Language(source))
		writeWxCodeBlock(w, source, n, lang)
		return ast.WalkSkipChildren, nil
	}
	_, _ = w.WriteString("</section>\n")
	return ast.WalkContinue, nil
}

// writeWxCodeBlock writes the WeChat code block structure for the given node.
// lang is the language from the fenced code block info string, or "" for
// indented code blocks.
func writeWxCodeBlock(w util.BufWriter, source []byte, n ast.Node, lang string) {
	lines := n.Lines()
	nLines := lines.Len()
	suffix := langSuffix(lang)

	// Always emit the full section structure, even for empty blocks (nLines == 0),
	// because the caller unconditionally writes </section> on exit.
	// An early return here would produce an orphaned closing tag.

	// <section class="code-snippet__fix code-snippet__LANG">
	_, _ = w.WriteString(`<section class="code-snippet__fix code-snippet__`)
	_, _ = w.WriteString(suffix)
	_, _ = w.WriteString(`">`)

	// <pre class="code-snippet__LANG" data-lang="LANG">
	_, _ = w.WriteString(`<pre class="code-snippet__`)
	_, _ = w.WriteString(suffix)
	_, _ = w.WriteString(`" data-lang="`)
	_, _ = w.WriteString(html.EscapeString(lang))
	_, _ = w.WriteString(`">`)

	// <code><span class="code-snippet_outer">line</span></code> per line
	for i := 0; i < nLines; i++ {
		line := lines.At(i)
		_, _ = w.WriteString("<code><span class=\"code-snippet_outer\">")
		// HTML-escape and strip trailing newline
		content := bytes.TrimRight(line.Value(source), "\n\r")
		_, _ = w.WriteString(html.EscapeString(string(content)))
		_, _ = w.WriteString("</span></code>\n")
	}

	_, _ = w.WriteString("</pre>")
	// </section> is written by the caller on exiting
}

// --- Custom list renderer ---
//
// WeChat does not support nested <ol>/<li>. All list items must be rendered
// as flat siblings inside a single container, with nesting depth expressed
// via padding-left on each item.

type listRenderer struct {
	depth     int // current list nesting depth (0 = outermost)
	inSection bool
}

func (r *listRenderer) RegisterFuncs(reg renderer.NodeRendererFuncRegisterer) {
	reg.Register(ast.KindList, r.renderList)
	reg.Register(ast.KindListItem, r.renderListItem)
	reg.Register(ast.KindParagraph, r.renderParagraph)
}

func (r *listRenderer) renderList(w util.BufWriter, source []byte, n ast.Node, entering bool) (ast.WalkStatus, error) {
	list := n.(*ast.List)
	if entering {
		if r.depth == 0 {
			// Outermost list — open container.
			if list.IsOrdered() {
				_, _ = w.WriteString(`<ol style="`)
			} else {
				_, _ = w.WriteString(`<ul style="`)
			}
			_, _ = w.WriteString(styleListContainer)
			_, _ = w.WriteString(`">` + "\n")
		} else {
			// Nested list — close the parent list item that's still open.
			if r.inSection {
				_, _ = w.WriteString(`</section></li>` + "\n")
				r.inSection = false
			}
		}
		r.depth++
	} else {
		r.depth--
		if r.depth == 0 {
			// Outermost list — close container.
			if list.IsOrdered() {
				_, _ = w.WriteString("</ol>\n")
			} else {
				_, _ = w.WriteString("</ul>\n")
			}
		}
	}
	return ast.WalkContinue, nil
}

func (r *listRenderer) renderListItem(w util.BufWriter, source []byte, n ast.Node, entering bool) (ast.WalkStatus, error) {
	if entering {
		itemDepth := r.depth - 1 // depth includes current list level
		// Each nesting level adds 1.5em of padding-left on top of
		// the container's base 1.5em padding.
		padding := float64(itemDepth) * 1.5
		_, _ = fmt.Fprintf(w, `<li style="display:block;margin:0 8px;padding-left:%.1fem"><section style="margin:0;padding:0">`, padding)
		r.inSection = true

		// Determine bullet character or number
		parent := n.Parent()
		if parent != nil && parent.Kind() == ast.KindList {
			list := parent.(*ast.List)
			if list.IsOrdered() {
				num := list.Start
				for s := n.PreviousSibling(); s != nil; s = s.PreviousSibling() {
					if s.Kind() == ast.KindListItem {
						num++
					}
				}
				_, _ = fmt.Fprintf(w, "%d. ", num)
			} else {
				_, _ = w.WriteString("• ")
			}
		} else {
			_, _ = w.WriteString("• ")
		}
	} else {
		if r.inSection {
			_, _ = w.WriteString(`</section></li>` + "\n")
			r.inSection = false
		}
	}
	return ast.WalkContinue, nil
}

// renderParagraph suppresses <p> tags when inside a list item (WeChat list
// items must not have paragraph wrappers). For paragraphs outside lists,
// it renders the default <p style="..."> as usual.
func (r *listRenderer) renderParagraph(w util.BufWriter, source []byte, n ast.Node, entering bool) (ast.WalkStatus, error) {
	// Check if this paragraph is inside a list item
	for p := n.Parent(); p != nil; p = p.Parent() {
		if p.Kind() == ast.KindListItem {
			return ast.WalkContinue, nil
		}
	}
	// Not inside a list item — render as normal <p>
	if !n.HasChildren() {
		return ast.WalkSkipChildren, nil
	}
	if entering {
		_, _ = w.WriteString("<p")
		writeNodeAttrs(w, n)
		_ = w.WriteByte('>')
	} else {
		_, _ = w.WriteString("</p>\n")
	}
	return ast.WalkContinue, nil
}

// writeNodeAttrs writes all attributes of a node as HTML attributes.
// It is equivalent to goldmark's internal renderAttributes but accessible
// from custom renderers.
func writeNodeAttrs(w util.BufWriter, n ast.Node) {
	for _, attr := range n.Attributes() {
		_ = w.WriteByte(' ')
		_, _ = w.Write(attr.Name)
		_, _ = w.WriteString(`="`)
		switch v := attr.Value.(type) {
		case []byte:
			_, _ = w.Write(v)
		case string:
			_, _ = w.WriteString(v)
		default:
			_, _ = fmt.Fprint(w, v)
		}
		_ = w.WriteByte('"')
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
				util.Prioritized(&listRenderer{}, 100),
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

// replaceMathSymbols replaces >= with ≥ and <= with ≤ in Markdown text,
// skipping fenced code blocks. This avoids relying on HTML named entities
// (&ge;, &le;) which WeChat's XML-based pipeline may not decode correctly.
func replaceMathSymbols(source []byte) []byte {
	lines := bytes.Split(source, []byte("\n"))
	var result [][]byte
	inFence := false
	for _, line := range lines {
		trimmed := bytes.TrimSpace(line)
		if bytes.HasPrefix(trimmed, []byte("```")) {
			inFence = !inFence
			result = append(result, line)
			continue
		}
		if inFence {
			result = append(result, line)
			continue
		}
		// Outside fenced code: replace >= and <=
		line = bytes.ReplaceAll(line, []byte(">="), []byte("≥"))
		line = bytes.ReplaceAll(line, []byte("<="), []byte("≤"))
		result = append(result, line)
	}
	return bytes.Join(result, []byte("\n"))
}

// --- Public API ---
// ConvertMarkdown converts Markdown source bytes to WeChat-friendly HTML.
//
// The returned HTML has all styles inline, making it suitable for pasting into
// the WeChat public platform editor. If the source contains YAML front matter
// (delimited by ---), the title and author are extracted and returned in the
// Result struct.
func ConvertMarkdown(source []byte) (*Result, error) {
	// 1. Parse front matter (if present)
	fm, body := parseFrontMatter(source)

	// 2. Validate title and sanitize author.
	if err := ValidateTitle(fm.Title); err != nil {
		return nil, err
	}
	author := SanitizeAuthor(fm.Author)

	// 3. Pre-process: replace >= with ≥ and <= with ≤ outside code blocks.
	//    WeChat's XML pipeline may not decode named HTML entities correctly,
	//    so we use Unicode characters directly.
	body = replaceMathSymbols(body)

	md := newGoldmark()

	// 4. Parse to AST.
	reader := text.NewReader(body)
	doc := md.Parser().Parse(reader)

	// 5. Inject inline style attributes.
	addInlineStyles(doc)

	// 6. Render to HTML.
	var buf bytes.Buffer
	if err := md.Renderer().Render(&buf, body, doc); err != nil {
		return nil, fmt.Errorf("rendering markdown: %w", err)
	}

	return &Result{
		HTML:         buf.Bytes(),
		Title:        fm.Title,
		Author:       author,
		ThumbMediaID: fm.ThumbMediaID,
		Digest:       fm.Digest,
	}, nil
}

// ConvertFile reads a Markdown file and returns WeChat-friendly HTML with
// metadata extracted from YAML front matter and/or a sidecar YAML file.
//
// Sidecar YAML: if a file named <filename>.yaml (or .yml) exists alongside
// the markdown file, its fields override any values found in the markdown's
// own YAML front matter. This allows keeping the markdown content clean
// while the metadata lives in a separate file.
func ConvertFile(filePath string) (*Result, error) {
	source, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("reading file %q: %w", filePath, err)
	}

	result, err := ConvertMarkdown(source)
	if err != nil {
		return nil, err
	}

	// Try sidecar YAML: <filename>.yaml, fallback <filename>.yml
	meta, ok := readSidecarYAML(filePath)
	if !ok {
		return result, nil
	}
	if meta.Title != "" {
		if err := ValidateTitle(meta.Title); err != nil {
			return nil, err
		}
		result.Title = meta.Title
	}
	if meta.Author != "" {
		result.Author = SanitizeAuthor(meta.Author)
	}
	if meta.ThumbMediaID != "" {
		result.ThumbMediaID = meta.ThumbMediaID
	}
	if meta.Digest != "" {
		result.Digest = meta.Digest
	}
	return result, nil
}

// readSidecarYAML attempts to read a sidecar YAML file for the given
// markdown path. It looks for <filename>.yaml first, then <filename>.yml.
// Returns the parsed metadata and true on success.
func readSidecarYAML(mdPath string) (frontMatter, bool) {
	base := mdPath[:len(mdPath)-len(filepath.Ext(mdPath))]
	for _, ext := range []string{".yaml", ".yml"} {
		yamlPath := base + ext
		data, err := os.ReadFile(yamlPath)
		if err != nil {
			continue
		}
		var fm frontMatter
		if err := yaml.Unmarshal(data, &fm); err != nil {
			continue
		}
		return fm, true
	}
	return frontMatter{}, false
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

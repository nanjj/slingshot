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
type Result struct {
	// HTML is the body HTML with all styles inline, suitable for WeChat.
	HTML []byte
	// Title extracted from the YAML front matter (if present).
	Title string
	// Author extracted from the YAML front matter (if present).
	Author string
	// ThumbMediaID extracted from the YAML front matter (if present).
	ThumbMediaID string
}

// frontMatter is the YAML structure expected at the top of a Markdown file.
type frontMatter struct {
	Title        string `yaml:"title"`
	Author       string `yaml:"author"`
	ThumbMediaID string `yaml:"thumb_media_id"`
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
	styleHR        = "margin:1.5em 0;border:none;border-top:2px solid #eee"
	styleDel       = "text-decoration:line-through"

	// WeChat list styles — WeChat does not support <ul>/<ol>/<li>, so lists
	// are rendered as <p> with <span> items using bullet/number characters.
	styleListContainer = "margin:20px 10px;margin-left:0;padding-left:20px"
	styleListItemWx    = "text-indent:-20px;display:block;margin:10px 10px"
	styleListBullet    = "margin-right: 10px;"
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

// --- Custom list renderer ---
//
// WeChat does not support <ul>/<ol>/<li> tags. Lists must be rendered as
// <p> with <span> items using bullet or number characters. This renderer
// replaces the default goldmark list rendering for WeChat compatibility.

type listRenderer struct{}

func (r *listRenderer) RegisterFuncs(reg renderer.NodeRendererFuncRegisterer) {
	reg.Register(ast.KindList, r.renderList)
	reg.Register(ast.KindListItem, r.renderListItem)
	reg.Register(ast.KindParagraph, r.renderParagraph)
}

func (r *listRenderer) renderList(w util.BufWriter, source []byte, n ast.Node, entering bool) (ast.WalkStatus, error) {
	if entering {
		_, _ = w.WriteString(`<p style="`)
		_, _ = w.WriteString(styleListContainer)
		list := n.(*ast.List)
		if !list.IsOrdered() {
			_, _ = w.WriteString(`;list-style:circle`)
		}
		_, _ = w.WriteString(`">` + "\n")
	} else {
		_, _ = w.WriteString("</p>\n")
	}
	return ast.WalkContinue, nil
}

func (r *listRenderer) renderListItem(w util.BufWriter, source []byte, n ast.Node, entering bool) (ast.WalkStatus, error) {
	if entering {
		_, _ = w.WriteString(`<span style="`)
		_, _ = w.WriteString(styleListItemWx)
		_, _ = w.WriteString(`"><span style="`)
		_, _ = w.WriteString(styleListBullet)
		_, _ = w.WriteString(`">`)
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
				_, _ = fmt.Fprintf(w, "%d.", num)
			} else {
				_, _ = w.WriteString("•")
			}
		} else {
			_, _ = w.WriteString("•")
		}
		_, _ = w.WriteString("</span>")
	} else {
		_, _ = w.WriteString("</span>\n")
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

	md := newGoldmark()

	// 3. Parse to AST.
	reader := text.NewReader(body)
	doc := md.Parser().Parse(reader)

	// 4. Inject inline style attributes.
	addInlineStyles(doc)

	// 5. Render to HTML.
	var buf bytes.Buffer
	if err := md.Renderer().Render(&buf, body, doc); err != nil {
		return nil, fmt.Errorf("rendering markdown: %w", err)
	}

	return &Result{
		HTML:         buf.Bytes(),
		Title:        fm.Title,
		Author:       author,
		ThumbMediaID: fm.ThumbMediaID,
	}, nil

}
// ConvertFile reads a Markdown file and returns WeChat-friendly HTML with
// metadata extracted from YAML front matter and/or a sidecar YAML file.
//
// Sidecar YAML: if a file named <filename>.yaml (or .yml) exists alongside
// the markdown file, its title/author/thumb_media_id fields override any
// values found in the markdown's own YAML front matter. This allows keeping
// the markdown content clean while the metadata lives in a separate file.
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

// Package highlight implements tree-sitter-based syntax highlighting for code
// blocks in HTML pages. It produces inline styles (color:#xxx) instead of CSS
// classes, making the output usable in Weixin drafts, RSS feeds, and any HTML
// context where external stylesheets might be stripped.
package highlight

import (
	"bytes"
	"fmt"
	"html"
	"os"
	"path/filepath"
	"strings"

	"github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
)

// captureColors maps tree-sitter capture names to inline CSS color values.
// Only exact matches are used — no hierarchical fallback. Unrecognized captures
// are silently ignored (no span wrapper emitted).
var captureColors = map[string]string{
	// Keywords
	"keyword":          "#d73a49",
	"keyword.function": "#d73a49",
	"keyword.operator": "#d73a49",
	"keyword.import":   "#d73a49",

	// Strings and literals
	"string":         "#032f62",
	"string.special": "#032f62",
	"number":         "#005cc5",
	"escape":         "#005cc5",

	// Identifiers
	"function":            "#6f42c1",
	"function.method":     "#6f42c1",
	"function.builtin":    "#6f42c1",
	"constructor":         "#6f42c1",
	"type":                "#22863a",
	"type.builtin":        "#22863a",
	"variable":            "#e36209",
	"variable.parameter":  "#e36209",
	"property":            "#005cc5",

	// Comments
	"comment":               "#6a737d",
	"comment.line":          "#6a737d",
	"comment.block":         "#6a737d",
	"comment.documentation": "#6a737d",

	// Other
	"constant":              "#005cc5",
	"constant.builtin":      "#005cc5",
	"operator":              "#d73a49",
	"punctuation":           "#24292e",
	"punctuation.bracket":   "#24292e",
	"punctuation.delimiter": "#24292e",
	"punctuation.special":   "#24292e",
	"embedded":              "#6a737d",
	"label":                 "#e36209",
	"namespace":             "#22863a",
}

// captureStyles maps capture names to additional CSS property overrides
// beyond color. Applied together with color when both match.
var captureStyles = map[string]string{
	"comment":               "font-style: italic",
	"comment.line":          "font-style: italic",
	"comment.block":         "font-style: italic",
	"comment.documentation": "font-style: italic",
}

// Highlight parses source code and returns an HTML fragment with inline-style
// <span> wrappers for syntax highlighting. The lang parameter should be a
// language name or file extension (e.g. "go", "python", ".js").
//
// If the language cannot be recognized or highlighting fails, the original
// source is returned as a safely HTML-escaped string (graceful degradation).
func Highlight(src []byte, lang string) []byte {
	if len(src) == 0 {
		return nil
	}

	entry := resolveEntry(lang)
	if entry == nil {
		return escapeHTML(src)
	}

	hl, err := newHighlighter(entry)
	if err != nil {
		return escapeHTML(src)
	}

	ranges := hl.Highlight(src)
	if len(ranges) == 0 {
		return escapeHTML(src)
	}

	return buildHTML(src, ranges)
}

// HighlightPage processes an HTML page, finding all <pre class="src-XXX"> blocks
// and syntax-highlighting the code inside their <code> elements.
//
// Returns the modified HTML and the number of blocks processed. On error,
// returns the original HTML with a count of successfully processed blocks
// (partial results are still returned).
func HighlightPage(html []byte) ([]byte, int, error) {
	if len(html) == 0 {
		return html, 0, nil
	}

	// Pattern: <pre class="src-XXX"> ... <code> ... </code> ... </pre>
	// We use simple byte search since Org-mode HTML has predictable structure.
	var result bytes.Buffer
	result.Grow(len(html) * 11 / 10) // slight over-allocation

	remaining := html
	var processed int

	for {
		// Find next <pre class="src-
		preStart := indexPreClassSrc(remaining)
		if preStart < 0 {
			result.Write(remaining)
			break
		}

		// Write everything before this <pre>
		result.Write(remaining[:preStart])
		preBlock := remaining[preStart:]

		// Find the closing </pre>
		preEnd := findClosingTag(preBlock, "pre")
		if preEnd < 0 {
			// Malformed — write the rest as-is
			result.Write(preBlock)
			break
		}

		fullPre := preBlock[:preEnd]
		remaining = preBlock[preEnd:]

		// Extract language from class="src-XXX"
		lang := extractLang(fullPre)
		if lang == "" {
			result.Write(fullPre)
			continue
		}

		// Find <code> block inside this <pre>
		codeStart := bytes.Index(fullPre, []byte("<code>"))
		if codeStart < 0 {
			result.Write(fullPre)
			continue
		}
		codeContent := fullPre[codeStart+6:] // skip <code>

		codeEnd := findClosingTag(codeContent, "code")
		if codeEnd < 0 {
			result.Write(fullPre)
			continue
		}

		rawCode := codeContent[:codeEnd-7] // strip </code> (7 bytes)

		// HTML-decode the code content for tree-sitter
		decoded := htmlUnescape(rawCode)
		highlighted := Highlight(decoded, lang)

		// Build the highlighted <pre> block
		highlightedPre := make([]byte, 0, len(fullPre)+len(highlighted))
		highlightedPre = append(highlightedPre, fullPre[:codeStart+6]...)
		highlightedPre = append(highlightedPre, highlighted...)
		highlightedPre = append(highlightedPre, fullPre[codeStart+6+codeEnd-7:]...)

		result.Write(highlightedPre)
		processed++
	}

	return result.Bytes(), processed, nil
}

// HighlightDir walks a directory recursively, finds all .html files, and
// syntax-highlights code blocks in each one using HighlightPage.
// Returns the total number of blocks processed across all files.
// On individual file errors, processing continues (best-effort).
func HighlightDir(dir string) (total int, totalFiles int, err error) {
	err = filepath.WalkDir(dir, func(path string, d os.DirEntry, e error) error {
		if e != nil {
			return nil // skip inaccessible entries
		}
		if d.IsDir() || !strings.HasSuffix(d.Name(), ".html") {
			return nil
		}
		src, err := os.ReadFile(path)
		if err != nil {
			return nil // skip unreadable files
		}
		highlighted, count, hlErr := HighlightPage(src)
		if hlErr != nil {
			return nil // skip problematic files
		}
		if count == 0 {
			return nil
		}
		if err := os.WriteFile(path, highlighted, 0644); err != nil {
			return nil // skip unwritable files
		}
		total += count
		totalFiles++
		return nil
	})
	return
}

// --- internal helpers ---

// resolveEntry finds the LangEntry for a language name or file extension.
func resolveEntry(lang string) *grammars.LangEntry {
	if lang == "" {
		return nil
	}

	// Try exact name first (e.g. "go", "python")
	if entry := grammars.DetectLanguageByName(lang); entry != nil {
		return entry
	}

	// Try as extension (e.g. ".go")
	if !strings.HasPrefix(lang, ".") {
		lang = "." + lang
	}
	if entry := grammars.DetectLanguage("file" + lang); entry != nil {
		return entry
	}

	return nil
}

// newHighlighter creates a gotreesitter Highlighter from a LangEntry,
// handling TokenSourceFactory if the language requires it.
func newHighlighter(entry *grammars.LangEntry) (*gotreesitter.Highlighter, error) {
	lang := entry.Language()
	q := entry.HighlightQuery
	if q == "" {
		return nil, fmt.Errorf("highlight: no highlight query for %q", entry.Name)
	}

	var opts []gotreesitter.HighlighterOption
	if entry.TokenSourceFactory != nil {
		factory := func(src []byte) gotreesitter.TokenSource {
			return entry.TokenSourceFactory(src, lang)
		}
		opts = append(opts, gotreesitter.WithTokenSourceFactory(factory))
	}

	hl, err := gotreesitter.NewHighlighter(lang, q, opts...)
	if err != nil {
		return nil, fmt.Errorf("highlight: new highlighter for %q: %w", entry.Name, err)
	}
	return hl, nil
}

// buildHTML takes source code and highlight ranges, producing an HTML fragment
// with <span style="color:#xxx"> wrappers.
func buildHTML(src []byte, ranges []gotreesitter.HighlightRange) []byte {
	var buf bytes.Buffer
	buf.Grow(len(src) + len(ranges)*40) // rough estimate

	pos := uint32(0)
	for _, r := range ranges {
		// Emit any unhighlighted text before this range
		if r.StartByte > pos {
			buf.Write(escapeHTML(src[pos:r.StartByte]))
		}

		// Look up color and optional style
		color, hasColor := captureColors[r.Capture]
		extraStyle, hasStyle := captureStyles[r.Capture]

		if !hasColor && !hasStyle {
			// Unknown capture — emit raw text
			buf.Write(escapeHTML(src[r.StartByte:r.EndByte]))
		} else if hasColor && !hasStyle {
			fmt.Fprintf(&buf, `<span style="color:%s">%s</span>`, color, escapeHTML(src[r.StartByte:r.EndByte]))
		} else if !hasColor && hasStyle {
			fmt.Fprintf(&buf, `<span style="%s">%s</span>`, extraStyle, escapeHTML(src[r.StartByte:r.EndByte]))
		} else {
			fmt.Fprintf(&buf, `<span style="color:%s;%s">%s</span>`, color, extraStyle, escapeHTML(src[r.StartByte:r.EndByte]))
		}

		pos = r.EndByte
	}

	// Emit remaining text
	if pos < uint32(len(src)) {
		buf.Write(escapeHTML(src[pos:]))
	}

	return buf.Bytes()
}

// escapeHTML is a convenience wrapper for html.EscapeString.
func escapeHTML(src []byte) []byte {
	return []byte(html.EscapeString(string(src)))
}

// htmlUnescape decodes HTML entities.
func htmlUnescape(src []byte) []byte {
	return []byte(html.UnescapeString(string(src)))
}

// --- HTML parsing helpers ---

// preClassPrefix is the marker we look for to identify Org-mode code blocks.
const preClassPrefix = `<pre class="src-`

// indexPreClassSrc finds the first occurrence of <pre class="src- in data.
func indexPreClassSrc(data []byte) int {
	return bytes.Index(data, []byte(preClassPrefix))
}

// findClosingTag finds the closing tag </tag> in src, handling self-closing
// tags and simple nesting of the same tag type. Returns the index of the
// byte right after </tag> (i.e., the end of the closing tag).
func findClosingTag(src []byte, tag string) int {
	openTag := []byte("<" + tag)
	closeTag := []byte("</" + tag + ">")

	depth := 0
	i := 0
	for i < len(src) {
		// Check for opening tag (not closing)
		if bytes.HasPrefix(src[i:], openTag) &&
			i+1 < len(src) && src[i+1] != '/' {
			// Check if it's self-closing
			rest := src[i+len(openTag):]
			closeIdx := bytes.IndexByte(rest, '>')
			if closeIdx >= 0 && closeIdx >= 1 && rest[closeIdx-1] == '/' {
				// Self-closing, skip past >
				i += len(openTag) + closeIdx + 1
				continue
			}
			if closeIdx >= 0 {
				if depth == 0 {
					// Root opening tag — skip past it (we're finding its matching close)
					i += len(openTag) + closeIdx + 1
					continue
				}
				// Nested opening tag of same type
				depth++
			}
		}
		// Check for closing tag
		if bytes.HasPrefix(src[i:], closeTag) {
			if depth == 0 {
				return i + len(closeTag)
			}
			depth--
			i += len(closeTag)
			continue
		}
		i++
	}
	return -1
}

// extractLang extracts the language name from a <pre class="src-XXX"> tag.
// Returns empty string if not found.
func extractLang(pre []byte) string {
	// Find class attribute containing "src-"
	classMarker := []byte(`class="`)
	idx := bytes.Index(pre, classMarker)
	if idx < 0 {
		return ""
	}
	rest := pre[idx+len(classMarker):]

	// Find closing quote
	end := bytes.IndexByte(rest, '"')
	if end < 0 {
		return ""
	}
	classValue := string(rest[:end])

	// Find "src-" in the class value
	for _, token := range strings.Fields(classValue) {
		if strings.HasPrefix(token, "src-") {
			lang := strings.TrimPrefix(token, "src-")
			lang = strings.ToLower(lang)
			lang = strings.ReplaceAll(lang, "c++", "cpp")
			lang = strings.ReplaceAll(lang, "c#", "csharp")
			lang = strings.ReplaceAll(lang, "c%2b%2b", "cpp")
			return lang
		}
	}

	return ""
}

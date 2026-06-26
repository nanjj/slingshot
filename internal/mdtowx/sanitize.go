// Package mdtowx provides Markdown-to-WeChat-HTML conversion.
package mdtowx

import (
	"bytes"
	"strings"
	"unicode"

	"golang.org/x/net/html"
)

// SanitizeHTML removes HTML elements that WeChat's API may reject.
//
// WeChat editor is stricter than the browser. Org-mode, Pandoc, and Goldmark
// generated HTML often contains elements that trigger error 45166
// ("Invalid Content"):
//
//  1. Full HTML document structure: <!DOCTYPE>, <html>, <head>, <body>
//     → stripped, keeping only the content inside <body>
//  2. Org-mode artifacts: \equal{} → =
//  3. mailto links: <a href="mailto:...">text</a> → text (keep label, remove link)
//  4. Footnote backlinks: <sup><a role="doc-backlink" class="footref"...>N</a></sup>
//     → N (remove <sup> and <a>, keep the number as plain text)
//  5. Footnote definition anchors: <sup><a id="fn.X" href="#fnr.X">N</a></sup>
//     → N (same treatment)
func SanitizeHTML(src []byte) []byte {
	// Preprocess: extract body content, fix common artifacts
	content := prepareContent(src)

	s := &sanitizer{
		z: html.NewTokenizer(bytes.NewReader(content)),
	}
	s.run()
	return s.out.Bytes()
}

// prepareContent preprocesses HTML content before sanitization:
//  1. Extracts content between <body> and </body> (removes DOCTYPE, html, head)
//  2. Replaces Org-mode \equal{} with =
func prepareContent(src []byte) []byte {
	// Extract body content if this is a full HTML document
	bodyStart := bytes.Index(src, []byte("<body"))
	bodyEnd := bytes.LastIndex(src, []byte("</body>"))
	if bodyStart >= 0 && bodyEnd > bodyStart {
		tagEnd := bytes.Index(src[bodyStart:], []byte(">"))
		if tagEnd >= 0 {
			contentStart := bodyStart + tagEnd + 1
			src = src[contentStart:bodyEnd]
		}
	}

	// Replace Org-mode \equal{} with =
	src = bytes.ReplaceAll(src, []byte(`\equal{}`), []byte(`=`))

	return src
}

// sanitizer holds the state while processing HTML tokens.
type sanitizer struct {
	z   *html.Tokenizer
	out bytes.Buffer

	// When buffering a <sup>, we collect tokens here until </sup>.
	buf         bytes.Buffer
	bufDepth    int  // nesting depth inside <sup>
	supHasIssue bool // whether the buffered <sup> contains a problematic <a>

	// strippedA tracks how many <a> opening tags we've stripped.
	// When we see </a> and strippedA > 0, we skip the closing tag.
	strippedA int
}

func (s *sanitizer) run() {
	for {
		tt := s.z.Next()
		if tt == html.ErrorToken {
			break
		}

		switch tt {
		case html.StartTagToken, html.SelfClosingTagToken:
			s.handleStart(tt)
		case html.EndTagToken:
			s.handleEnd()
		case html.TextToken:
			s.handleText()
		case html.CommentToken:
			s.handleRaw()
		case html.DoctypeToken:
			s.handleRaw()
		}
	}
}

func (s *sanitizer) handleStart(tt html.TokenType) {
	tagName, hasAttrs := s.z.TagName()
	name := string(tagName)

	// <sup> always starts buffering.
	if name == "sup" {
		s.buf.Reset()
		s.buf.Write(s.z.Raw())
		s.bufDepth = 1
		s.supHasIssue = false
		return
	}

	if s.bufDepth > 0 {
		// Inside a <sup> buffer.
		s.buf.Write(s.z.Raw())
		if tt == html.StartTagToken {
			s.bufDepth++
			if name == "a" && hasAttrs && isProblematicA(s.z) {
				s.supHasIssue = true
			}
		}
		return
	}

	// Outside <sup>: check for problematic <a>.
	if name == "a" && hasAttrs && isProblematicA(s.z) {
		s.strippedA++
		return // skip opening tag, text passes through naturally
	}

	s.out.Write(s.z.Raw())
}

func (s *sanitizer) handleEnd() {
	tagName, _ := s.z.TagName()
	name := string(tagName)

	if s.bufDepth > 0 {
		// Inside a <sup> buffer: all closing tags decrement depth.
		s.buf.Write(s.z.Raw())
		s.bufDepth--
		if s.bufDepth == 0 {
			// <sup> is fully buffered. Decide what to output.
			if s.supHasIssue {
				// Keep <sup> wrapper but strip the <a> inside.
				// This makes footnote references display as superscript in WeChat.
				text := RemoveCJCSpace(stripTags(s.buf.Bytes()))
				if len(bytes.TrimSpace(text)) > 0 {
					s.out.Write([]byte("<sup>"))
					s.out.Write(text)
					s.out.Write([]byte("</sup>"))
				}
			} else {
				s.out.Write(s.buf.Bytes())
			}
			s.buf.Reset()
		}
		return
	}

	// Outside buffer: skip </a> that matches a stripped opening tag.
	if name == "a" && s.strippedA > 0 {
		s.strippedA--
		return
	}

	s.out.Write(s.z.Raw())
}

func (s *sanitizer) handleText() {
	if s.bufDepth > 0 {
		s.buf.Write(s.z.Raw())
		return
	}
	s.out.Write(RemoveCJCSpace(s.z.Raw()))
}

func (s *sanitizer) handleRaw() {
	if s.bufDepth > 0 {
		s.buf.Write(s.z.Raw())
		return
	}
	s.out.Write(s.z.Raw())
}

// isProblematicA checks if an <a> tag (with attributes already buffered by TagName)
// is a mailto link or a footnote backlink.
func isProblematicA(z *html.Tokenizer) bool {
	hasMailto := false
	for {
		key, val, more := z.TagAttr()
		sk := string(key)
		sv := string(val)
		if sk == "href" && strings.HasPrefix(strings.ToLower(sv), "mailto:") {
			hasMailto = true
		}
		if sk == "role" && sv == "doc-backlink" {
			return true
		}
		if sk == "class" {
			for _, c := range strings.Fields(sv) {
				if c == "footref" {
					return true
				}
			}
		}
		// Footnote definition anchors: <a id="fn.1" href="#fnr.1">.
		// Pandoc generates these without role/class, so they need a
		// separate check on id prefix.
		if sk == "id" && (strings.HasPrefix(sv, "fn.") || strings.HasPrefix(sv, "fnr.")) {
			return true
		}
		if !more {
			break
		}
	}
	return hasMailto
}

// stripTags removes all HTML tags from content, keeping only text.
func stripTags(src []byte) []byte {
	var out bytes.Buffer
	inTag := false
	for _, b := range src {
		if b == '<' {
			inTag = true
			continue
		}
		if b == '>' {
			inTag = false
			continue
		}
		if !inTag {
			out.WriteByte(b)
		}
	}
	return bytes.TrimSpace(out.Bytes())
}

// RemoveCJCSpace removes spaces (' ') that appear between two non-ASCII
// characters. This is commonly needed for CJK text where spaces between
// Chinese/Japanese/Korean characters are artifacts of markdown or org-mode
// processing, but works for any non-ASCII script (Cyrillic, Greek, etc.).
//
// Examples:
//
//	"相 信"             → "相信"
//	"I am an AI"        → "I am an AI"  (unchanged — all ASCII)
//	"Hello 世界"        → "Hello 世界"  (unchanged — ASCII on one side)
//	"我们 相信 你"      → "我们相信你"
// RemoveCJCSpace removes whitespace characters (spaces, newlines, tabs, etc.)
// that appear between two non-ASCII characters. This is commonly needed for CJK
// text where whitespace between Chinese/Japanese/Korean characters are artifacts
// of markdown or org-mode line wrapping, but works for any non-ASCII script
// (Cyrillic, Greek, etc.).
//
// Examples:
//
//	"相 信"             → "相信"
//	"I am an AI"        → "I am an AI"  (unchanged — all ASCII)
//	"Hello 世界"        → "Hello 世界"  (unchanged — ASCII on one side)
//	"我们 相信 你"      → "我们相信你"
//	"有\n多"            → "有多少"      (newline removed)
func RemoveCJCSpace(src []byte) []byte {
	s := string(src)
	runes := []rune(s)
	var b strings.Builder
	b.Grow(len(s))

	for i := 0; i < len(runes); i++ {
		if unicode.IsSpace(runes[i]) && i > 0 && i < len(runes)-1 {
			// Look ahead past consecutive whitespace
			j := i + 1
			for j < len(runes) && unicode.IsSpace(runes[j]) {
				j++
			}
			// Remove the whole whitespace-run if sandwiched between non-ASCII chars
			if j < len(runes) && runes[i-1] > unicode.MaxASCII && runes[j] > unicode.MaxASCII {
				i = j - 1 // loop increment will advance past j
				continue
			}
		}
		b.WriteRune(runes[i])
	}
	return []byte(b.String())
}

// Package mdtowx provides Markdown-to-WeChat-HTML conversion.
package mdtowx

import (
	"bytes"
	"strings"

	"golang.org/x/net/html"
)

// SanitizeHTML removes HTML elements that WeChat's API may reject.
//
// WeChat editor is stricter than the browser. Pandoc-generated HTML often
// contains elements that trigger error 45166 ("Invalid Content"):
//
//  1. mailto links: <a href="mailto:...">text</a> → text (keep label, remove link)
//  2. Footnote backlinks: <sup><a role="doc-backlink" class="footref"...>N</a></sup>
//     → N (remove <sup> and <a>, keep the number as plain text)
//  3. Footnote definition anchors: <sup><a id="fn.X" href="#fnr.X">N</a></sup>
//     → N (same treatment)
//
// The function uses html.NewTokenizer so it works on HTML fragments without
// wrapping them in a full document structure.
func SanitizeHTML(src []byte) []byte {
	s := &sanitizer{
		z: html.NewTokenizer(bytes.NewReader(src)),
	}
	s.run()
	return s.out.Bytes()
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
				s.out.Write(stripTags(s.buf.Bytes()))
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
	s.out.Write(s.z.Raw())
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

package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
)

// --- helpers ---

// stripTags removes HTML tags for preview text.
func stripTags(s string) string {
	var out strings.Builder
	inTag := false
	for _, r := range s {
		if r == '<' {
			inTag = true
			continue
		}
		if r == '>' {
			inTag = false
			continue
		}
		if !inTag {
			out.WriteRune(r)
		}
	}
	return out.String()
}

// truncate truncates a string to maxLen, adding "..." if truncated.
func truncate(s string, maxLen int) string {
	runes := []rune(s)
	if len(runes) <= maxLen {
		return s
	}
	return string(runes[:maxLen]) + "..."
}

// wrapHTML wraps body HTML in a complete document with metadata.
func wrapHTML(title, author, thumbMediaID string, body []byte) []byte {
	var buf bytes.Buffer
	buf.WriteString("<!DOCTYPE html>\n<html>\n<head>\n")
	if title != "" {
		buf.WriteString("<title>")
		buf.WriteString(htmlEscape(title))
		buf.WriteString("</title>\n")
	}
	if author != "" {
		buf.WriteString(`<meta name="author" content="`)
		buf.WriteString(htmlEscape(author))
		buf.WriteString(`">` + "\n")
	}
	if thumbMediaID != "" {
		buf.WriteString(`<meta name="thumb_media_id" content="`)
		buf.WriteString(htmlEscape(thumbMediaID))
		buf.WriteString(`">` + "\n")
	}
	buf.WriteString(`<meta charset="utf-8">` + "\n")
	buf.WriteString("</head>\n<body>\n")
	buf.Write(body)
	buf.WriteString("\n</body>\n</html>\n")
	return buf.Bytes()
}

// htmlEscape escapes special HTML characters.
func htmlEscape(s string) string {
	s = strings.ReplaceAll(s, "&", "&amp;")
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, ">", "&gt;")
	s = strings.ReplaceAll(s, `"`, "&quot;")
	s = strings.ReplaceAll(s, "'", "&#39;")
	return s
}

// replaceExt replaces the extension of a file path with ext.
func replaceExt(path, ext string) string {
	orig := filepath.Ext(path)
	return path[:len(path)-len(orig)] + ext
}

// looksLikeImageFile returns true if the path has a common image file extension.
// This distinguishes file paths from raw media_id values.
func looksLikeImageFile(path string) bool {
	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".png", ".jpg", ".jpeg", ".gif", ".webp", ".bmp", ".svg", ".ico", ".tiff", ".tif", ".avif":
		return true
	}
	return false
}

// isLocalFile checks if a path relative to baseDir exists on disk.
func isLocalFile(baseDir, path string) bool {
	if !filepath.IsAbs(path) {
		path = filepath.Join(baseDir, path)
	}
	path = filepath.Clean(path)
	_, err := os.Stat(path)
	return err == nil
}

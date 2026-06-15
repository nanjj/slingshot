package main

import (
	"html"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// --- HTML extraction helpers ---

// extractTitle extracts the article title from HTML <title> tag,
// falling back to the filename without extension.
func extractTitle(htmlContent, filePath string) string {
	// Try <title> tag
	lower := strings.ToLower(htmlContent)
	start := strings.Index(lower, "<title>")
	if start >= 0 {
		start += len("<title>")
		end := strings.Index(lower[start:], "</title>")
		if end >= 0 {
			title := htmlContent[start : start+end]
			title = strings.TrimSpace(title)
			title = html.UnescapeString(title)
			if title != "" {
				return title
			}
		}
	}

	// Fallback: filename without extension
	base := filepath.Base(filePath)
	ext := filepath.Ext(base)
	return base[:len(base)-len(ext)]
}

// extractAuthor extracts the author from <meta name="author" content="..."> in HTML.
func extractAuthor(htmlContent string) string {
	lower := strings.ToLower(htmlContent)
	// Match <meta name="author" content="...">
	marker := `<meta name="author" content="`
	start := strings.Index(lower, marker)
	if start < 0 {
		// Try with single quotes
		marker = `<meta name="author" content='`
		start = strings.Index(lower, marker)
		if start < 0 {
			return ""
		}
	}
	start += len(marker)
	end := strings.Index(htmlContent[start:], `"`)
	if end < 0 {
		end = strings.Index(htmlContent[start:], `'`)
	}
	if end < 0 {
		return ""
	}
	author := htmlContent[start : start+end]
	author = strings.TrimSpace(author)
	author = html.UnescapeString(author)
	return author
}

// extractThumbMediaID extracts the cover media_id from sidecar YAML first,
// falling back to <meta name="thumb_media_id" content="..."> in the HTML content.
func extractThumbMediaID(htmlContent, filePath string) string {
	// Prefer sidecar YAML
	if meta, ok := readSidecarYAML(filePath); ok && meta.ThumbMediaID != "" {
		return meta.ThumbMediaID
	}
	// Fallback: <meta name="thumb_media_id" content="..."> in HTML
	lower := strings.ToLower(htmlContent)
	for _, quote := range []string{`"`, `'`} {
		marker := `<meta name="thumb_media_id" content=` + quote
		start := strings.Index(lower, marker)
		if start < 0 {
			continue
		}
		start += len(marker)
		end := strings.Index(htmlContent[start:], quote)
		if end < 0 {
			continue
		}
		return strings.TrimSpace(htmlContent[start : start+end])
	}
	return ""
}

// --- Sidecar YAML helpers ---

// sidecarMeta defines the YAML fields that can be stored alongside an HTML file.
type sidecarMeta struct {
	MediaID            string `yaml:"media_id"`
	Title              string `yaml:"title"`
	Author             string `yaml:"author"`
	ThumbMediaID       string `yaml:"thumb_media_id"`
	Digest             string `yaml:"digest"`
	ContentSourceURL   string `yaml:"content_source_url"`
	NeedOpenComment    *int   `yaml:"need_open_comment"`
	OnlyFansCanComment *int   `yaml:"only_fans_can_comment"`
}

// readSidecarYAML attempts to read a sidecar YAML file for the given
// file path. It looks for <filename>.yaml first, then <filename>.yml.
// Returns the parsed metadata and true on success.
func readSidecarYAML(filePath string) (sidecarMeta, bool) {
	base := filePath[:len(filePath)-len(filepath.Ext(filePath))]
	for _, ext := range []string{".yaml", ".yml"} {
		yamlPath := base + ext
		data, err := os.ReadFile(yamlPath)
		if err != nil {
			continue
		}
		var meta sidecarMeta
		if err := yaml.Unmarshal(data, &meta); err != nil {
			continue
		}
		return meta, true
	}
	return sidecarMeta{}, false
}

// writeSidecarYAML writes or updates the sidecar YAML file for the given
// file path. It always writes <filename>.yaml.
func writeSidecarYAML(filePath string, meta sidecarMeta) error {
	base := filePath[:len(filePath)-len(filepath.Ext(filePath))]
	yamlPath := base + ".yaml"
	data, err := yaml.Marshal(&meta)
	if err != nil {
		return err
	}
	return os.WriteFile(yamlPath, data, 0644)
}

// extractDigest extracts the digest from sidecar YAML first, falling back
// to <meta name="digest" content="..."> in the HTML content.
func extractDigest(htmlContent, filePath string) string {
	// Prefer sidecar YAML
	if meta, ok := readSidecarYAML(filePath); ok && meta.Digest != "" {
		return meta.Digest
	}
	// Fallback: <meta name="digest" content="..."> in HTML
	lower := strings.ToLower(htmlContent)
	for _, quote := range []string{`"`, `'`} {
		marker := `<meta name="digest" content=` + quote
		start := strings.Index(lower, marker)
		if start < 0 {
			continue
		}
		start += len(marker)
		end := strings.Index(htmlContent[start:], quote)
		if end < 0 {
			continue
		}
		return strings.TrimSpace(htmlContent[start : start+end])
	}
	return ""
}

// extractContentSourceURL extracts the content_source_url from sidecar YAML first,
// falling back to <meta name="content_source_url" content="..."> in the HTML content.
func extractContentSourceURL(htmlContent, filePath string) string {
	// Prefer sidecar YAML
	if meta, ok := readSidecarYAML(filePath); ok && meta.ContentSourceURL != "" {
		return meta.ContentSourceURL
	}
	// Fallback: <meta name="content_source_url" content="..."> in HTML
	lower := strings.ToLower(htmlContent)
	for _, quote := range []string{`"`, `'`} {
		marker := `<meta name="content_source_url" content=` + quote
		start := strings.Index(lower, marker)
		if start < 0 {
			continue
		}
		start += len(marker)
		end := strings.Index(htmlContent[start:], quote)
		if end < 0 {
			continue
		}
		return strings.TrimSpace(htmlContent[start : start+end])
	}
	return ""
}

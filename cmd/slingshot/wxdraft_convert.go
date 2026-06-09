package main

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/fatih/color"
	"github.com/spf13/cobra"

	cli "github.com/nanjj/slingshot/internal/cmd"
	"github.com/nanjj/slingshot/internal/config"
	"github.com/nanjj/slingshot/internal/getaccesstoken"
	"github.com/nanjj/slingshot/internal/i18n"
	"github.com/nanjj/slingshot/internal/mdtowx"
	"github.com/nanjj/slingshot/internal/uploadcache"
	"github.com/nanjj/slingshot/internal/uploadimage"
	u "github.com/nanjj/slingshot/internal/usage"
)

// 定义 wxdraft convert 子命令的语法:
//
//	wxdraft convert <file>
var wxdraftConvertUsage = u.Usage{
	u.File, // <file>
}

// cmdWxdraftConvert 实现 "slingshot wxdraft convert" 子命令。
type cmdWxdraftConvert struct {
	global *cmdGlobal
	upload bool
}

func (c *cmdWxdraftConvert) command() *cobra.Command {
	cmd := &cobra.Command{}
	cmd.Use = "convert " + u.File.Render()
	cmd.Short = i18n.G("Convert Markdown to WeChat public account HTML")
	cmd.Long = cli.FormatSection(
		color.CyanString("Description:"),
		i18n.G(`Convert a Markdown file to HTML format suitable for WeChat public accounts.

The conversion process:
  1. Parse the Markdown file using goldmark (supports GFM tables, strikethrough)
  2. Inject inline CSS styles on every element (required by WeChat)
  3. Save the result as <filename>.html

With --upload:
  4. Parse local image references from the HTML
  5. Upload images to WeChat material management (with caching via images.yaml)
  6. Update image URLs in the HTML

WeChat only supports inline styles and a limited set of HTML elements.
This command handles all common Markdown syntax:
  - Headings (h1-h4), paragraphs, blockquotes
  - Bold, italic, inline code, strikethrough
  - Fenced code blocks with syntax highlighting
  - Links and images
  - Ordered and unordered lists
  - Tables (GFM-style)
  - Thematic breaks (hr)
  - Raw HTML passthrough (footnotes, etc.)`),
	)
	cmd.RunE = c.run
	cmd.Args = cobra.ArbitraryArgs
	cmd.Flags().BoolVarP(&c.upload, "upload", "u", false,
		i18n.G("Upload images to WeChat and update URLs in the HTML"))

	return cmd
}

func (c *cmdWxdraftConvert) run(cmd *cobra.Command, args []string) error {
	parsed, err := c.global.Parse(wxdraftConvertUsage, cmd, args)
	if err != nil {
		return err
	}

	// parsed[0] = File
	file := parsed[0]

	fmt.Fprintf(cmd.OutOrStdout(), i18n.G("Converting %s to WeChat HTML...\n"), file.String)

	result, err := mdtowx.ConvertFile(file.String)
	if err != nil {
		return fmt.Errorf("conversion failed: %w", err)
	}
	html := result.HTML

	// 输出文件名: <filename>.html (与输入同目录)
	outPath := replaceExt(file.String, ".html")

	if !c.upload {
		// Without --upload: wrap with metadata and save
		outHTML := wrapHTML(result.Title, result.Author, result.ThumbMediaID, html)
		if err := os.WriteFile(outPath, outHTML, 0644); err != nil {
			return fmt.Errorf("writing output %q: %w", outPath, err)
		}
		fmt.Fprintf(cmd.OutOrStdout(), i18n.G("Written to %s\n"), outPath)
		return nil
	}

	// With --upload: full pipeline

	// Step 2: Extract local image paths from HTML
	baseDir := filepath.Dir(file.String)
	refs := mdtowx.ExtractImagePaths(html, baseDir)

	if len(refs) == 0 {
		fmt.Fprintln(cmd.OutOrStdout(), i18n.G("No local images found, saving HTML..."))
		outHTML := wrapHTML(result.Title, result.Author, result.ThumbMediaID, html)
		if err := os.WriteFile(outPath, outHTML, 0644); err != nil {
			return fmt.Errorf("writing output %q: %w", outPath, err)
		}
		fmt.Fprintf(cmd.OutOrStdout(), i18n.G("Written to %s\n"), outPath)
		return nil
	}

	fmt.Fprintf(cmd.OutOrStdout(), i18n.G("Found %d local image(s)\n"), len(refs))

	cfg, _, err := config.Load()
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	// Get access token (uses cache or requests new)
	token, err := getaccesstoken.GetToken(cfg)
	if err != nil {
		return fmt.Errorf("getting WeChat access token: %w", err)
	}

	// Step 3: Load image upload cache and upload (or skip cached) images
	cache := uploadcache.Load(baseDir)

	uploaded := make(map[string]string)     // AbsPath → WeChat URL
	replacements := make(map[string]string) // original Src → WeChat URL

	for _, ref := range refs {
		// Skip if already processed this absolute path
		if _, done := uploaded[ref.AbsPath]; done {
			replacements[ref.Src] = uploaded[ref.AbsPath]
			continue
		}

		// Check file exists
		if _, err := os.Stat(ref.AbsPath); os.IsNotExist(err) {
			fmt.Fprintf(cmd.ErrOrStderr(), i18n.G("Warning: image file not found: %s\n"), ref.AbsPath)
			continue
		}

		// Compute cache key: md5 checksum
		key, err := uploadcache.Key(ref.AbsPath)
		if err != nil {
			fmt.Fprintf(cmd.ErrOrStderr(), i18n.G("Warning: checksum failed for %s: %v\n"), ref.AbsPath, err)
			continue
		}

		// Check image upload cache
		if cachedURL, ok := cache.Get(key); ok && cachedURL != "" {
			fmt.Fprintf(cmd.OutOrStdout(), i18n.G("Using cached %s -> %s\n"), filepath.Base(ref.AbsPath), cachedURL)
			uploaded[ref.AbsPath] = cachedURL
			replacements[ref.Src] = cachedURL
			continue
		}

		// Not cached — upload to WeChat
		fmt.Fprintf(cmd.OutOrStdout(), i18n.G("Uploading %s...\n"), ref.AbsPath)
		resp, err := uploadimage.Upload(token, ref.AbsPath)
		if err != nil {
			fmt.Fprintf(cmd.ErrOrStderr(), i18n.G("Warning: upload failed for %s: %v\n"), ref.AbsPath, err)
			continue
		}

		fmt.Fprintf(cmd.OutOrStdout(), i18n.G("Uploaded %s -> %s\n"), filepath.Base(ref.AbsPath), resp.URL)
		cache.Set(key, filepath.Base(ref.AbsPath), resp.URL)
		uploaded[ref.AbsPath] = resp.URL
		replacements[ref.Src] = resp.URL
	}

	if len(replacements) == 0 {
		fmt.Fprintln(cmd.ErrOrStderr(), i18n.G("Warning: no images were uploaded, saving original HTML"))
	}

	// Save image upload cache
	if err := cache.Save(); err != nil {
		fmt.Fprintf(cmd.ErrOrStderr(), i18n.G("Warning: failed to save image cache: %v\n"), err)
	}

	// Step 4: Update image URLs in HTML
	updatedHTML := mdtowx.ReplaceImageURLs(html, replacements)

	// Wrap with metadata and save
	outHTML := wrapHTML(result.Title, result.Author, result.ThumbMediaID, updatedHTML)
	if err := os.WriteFile(outPath, outHTML, 0644); err != nil {
		return fmt.Errorf("writing output %q: %w", outPath, err)
	}

	fmt.Fprintf(cmd.OutOrStdout(), i18n.G("Upload complete, written to %s\n"), outPath)
	return nil
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

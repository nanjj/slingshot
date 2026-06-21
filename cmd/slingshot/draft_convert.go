package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/fatih/color"
	"github.com/spf13/cobra"

	cli "github.com/nanjj/slingshot/internal/cmd"
	"github.com/nanjj/slingshot/internal/i18n"
	"github.com/nanjj/slingshot/internal/mdtowx"
	u "github.com/nanjj/slingshot/internal/usage"
)

// 定义 draft convert 子命令的语法:
//
//	draft convert <file>

var draftConvertUsage = u.Usage{
	u.File, // <file>
}

// cmdDraftConvert 实现 "slingshot draft convert" 子命令。
type cmdDraftConvert struct {
	global *cmdGlobal
	upload bool
}

func (c *cmdDraftConvert) command() *cobra.Command {
	cmd := &cobra.Command{}
	cmd.Use = "convert " + u.File.Render()
	cmd.Short = i18n.G("Convert Markdown or Org to WeChat public account HTML")
	cmd.Long = cli.FormatSection(
		color.CyanString("Description:"),
		i18n.G(`Convert a Markdown (.md) or Org (.org) file to HTML format suitable for WeChat public accounts.

The conversion process:
  1. If the input is a .org file, convert to Markdown using Emacs batch mode
  2. Parse the Markdown using goldmark (supports GFM tables, strikethrough)
  3. Inject inline CSS styles on every element (required by WeChat)
  4. Save the result as <filename>.html

With --upload:
  5. Parse local image references from the HTML
  6. Upload images to WeChat material management (with caching via images.yaml)
  7. Update image URLs in the HTML
  8. If the YAML front matter or sidecar YAML contains 'thumb_media_id: <path>', upload the
     thumbnail image as permanent material and replace with the real media_id

WeChat only supports inline styles and a limited set of HTML elements.
This command handles all common syntax:
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

func (c *cmdDraftConvert) run(cmd *cobra.Command, args []string) error {
	parsed, err := c.global.Parse(draftConvertUsage, cmd, args)
	if err != nil {
		return err
	}

	// parsed[0] = File
	file := parsed[0]

	fmt.Fprintf(cmd.OutOrStdout(), i18n.G("Converting %s to WeChat HTML...\n"), file.String)

	// Route based on file extension: .org -> new Org converter, .md -> existing Markdown converter
	ext := strings.ToLower(filepath.Ext(file.String))

	var result *mdtowx.Result
	switch ext {
	case ".org":
		result, err = mdtowx.ConvertOrgFile(file.String)
	default:
		result, err = mdtowx.ConvertFile(file.String)
	}
	if err != nil {
		return fmt.Errorf("conversion failed: %w", err)
	}
	html := result.HTML
	// Remove spaces between non-ASCII (e.g., CJK) characters — common artifact from
	// markdown/org paragraph wrapping or manual editing (e.g., "相 信" -> "相信").
	html = mdtowx.RemoveCJCSpace(html)

	outPath := replaceExt(file.String, ".html")

	if !c.upload {
		// Without --upload: check for thumbnail file that needs uploading
		if result.ThumbMediaID != "" && looksLikeImageFile(result.ThumbMediaID) && isLocalFile(filepath.Dir(file.String), result.ThumbMediaID) {
			fmt.Fprintf(cmd.ErrOrStderr(), i18n.G("Warning: thumb_media_id %q looks like a local file.\n"+
				"  Use --upload to auto-upload the thumbnail image, or upload it manually and \n"+
				"  replace the value with a real media_id from WeChat material management.\n"), result.ThumbMediaID)
		}
		outHTML := wrapHTML(result.Title, result.Author, result.ThumbMediaID, html)
		if err := os.WriteFile(outPath, outHTML, 0644); err != nil {
			return fmt.Errorf("writing output %q: %w", outPath, err)
		}
		fmt.Fprintf(cmd.OutOrStdout(), i18n.G("Written to %s\n"), outPath)
		return nil
	}

	return runUploadPipeline(cmd, result, html, outPath, file.String)
}

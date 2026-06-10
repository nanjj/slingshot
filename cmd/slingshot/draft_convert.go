package main

import (
	"fmt"
	"os"
	"path/filepath"

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
  7. If the YAML front matter contains 'thumb_media_id: <path>', upload the
     thumbnail image as permanent material and replace with the real media_id

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

func (c *cmdDraftConvert) run(cmd *cobra.Command, args []string) error {
	parsed, err := c.global.Parse(draftConvertUsage, cmd, args)
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

	return c.runUpload(cmd, result, html, outPath, file.String)
}

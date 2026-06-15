package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/fatih/color"
	cli "github.com/nanjj/slingshot/internal/cmd"
	"github.com/nanjj/slingshot/internal/config"
	"github.com/nanjj/slingshot/internal/draft"
	"github.com/nanjj/slingshot/internal/getaccesstoken"
	"github.com/nanjj/slingshot/internal/i18n"
	"github.com/nanjj/slingshot/internal/mdtowx"
	"github.com/nanjj/slingshot/internal/uploadcache"
	"github.com/nanjj/slingshot/internal/uploadimage"
	u "github.com/nanjj/slingshot/internal/usage"
	"github.com/spf13/cobra"
)

// --- cmdDraftAdd ---

// cmdDraftAdd implements "slingshot draft add <file>".
type cmdDraftAdd struct {
	global *cmdGlobal
	title  string
	thumb  string
}

func (c *cmdDraftAdd) command() *cobra.Command {
	cmd := &cobra.Command{}
	cmd.Use = "add " + u.File.Render()
	cmd.Short = i18n.G("Create a new draft from .org/.md/.html file")
	cmd.Long = cli.FormatSection(
		color.CyanString("Description:"),
		i18n.G(`Create a new WeChat draft from a file. Supports .org, .md, and .html.
If a .org or .md file is provided, it is auto-converted to HTML first
(with image uploads and thumbnail resolution).

The title is extracted from the <title> tag in the HTML, falling back
to the filename without extension. Use --title to override.

The cover media_id is required by WeChat. Use --thumb to specify, or
include <meta name="thumb_media_id" content="..."> in the HTML.`),
	)

	cmd.Flags().StringVarP(&c.title, "title", "t", "",
		i18n.G("Article title (overrides auto-detection from HTML)"))
	cmd.Flags().StringVarP(&c.thumb, "thumb", "", "",
		i18n.G("Cover image media_id (required; local image file paths are auto-uploaded to WeChat material)"))
	cmd.RunE = c.run
	cmd.Args = cobra.ArbitraryArgs
	return cmd
}

func (c *cmdDraftAdd) run(cmd *cobra.Command, args []string) error {
	parsed, err := c.global.Parse(draftAddUsage, cmd, args)
	if err != nil {
		return err
	}
	if len(parsed) < 1 || parsed[0].Skipped {
		return errors.New(i18n.G("expected a file argument"))
	}
	file := parsed[0].String

	// Auto-convert .org/.md to .html if needed
	file, err = ensureHTMLFile(cmd, file)
	if err != nil {
		return err
	}

	// Read HTML file
	htmlContent, err := os.ReadFile(file)
	if err != nil {
		return fmt.Errorf("reading %q: %w", file, err)
	}
	htmlStr := string(htmlContent)

	// Determine title
	title := c.title
	if title == "" {
		title = extractTitle(htmlStr, file)
	}

	// Validate title
	if err := mdtowx.ValidateTitle(title); err != nil {
		return fmt.Errorf("invalid title: %w", err)
	}

	// Extract and sanitize author
	author := mdtowx.SanitizeAuthor(extractAuthor(htmlStr))

	// Determine cover media_id
	thumbMediaID := c.thumb
	if thumbMediaID == "" {
		thumbMediaID = extractThumbMediaID(htmlStr, file)
	}
	if thumbMediaID == "" {
		return errors.New(i18n.G("cover media_id is required: use --thumb flag or add <meta name=\"thumb_media_id\" content=\"...\"> to HTML"))
	}

	// Extract digest from HTML meta or sidecar YAML
	digest := extractDigest(htmlStr, file)

	// Extract content_source_url from sidecar YAML or HTML meta
	contentSourceURL := extractContentSourceURL(htmlStr, file)

	// NeedOpenComment and OnlyFansCanComment: default to 1 (open, fans-only)
	// to help attract attention and gain more followers. Sidecar YAML can override.
	needOpenComment := 1
	onlyFansCanComment := 1
	if meta, ok := readSidecarYAML(file); ok {
		if meta.NeedOpenComment != nil {
			needOpenComment = *meta.NeedOpenComment
		}
		if meta.OnlyFansCanComment != nil {
			onlyFansCanComment = *meta.OnlyFansCanComment
		}
	}

	// Load config and get token
	cfg, _, err := config.Load()
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}
	token, err := getaccesstoken.GetToken(cfg)
	if err != nil {
		return fmt.Errorf("getting access token: %w", err)
	}

	// Auto-upload thumbnail if the value looks like a local image file path
	if looksLikeImageFile(thumbMediaID) {
		baseDir := filepath.Dir(file)
		thumbPath := thumbMediaID
		if !filepath.IsAbs(thumbPath) {
			thumbPath = filepath.Join(baseDir, thumbPath)
		}
		thumbPath = filepath.Clean(thumbPath)

		if _, err := os.Stat(thumbPath); err == nil {
			thumbBaseName := filepath.Base(thumbPath)
			cache := uploadcache.LoadCache(baseDir)
			key, err := uploadcache.Key(thumbPath)
			if err != nil {
				fmt.Fprintf(cmd.ErrOrStderr(), i18n.G("Warning: checksum failed for thumbnail %s: %v\n"), thumbPath, err)
			} else {
				// Check cache by md5 key, then by filename
				var cachedMediaID string

				if mid, ok := cache.GetMediaID(key); ok && mid != "" {
					cachedMediaID = mid
				} else if entry, ok := cache.GetByFilename(thumbBaseName); ok && entry.MediaID != "" {
					cachedMediaID = entry.MediaID
				}

				if cachedMediaID != "" && cachedMediaID != "0" {
					thumbMediaID = cachedMediaID
					fmt.Fprintf(cmd.OutOrStdout(), i18n.G("Using cached thumbnail %s -> %s\n"),
						thumbBaseName, thumbMediaID)
				} else {
					// Convert SVG thumbnail to PNG if needed
					thumbUploadPath, thumbCleanup := maybeConvertSVG(thumbPath, cmd.ErrOrStderr())

					fmt.Fprintf(cmd.OutOrStdout(), i18n.G("Uploading thumbnail %s...\n"), thumbUploadPath)
					mediaID, thumbURL, err := uploadimage.UploadThumb(token, thumbUploadPath)
					if thumbCleanup {
						os.Remove(thumbUploadPath)
					}
					if err != nil {
						fmt.Fprintf(cmd.ErrOrStderr(), i18n.G("Warning: thumbnail upload failed for %s: %v\n"), thumbPath, err)
					} else {
						fmt.Fprintf(cmd.OutOrStdout(), i18n.G("Uploaded thumbnail -> media_id: %s\n"), mediaID)
						cache.SetEntry(key, thumbBaseName, thumbURL, mediaID)
						thumbMediaID = mediaID
					}
				}
			}
			// Save cache after thumbnail upload
			if err := cache.Save(); err != nil {
				fmt.Fprintf(cmd.ErrOrStderr(), i18n.G("Warning: failed to save image cache: %v\n"), err)
			}
		} else {
			fmt.Fprintf(cmd.ErrOrStderr(), i18n.G("Warning: thumbnail file %q not found, passing value as-is "+
				"(make sure it's a valid media_id)\n"), thumbPath)
		}
	}
	// Replace local image paths with CDN URLs from upload cache (if available)
	htmlContent = replaceLocalImagePaths(htmlContent, file, cmd.ErrOrStderr())

	// Sanitize HTML before sending to WeChat (strip mailto links, footnote backlinks)
	sanitized := mdtowx.SanitizeHTML(htmlContent)

	// Create draft
	resp, err := draft.Add(token, []draft.Article{
		{
			Title:              title,
			Author:             author,
			ThumbMediaID:       thumbMediaID,
			Digest:             digest,
			ContentSourceURL:   contentSourceURL,
			NeedOpenComment:    needOpenComment,
			OnlyFansCanComment: onlyFansCanComment,
			Content:            string(sanitized),
		},
	})

	if err != nil {
		return fmt.Errorf("creating draft: %w", err)
	}

	fmt.Fprintf(cmd.OutOrStdout(), i18n.G("Draft created: %s\n"), color.GreenString(resp.MediaID))

	// Save media_id to sidecar YAML for future updates
	meta, _ := readSidecarYAML(file)
	meta.MediaID = resp.MediaID
	if err := writeSidecarYAML(file, meta); err != nil {
		fmt.Fprintf(cmd.ErrOrStderr(), i18n.G("Warning: failed to save media_id to sidecar YAML: %v\n"), err)
	}
	return nil
}

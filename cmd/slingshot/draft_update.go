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

// --- cmdDraftUpdate ---

// cmdDraftUpdate implements "slingshot draft update <file>".
type cmdDraftUpdate struct {
	global *cmdGlobal
	title  string
	thumb  string
	index  int
}

func (c *cmdDraftUpdate) command() *cobra.Command {
	cmd := &cobra.Command{}
	cmd.Use = "update " + u.File.Render()
	cmd.Short = i18n.G("Update an existing draft")
	cmd.Long = cli.FormatSection(
		color.CyanString("Description:"),
		i18n.G(`Update an existing WeChat draft with new HTML content.

The draft is identified by (in priority order):
  1. Sidecar YAML — if <file>.yaml/.yml exists with a "media_id" field
  2. 1st draft — if no sidecar is found, defaults to the first draft in the list

Usage:
  slingshot draft update <file>               # auto-detect from sidecar YAML or first draft

The --index flag specifies which article in a multi-article draft to update
(default 0, the first article). Use --thumb to update the cover image.`),
	)
	cmd.Flags().StringVarP(&c.title, "title", "t", "",
		i18n.G("New article title (default: auto-detect from HTML)"))
	cmd.Flags().StringVarP(&c.thumb, "thumb", "", "",
		i18n.G("New cover image media_id (optional; also detected from <meta name=\"thumb_media_id\"> in HTML)"))
	cmd.Flags().IntVarP(&c.index, "index", "i", 0,
		i18n.G("Article index in multi-article draft (0-based)"))
	cmd.RunE = c.run
	cmd.Args = cobra.ArbitraryArgs
	return cmd
}

func (c *cmdDraftUpdate) run(cmd *cobra.Command, args []string) error {
	parsed, err := c.global.Parse(draftUpdateUsage, cmd, args)
	if err != nil {
		return err
	}
	if len(parsed) < 1 || parsed[0].Skipped {
		return errors.New(i18n.G("expected <file> argument"))
	}

	// Load config and get token early (needed for ID resolution)
	cfg, _, err := config.Load()
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}
	token, err := getaccesstoken.GetToken(cfg)
	if err != nil {
		return fmt.Errorf("getting access token: %w", err)
	}

	// Resolve media_id and file path
	var mediaID string
	file := parsed[0].String
	meta, ok := readSidecarYAML(file)
	if ok && meta.MediaID != "" {
		mediaID = meta.MediaID
	} else {
		// No sidecar — default to the first draft in the list
		mediaID, err = resolveID(token, "1")
		if err != nil {
			return fmt.Errorf("resolving first draft: %w", err)
		}
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

	// Determine cover media_id (optional for update)
	thumbMediaID := c.thumb
	if thumbMediaID == "" {
		thumbMediaID = extractThumbMediaID(htmlStr, file)
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

	// Extract digest from sidecar YAML or HTML meta
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

	// Replace local image paths with CDN URLs from upload cache (if available)
	htmlContent = replaceLocalImagePaths(htmlContent, file, cmd.ErrOrStderr())

	// Sanitize HTML before sending to WeChat (strip mailto links, footnote backlinks)
	sanitized := mdtowx.SanitizeHTML(htmlContent)

	// Build article
	article := draft.Article{
		Title:              title,
		Author:             author,
		Digest:             digest,
		ContentSourceURL:   contentSourceURL,
		NeedOpenComment:    needOpenComment,
		OnlyFansCanComment: onlyFansCanComment,
		Content:            string(sanitized),
	}
	if thumbMediaID != "" {
		article.ThumbMediaID = thumbMediaID
	}

	// Update draft
	if err := draft.Update(token, mediaID, c.index, article); err != nil {
		return fmt.Errorf("updating draft: %w", err)
	}
	fmt.Fprintf(cmd.OutOrStdout(), i18n.G("Draft updated: %s\n"), color.GreenString(mediaID))

	// Save/update media_id in sidecar YAML for future 1-arg usage
	meta, _ = readSidecarYAML(file)

	meta.MediaID = mediaID
	if err := writeSidecarYAML(file, meta); err != nil {
		fmt.Fprintf(cmd.ErrOrStderr(), i18n.G("Warning: failed to save media_id to sidecar YAML: %v\n"), err)
	}
	return nil
}

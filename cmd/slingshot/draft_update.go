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

// cmdDraftUpdate implements "slingshot draft update <id> <file>".
type cmdDraftUpdate struct {
	global *cmdGlobal
	title  string
	thumb  string
	index  int
}

func (c *cmdDraftUpdate) command() *cobra.Command {
	cmd := &cobra.Command{}
	cmd.Use = "update " + u.ID.Render() + " " + u.File.Render()
	cmd.Short = i18n.G("Update an existing draft")
	cmd.Long = cli.FormatSection(
		color.CyanString("Description:"),
		i18n.G(`Update an existing WeChat draft by media ID with new HTML content.

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
	if len(parsed) < 2 || parsed[1].Skipped {
		return errors.New(i18n.G("expected id and file arguments"))
	}
	mediaID := parsed[0].String
	file := parsed[1].String

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
		thumbMediaID = extractThumbMediaID(htmlStr)
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
			// File exists — upload as permanent material
			cache := uploadcache.Load(baseDir)
			key, err := uploadcache.Key(thumbPath)
			if err != nil {
				fmt.Fprintf(cmd.ErrOrStderr(), i18n.G("Warning: checksum failed for thumbnail %s: %v\n"), thumbPath, err)
			} else {
				// Check cache first
				if cachedMediaID, ok := cache.GetMediaID(key); ok && cachedMediaID != "" {
					thumbMediaID = cachedMediaID
					fmt.Fprintf(cmd.OutOrStdout(), i18n.G("Using cached thumbnail %s -> %s\n"),
						filepath.Base(thumbPath), thumbMediaID)
				} else {
					fmt.Fprintf(cmd.OutOrStdout(), i18n.G("Uploading thumbnail %s...\n"), thumbPath)
					mediaID, err := uploadimage.UploadThumb(token, thumbPath)
					if err != nil {
						fmt.Fprintf(cmd.ErrOrStderr(), i18n.G("Warning: thumbnail upload failed for %s: %v\n"), thumbPath, err)
					} else {
						fmt.Fprintf(cmd.OutOrStdout(), i18n.G("Uploaded thumbnail -> media_id: %s\n"), mediaID)
						thumbMediaID = mediaID
						cache.SetMediaID(key, filepath.Base(thumbPath), mediaID)
					}
				}
				// Save cache after thumbnail upload
				if err := cache.Save(); err != nil {
					fmt.Fprintf(cmd.ErrOrStderr(), i18n.G("Warning: failed to save image cache: %v\n"), err)
				}
			}
		} else {
			fmt.Fprintf(cmd.ErrOrStderr(), i18n.G("Warning: thumbnail file %q not found, passing value as-is "+
				"(make sure it's a valid media_id)\n"), thumbPath)
		}
	}

	// Extract digest from sidecar YAML or HTML meta
	digest := extractDigest(htmlStr, file)

	// NeedOpenComment and OnlyFansCanComment: default to 1 (open, fans-only)
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

	// Build article
	article := draft.Article{
		Title:              title,
		Author:             author,
		Digest:             digest,
		NeedOpenComment:    needOpenComment,
		OnlyFansCanComment: onlyFansCanComment,
		Content:            string(htmlContent),
	}
	if thumbMediaID != "" {
		article.ThumbMediaID = thumbMediaID
	}

	// Update draft
	if err := draft.Update(token, mediaID, c.index, article); err != nil {
		return fmt.Errorf("updating draft: %w", err)
	}

	fmt.Fprintf(cmd.OutOrStdout(), i18n.G("Draft updated: %s\n"), color.GreenString(mediaID))
	return nil
}


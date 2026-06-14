package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/nanjj/slingshot/internal/config"
	"github.com/nanjj/slingshot/internal/getaccesstoken"
	"github.com/nanjj/slingshot/internal/i18n"
	"github.com/nanjj/slingshot/internal/mdtowx"
	"github.com/nanjj/slingshot/internal/uploadcache"
	"github.com/nanjj/slingshot/internal/uploadimage"
)

// runUpload implements the full upload pipeline for draft convert --upload.
func (c *cmdDraftConvert) runUpload(cmd *cobra.Command, result *mdtowx.Result, html []byte, outPath, sourceFile string) error {
	baseDir := filepath.Dir(sourceFile)

	// Step 2: Extract local image paths from HTML
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

		// Convert SVG to PNG if needed (WeChat does not support SVG)
		uploadPath, cleanup := maybeConvertSVG(ref.AbsPath, cmd.ErrOrStderr())

		// Not cached — upload to WeChat
		fmt.Fprintf(cmd.OutOrStdout(), i18n.G("Uploading %s...\n"), uploadPath)
		resp, err := uploadimage.Upload(token, uploadPath)
		if cleanup {
			os.Remove(uploadPath)
		}
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

	// Step 5: Handle thumbnail image (thumb_media_id from front matter)
	thumbMediaID := result.ThumbMediaID
	if thumbMediaID != "" {
		if looksLikeImageFile(thumbMediaID) {
			// Resolve relative to the markdown file's directory
			thumbPath := thumbMediaID
			if !filepath.IsAbs(thumbPath) {
				thumbPath = filepath.Join(baseDir, thumbPath)
			}
			thumbPath = filepath.Clean(thumbPath)

			if _, err := os.Stat(thumbPath); err == nil {
				// File exists — upload as permanent material
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
						// Convert SVG thumbnail to PNG if needed
						thumbUploadPath, thumbCleanup := maybeConvertSVG(thumbPath, cmd.ErrOrStderr())

						fmt.Fprintf(cmd.OutOrStdout(), i18n.G("Uploading thumbnail %s...\n"), thumbUploadPath)
						mediaID, err := uploadimage.UploadThumb(token, thumbUploadPath)
						if thumbCleanup {
							os.Remove(thumbUploadPath)
						}
						if err != nil {
							fmt.Fprintf(cmd.ErrOrStderr(), i18n.G("Warning: thumbnail upload failed for %s: %v\n"), thumbPath, err)
						} else {
							fmt.Fprintf(cmd.OutOrStdout(), i18n.G("Uploaded thumbnail -> media_id: %s\n"), mediaID)
							cache.SetMediaID(key, filepath.Base(thumbPath), mediaID)
							thumbMediaID = mediaID
						}
					}
					// Save cache after thumbnail upload (in case of partial updates)
					if err := cache.Save(); err != nil {
						fmt.Fprintf(cmd.ErrOrStderr(), i18n.G("Warning: failed to save image cache: %v\n"), err)
					}
				}
			} else {
				// File doesn't exist as a local path — fall through and use original value as-is
			}
		}
		// No image extension → treat as an already-valid media_id, keep it as-is
	}

	// Wrap with metadata and save
	outHTML := wrapHTML(result.Title, result.Author, thumbMediaID, updatedHTML)
	if err := os.WriteFile(outPath, outHTML, 0644); err != nil {
		return fmt.Errorf("writing output %q: %w", outPath, err)
	}

	fmt.Fprintf(cmd.OutOrStdout(), i18n.G("Upload complete, written to %s\n"), outPath)
	return nil
}

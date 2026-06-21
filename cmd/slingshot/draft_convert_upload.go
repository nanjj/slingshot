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

// runUploadPipeline runs the full upload pipeline for draft convert --upload.
// It uploads local images to WeChat, updates URLs, handles thumbnails, and
// saves the final HTML file. Standalone so it can be reused by auto-convert.
func runUploadPipeline(cmd *cobra.Command, result *mdtowx.Result, html []byte, outPath, sourceFile string) error {
	baseDir := filepath.Dir(sourceFile)

	// Remove spaces between non-ASCII (e.g., CJK) characters — common artifact from
	// markdown/org paragraph wrapping or manual editing (e.g., "相 信" -> "相信").
	html = mdtowx.RemoveCJCSpace(html)

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

	// Step 3: Load image upload cache (local + global) and upload (or skip cached) images
	cache := uploadcache.LoadCache(baseDir)

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

		baseName := filepath.Base(ref.AbsPath)

		// Check cache by md5 key first
		key, err := uploadcache.Key(ref.AbsPath)
		if err != nil {
			fmt.Fprintf(cmd.ErrOrStderr(), i18n.G("Warning: checksum failed for %s: %v\n"), ref.AbsPath, err)
			continue
		}

		if cachedURL, ok := cache.Get(key); ok && cachedURL != "" {
			fmt.Fprintf(cmd.OutOrStdout(), i18n.G("Using cached %s -> %s\n"), baseName, cachedURL)
			uploaded[ref.AbsPath] = cachedURL
			replacements[ref.Src] = cachedURL
			continue
		}

		// Check cache by filename — if the same filename was already uploaded
		// (e.g. as a thumbnail permanent material), reuse its URL to avoid
		// uploading the image again.
		if entry, ok := cache.GetByFilename(baseName); ok && entry.URL != "" {
			fmt.Fprintf(cmd.OutOrStdout(), i18n.G("Using cached %s (by filename) -> %s\n"), baseName, entry.URL)
			// Also cache under the new md5 key for future lookups
			cache.Set(key, baseName, entry.URL)
			uploaded[ref.AbsPath] = entry.URL
			replacements[ref.Src] = entry.URL
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

		fmt.Fprintf(cmd.OutOrStdout(), i18n.G("Uploaded %s -> %s\n"), baseName, resp.URL)
		cache.Set(key, baseName, resp.URL)
		uploaded[ref.AbsPath] = resp.URL
		replacements[ref.Src] = resp.URL
	}

	if len(replacements) == 0 {
		fmt.Fprintln(cmd.ErrOrStderr(), i18n.G("Warning: no images were uploaded, saving original HTML"))
	}

	// Save image upload cache (also syncs to global cache)
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
				thumbBaseName := filepath.Base(thumbPath)

				// File exists — upload as permanent material
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
							// Save both media_id and URL so content images with the same filename
							// can reuse the URL without a separate upload.
							cache.SetEntry(key, thumbBaseName, thumbURL, mediaID)
							thumbMediaID = mediaID
						}
					}
				}
				// Save cache after thumbnail upload (in case of partial updates)
				if err := cache.Save(); err != nil {
					fmt.Fprintf(cmd.ErrOrStderr(), i18n.G("Warning: failed to save image cache: %v\n"), err)
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

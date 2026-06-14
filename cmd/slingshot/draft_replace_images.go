package main

import (
	"fmt"
	"io"
	"path/filepath"

	"github.com/nanjj/slingshot/internal/i18n"
	"github.com/nanjj/slingshot/internal/mdtowx"
	"github.com/nanjj/slingshot/internal/uploadcache"
)

// replaceLocalImagePaths checks the upload cache for CDN URLs of local images
// in the HTML and replaces them. This handles the case where HTML was generated
// by 'convert' without --upload but the images are cached from a previous
// 'convert --upload' run.
//
// It only reads the cache — it does NOT upload images. If no cached URL is
// found for an image, it prints a warning and leaves the path unchanged.
// The user should run 'convert --upload' first to populate the cache.
func replaceLocalImagePaths(html []byte, file string, stderr io.Writer) []byte {
	baseDir := filepath.Dir(file)
	refs := mdtowx.ExtractImagePaths(html, baseDir)
	if len(refs) == 0 {
		return html
	}

	cache := uploadcache.Load(baseDir)
	replacements := make(map[string]string)

	for _, ref := range refs {
		key, err := uploadcache.Key(ref.AbsPath)
		if err != nil {
			fmt.Fprintf(stderr, i18n.G("Warning: checksum failed for %s: %v\n"), ref.AbsPath, err)
			continue
		}

		if cachedURL, ok := cache.Get(key); ok && cachedURL != "" {
			replacements[ref.Src] = cachedURL
		} else {
			fmt.Fprintf(stderr, i18n.G("Warning: no cached CDN URL for %s; run 'convert --upload' first\n"), ref.AbsPath)
		}
	}

	if len(replacements) == 0 {
		return html
	}

	return mdtowx.ReplaceImageURLs(html, replacements)
}

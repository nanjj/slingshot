// Package imgconv converts SVG images to PNG format for WeChat compatibility.
//
// WeChat official accounts do not support SVG images — they must be converted
// to a raster format (PNG) before uploading. This package shells out to
// rsvg-convert (from librsvg) for reliable, high-quality SVG→PNG conversion.
//
// rsvg-convert is a system dependency:
//
//	Debian/Ubuntu: apt install librsvg2-bin
//	macOS:         brew install librsvg
//	Arch Linux:    pacman -S librsvg
package imgconv

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// IsSVG reports whether the file path has an SVG extension (case-insensitive).
func IsSVG(path string) bool {
	ext := strings.ToLower(filepath.Ext(path))
	return ext == ".svg"
}

// ToPNG converts an SVG file to a temporary PNG file using rsvg-convert.
//
// The caller is responsible for removing the returned temp file when done.
// Returns an error if rsvg-convert is not installed or conversion fails.
func ToPNG(svgPath string) (pngPath string, err error) {
	// Create temp file for the PNG output
	f, err := os.CreateTemp("", "slingshot-svg-*.png")
	if err != nil {
		return "", fmt.Errorf("creating temp PNG file: %w", err)
	}
	pngPath = f.Name()
	if err := f.Close(); err != nil {
		os.Remove(pngPath)
		return "", fmt.Errorf("closing temp file: %w", err)
	}

	// Run rsvg-convert
	cmd := exec.Command("rsvg-convert",
		"-f", "png",
		"-o", pngPath,
		svgPath,
	)
	if output, err := cmd.CombinedOutput(); err != nil {
		os.Remove(pngPath)
		return "", fmt.Errorf("rsvg-convert failed: %w\n%s", err, output)
	}

	return pngPath, nil
}

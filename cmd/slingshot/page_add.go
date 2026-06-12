package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/fatih/color"
	"github.com/spf13/cobra"

	cli "github.com/nanjj/slingshot/internal/cmd"
	"github.com/nanjj/slingshot/internal/config"
	"github.com/nanjj/slingshot/internal/i18n"
	u "github.com/nanjj/slingshot/internal/usage"
)

// imgSrcRe matches <img src="..."> and <img src='...'> attributes.
var imgSrcRe = regexp.MustCompile(`<img[^>]+src=["']([^"']+)["']`)

// --- cmdPageAdd ---

// cmdPageAdd implements both "slingshot page add <site> <file>"
// and "slingshot page update <site> <file>".
type cmdPageAdd struct {
	global *cmdGlobal
	update bool // false = add, true = update
}

func (c *cmdPageAdd) command() *cobra.Command {
	cmd := &cobra.Command{}
	if c.update {
		cmd.Use = "update " + u.Name.Render() + " " + u.File.Render()
		cmd.Short = i18n.G("Update an existing page from HTML file")
		cmd.Long = cli.FormatSection(
			color.CyanString("Description:"),
			i18n.G(`Update an existing page in a deployment site with new content.

The page name is derived from the HTML filename (without extension).
The page directory named <page-name> must already exist in the site directory.

Images referenced in the HTML (<img src="...">) are copied from their
source locations into the page directory.`),
		)
	} else {
		cmd.Use = "add " + u.Name.Render() + " " + u.File.Render()
		cmd.Short = i18n.G("Add a new page from HTML file")
		cmd.Long = cli.FormatSection(
			color.CyanString("Description:"),
			i18n.G(`Add a new page to a deployment site from an HTML file.

The page name is derived from the HTML filename (without extension).
A new subdirectory named <page-name> is created under the site's directory.

Images referenced in the HTML (<img src="...">) are copied from their
source locations into the page directory.

The site's index.html is automatically regenerated after adding the page.`),
		)
	}
	cmd.RunE = c.run
	cmd.Args = cobra.ArbitraryArgs
	return cmd
}

func (c *cmdPageAdd) run(cmd *cobra.Command, args []string) error {
	usage := pageAddUsage
	if c.update {
		usage = pageUpdateUsage
	}

	parsed, err := c.global.Parse(usage, cmd, args)
	if err != nil {
		return err
	}
	if len(parsed) < 2 || parsed[1].Skipped {
		return errors.New(i18n.G("expected site and file arguments"))
	}
	siteName := parsed[0].String
	filePath := parsed[1].String

	// Validate source file exists
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		return fmt.Errorf(i18n.G("file not found: %s"), filePath)
	}

	// If source is an Org file, convert to HTML using Emacs first
	if strings.HasSuffix(filePath, ".org") {
		htmlPath, err := orgToHTMLFile(filePath)
		if err != nil {
			return err
		}
		fmt.Fprintf(cmd.OutOrStdout(), "%s %s -> %s\n",
			color.CyanString("→"), filepath.Base(filePath), filepath.Base(htmlPath))
		filePath = htmlPath
	}

	// Read source HTML
	htmlContent, err := os.ReadFile(filePath)
	if err != nil {
		return fmt.Errorf("reading %q: %w", filePath, err)
	}

	// Load config and get site
	cfg, _, err := config.Load()
	if err != nil {
		return fmt.Errorf("%s: %w", i18n.G("loading config"), err)
	}

	site, ok := config.GetSite(cfg, siteName)
	if !ok {
		return fmt.Errorf(i18n.G("site %q not found"), siteName)
	}

	if site.Dir == "" {
		return fmt.Errorf(i18n.G("site %q has no directory configured"), siteName)
	}

	// Derive page name from filename (strip extension)
	pageName := strings.TrimSuffix(filepath.Base(filePath), filepath.Ext(filePath))
	if pageName == "" {
		return errors.New(i18n.G("invalid page name derived from filename"))
	}

	// Validate page name: only allow safe characters
	if !isValidPageName(pageName) {
		return fmt.Errorf(i18n.G("invalid page name %q: use only letters, numbers, hyphens, and underscores"), pageName)
	}

	pageDir := filepath.Join(site.Dir, pageName)

	// Check existence based on mode
	_, dirErr := os.Stat(pageDir)
	if c.update {
		if os.IsNotExist(dirErr) {
			return fmt.Errorf(i18n.G("page %q does not exist in site %q"), pageName, siteName)
		}
	} else {
		if dirErr == nil {
			return fmt.Errorf(i18n.G("page %q already exists in site %q"), pageName, siteName)
		}
	}

	// Create page directory
	if err := os.MkdirAll(pageDir, 0755); err != nil {
		return fmt.Errorf("creating page directory: %w", err)
	}

	// Write index.html
	indexPath := filepath.Join(pageDir, "index.html")
	if err := os.WriteFile(indexPath, htmlContent, 0644); err != nil {
		return fmt.Errorf("writing index.html: %w", err)
	}

	// Extract and copy image assets
	sourceDir := filepath.Dir(filePath)
	copied, err := copyPageImages(htmlContent, sourceDir, pageDir)
	if err != nil {
		// Non-fatal: warn but continue
		fmt.Fprintf(cmd.ErrOrStderr(), "%s %v\n",
			color.YellowString(i18n.G("Warning:")), err)
	}
	if copied > 0 {
		fmt.Fprintf(cmd.OutOrStdout(), "%s %d %s\n",
			color.CyanString("→"), copied, i18n.G("image(s) copied"))
	}

	// Regenerate site index
	if err := regenerateSiteIndex(site.Dir); err != nil {
		return fmt.Errorf("regenerating site index: %w", err)
	}

	verb := i18n.G("Added")
	if c.update {
		verb = i18n.G("Updated")
	}
	fmt.Fprintf(color.Output, "%s %s/%s\n", color.GreenString(verb+":"), siteName, color.GreenString(pageName))
	return nil
}

// copyPageImages finds all <img src="..."> references in the HTML and copies
// local image files to the page directory, preserving subdirectory structure.
func copyPageImages(html []byte, sourceDir, pageDir string) (int, error) {
	htmlStr := string(html)
	matches := imgSrcRe.FindAllStringSubmatch(htmlStr, -1)
	if len(matches) == 0 {
		return 0, nil
	}

	var errs []string
	copied := 0

	for _, m := range matches {
		src := m[1]
		if src == "" {
			continue
		}

		// Skip absolute URLs and data URIs
		if strings.HasPrefix(src, "http://") || strings.HasPrefix(src, "https://") ||
			strings.HasPrefix(src, "data:") || strings.HasPrefix(src, "//") {
			continue
		}

		// Resolve source path relative to the HTML file's directory
		srcPath := src
		if !filepath.IsAbs(srcPath) {
			srcPath = filepath.Join(sourceDir, srcPath)
		}
		srcPath = filepath.Clean(srcPath)

		// Check if source file exists
		if _, err := os.Stat(srcPath); os.IsNotExist(err) {
			errs = append(errs, fmt.Sprintf("image not found: %s", src))
			continue
		}

		// Determine destination path preserving subdirectory structure
		dstPath := filepath.Join(pageDir, src)
		dstDir := filepath.Dir(dstPath)
		if err := os.MkdirAll(dstDir, 0755); err != nil {
			errs = append(errs, fmt.Sprintf("creating directory for %s: %v", src, err))
			continue
		}

		// Copy file
		if err := copyFile(srcPath, dstPath); err != nil {
			errs = append(errs, fmt.Sprintf("copying %s: %v", src, err))
			continue
		}
		copied++
	}

	if len(errs) > 0 {
		return copied, fmt.Errorf("image copy warnings:\n  %s", strings.Join(errs, "\n  "))
	}
	return copied, nil
}

// copyFile copies a file from src to dst.
func copyFile(src, dst string) error {
	srcFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer srcFile.Close()

	dstFile, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer dstFile.Close()

	if _, err := io.Copy(dstFile, srcFile); err != nil {
		return err
	}
	return dstFile.Sync()
}

// regenerateSiteIndex regenerates the index.html in the site root directory.
// It lists all page subdirectories containing index.html and creates a
// simple listing page with links to each page.
func regenerateSiteIndex(siteDir string) error {
	pages, err := listPages(siteDir)
	if err != nil {
		return fmt.Errorf("listing pages: %w", err)
	}

	var sb strings.Builder
	sb.WriteString("<!DOCTYPE html>\n")
	sb.WriteString("<html>\n<head>\n")
	sb.WriteString(`<meta charset="utf-8">` + "\n")
	sb.WriteString("<title>" + htmlEscape(filepath.Base(siteDir)) + "</title>\n")
	sb.WriteString("</head>\n<body>\n")

	if len(pages) == 0 {
		sb.WriteString("<p>No pages yet.</p>\n")
	} else {
		sb.WriteString("<ul>\n")
		for _, p := range pages {
			title := p.Title
			if title == "" {
				title = p.Name
			}
			sb.WriteString(fmt.Sprintf("  <li><a href=\"%s/\">%s</a></li>\n",
				htmlEscape(p.Name), htmlEscape(title)))
		}
		sb.WriteString("</ul>\n")
	}

	sb.WriteString("</body>\n</html>\n")

	indexPath := filepath.Join(siteDir, "index.html")
	if err := os.WriteFile(indexPath, []byte(sb.String()), 0644); err != nil {
		return fmt.Errorf("writing site index: %w", err)
	}

	return nil
}

// isValidPageName checks that a page name contains only safe characters.
func isValidPageName(name string) bool {
	if name == "" {
		return false
	}
	for _, r := range name {
		if !((r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') ||
			(r >= '0' && r <= '9') || r == '-' || r == '_' || r == '.') {
			return false
		}
	}
	return true
}

// orgToHTMLFile converts an Org file to HTML using Emacs batch mode.
// It calls emacs --batch with org-html-export-to-html, which writes
// the .html file alongside the .org file. Returns the HTML file path.
func orgToHTMLFile(orgPath string) (string, error) {
	if _, err := exec.LookPath("emacs"); err != nil {
		return "", fmt.Errorf(i18n.G("emacs not found: %w")+
			"\n"+i18n.G("Org-to-HTML conversion requires GNU Emacs (>= 26.1) with Org mode. "+
				"Install it or use an .html file instead."), err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	var stderr bytes.Buffer
	cmd := exec.CommandContext(ctx, "emacs",
		"--batch",
		"--visit="+orgPath,
		// Suppress the postamble (<div id="postamble">) — the auto-generated
		// date/author/validation boilerplate doesn't suit our page layout.
		"--eval", "(setq org-html-postamble nil)",
		"--eval", "(setq org-html-validation-link nil)",
		"--eval", "(org-html-export-to-html)",
	)
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return "", errors.New(i18n.G("emacs org-to-html conversion timed out (30s)"))
		}
		errMsg := stderr.String()
		if len(errMsg) > 1024 {
			errMsg = errMsg[:1024] + "... (truncated)"
		}
		return "", fmt.Errorf(i18n.G("emacs org-to-html conversion failed: %w\nstderr: %s"), err, errMsg)
	}

	// The HTML file is created alongside the org file with .html extension
	base := strings.TrimSuffix(orgPath, filepath.Ext(orgPath))
	htmlPath := base + ".html"

	if _, err := os.Stat(htmlPath); os.IsNotExist(err) {
		return "", fmt.Errorf(i18n.G("emacs did not produce expected HTML file: %s"), htmlPath)
	}

	return htmlPath, nil
}

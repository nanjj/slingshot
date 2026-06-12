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
	"github.com/nanjj/slingshot/internal/site"
	u "github.com/nanjj/slingshot/internal/usage"
)

// imgSrcRe matches <img src="..."> and <img src='...'> attributes.
var imgSrcRe = regexp.MustCompile(`<img[^>]+src=["']([^"']+)["']`)

// dateParagraphRe matches <p class="date">...</p> inserted by Emacs org export.
var dateParagraphRe = regexp.MustCompile(`(?i)<p\s+class="date"[^>]*>.*?</p>\s*`)

// slingshotDateRe matches the embedded date comment written by page add.
// Format: <!-- slingshot-date: YYYY-MM-DD -->
var slingshotDateRe = regexp.MustCompile(`(?i)<!--\s*slingshot-date:\s*(\d{4}-\d{2}-\d{2})\s*-->`)
// --- cmdPageAdd ---

// cmdPageAdd implements both "slingshot page add <site> <file>"
// and "slingshot page update <site> <file>".
type cmdPageAdd struct {
	global    *cmdGlobal
	update    bool // false = add, true = update
	flagRsync bool // --rsync: deploy site via rsync after success
}

func (c *cmdPageAdd) command() *cobra.Command {
	cmd := &cobra.Command{}
	if c.update {
		cmd.Use = "update " + u.Name.Render() + " " + u.File.List(1).Render()
		cmd.Short = i18n.G("Update existing pages from HTML files")
		cmd.Long = cli.FormatSection(
			color.CyanString("Description:"),
			i18n.G(`Update existing pages in a deployment site with new content.

Each page name is derived from its HTML filename (without extension).
The page directory named <page-name> must already exist in the site directory.

Images referenced in the HTML (<img src="...">) are copied from their
source locations into the page directory.

The site's index.html is regenerated once after all pages are processed.

Use --rsync to automatically deploy the site after the update.`),
		)
	} else {
		cmd.Use = "add " + u.Name.Render() + " " + u.File.List(1).Render()
		cmd.Short = i18n.G("Add new pages from HTML files")
		cmd.Long = cli.FormatSection(
			color.CyanString("Description:"),
			i18n.G(`Add new pages to a deployment site from HTML files.

Each page name is derived from its HTML filename (without extension).
A new subdirectory named <page-name> is created under the site's directory.

Images referenced in the HTML (<img src="...">) are copied from their
source locations into the page directory.

The site's index.html is regenerated once after all pages are processed.

Use --rsync to automatically deploy the site after adding the pages.`),
		)
	}
	cmd.Flags().BoolVar(&c.flagRsync, "rsync", false, i18n.G("Deploy site via rsync after adding/updating pages"))
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
	if len(parsed) < 2 || parsed[0].Skipped {
		return errors.New(i18n.G("expected site name"))
	}
	if len(parsed) < 2 || parsed[1].Skipped || len(parsed[1].StringList) == 0 {
		return errors.New(i18n.G("expected at least one file argument"))
	}
	siteName := parsed[0].String
	filePaths := parsed[1].StringList

	// Load config and get site (once, before processing files)
	cfg, _, err := config.Load()
	if err != nil {
		return fmt.Errorf("%s: %w", i18n.G("loading config"), err)
	}
	siteConfig, ok := config.GetSite(cfg, siteName)
	if !ok {
		return fmt.Errorf(i18n.G("site %q not found"), siteName)
	}
	if siteConfig.Dir == "" {
		return fmt.Errorf(i18n.G("site %q has no directory configured"), siteName)
	}

	// Ensure shared style.css exists in the site directory
	if err := site.EnsureCSS(siteConfig.Dir); err != nil {
		return fmt.Errorf("ensuring site style.css: %w", err)
	}

	var processed int
	for _, filePath := range filePaths {
		// Validate source file exists
		if _, err := os.Stat(filePath); os.IsNotExist(err) {
			return fmt.Errorf(i18n.G("file not found: %s"), filePath)
		}
		// Save original path before possible .org -> .html conversion
		originalPath := filePath

		// If source is an Org file, convert to HTML using Emacs first
		var wasOrg bool
		if strings.HasSuffix(filePath, ".org") {
			wasOrg = true
			htmlPath, err := orgToHTMLFile(filePath)
			if err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "%s %s -> %s\n",
				color.CyanString("\xe2\x86\x92"), filepath.Base(filePath), filepath.Base(htmlPath))
			filePath = htmlPath
		}

		// Read source HTML
		htmlContent, err := os.ReadFile(filePath)
		if err != nil {
			return fmt.Errorf("reading %q: %w", filePath, err)
		}

		// Strip <p class="date">...</p> inserted by Emacs org export
		htmlContent = stripDateParagraphs(htmlContent)
		// If source was Org, extract #+DATE: and embed it as a machine-readable comment
		if wasOrg {
			pageDate := extractOrgDate(originalPath)
			if !pageDate.IsZero() {
				htmlContent = embedDateComment(htmlContent, pageDate)
			}
		}

		// Inject <link> to the shared stylesheet
		htmlContent = injectStyleLink(htmlContent, "/style.css?v="+site.CSSVersion)

		// Inject site title heading as a clickable link to the homepage
		htmlContent = injectSiteTitle(htmlContent, siteConfig.Title)
		// Downgrade the article's <h1 class="title"> to <h2>
		htmlContent = downgradeTitleHeading(htmlContent)

		// Derive page name from filename (strip extension)
		pageName := strings.TrimSuffix(filepath.Base(filePath), filepath.Ext(filePath))
		if pageName == "" {
			return errors.New(i18n.G("invalid page name derived from filename"))
		}

		// Validate page name: only allow safe characters
		if !isValidPageName(pageName) {
			return fmt.Errorf(i18n.G("invalid page name %q: use only letters, numbers, hyphens, and underscores"), pageName)
		}

		pageDir := filepath.Join(siteConfig.Dir, pageName)

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
			fmt.Fprintf(cmd.ErrOrStderr(), "%s %v\n",
				color.YellowString(i18n.G("Warning:")), err)
		}
		if copied > 0 {
			fmt.Fprintf(cmd.OutOrStdout(), "%s %d %s\n",
				color.CyanString("\xe2\x86\x92"), copied, i18n.G("image(s) copied"))
		}

		verb := i18n.G("Added")
		if c.update {
			verb = i18n.G("Updated")
		}
		fmt.Fprintf(color.Output, "%s %s/%s\n", color.GreenString(verb+":"), siteName, color.GreenString(pageName))
		processed++
	}

	if processed == 0 {
		return errors.New(i18n.G("no files processed"))
	}

	// Regenerate site index (once, after all pages)
	if err := regenerateSiteIndex(siteConfig.Dir, siteName, siteConfig.Title); err != nil {
		return fmt.Errorf("regenerating site index: %w", err)
	}

	// --rsync: deploy site via rsync after success
	if c.flagRsync {
		if siteConfig.Rsync == "" {
			return fmt.Errorf(i18n.G("site %q has no rsync command configured"), siteName)
		}
		fmt.Fprintf(color.Output, "\n%s %s\n", i18n.G("Deploying via rsync for site:"), color.GreenString(siteName))
		if err := doRsync(siteConfig.Dir, siteConfig.Rsync); err != nil {
			return err
		}
		fmt.Fprintf(color.Output, "%s %s\n", color.GreenString("\xe2\x9c\x93"), i18n.G("Rsync completed successfully."))
	}

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
// date-grouped listing page with links to each page, using the format:
//
//	<h1>{siteTitle}</h1>
//	<section class="article-grid">
//	  <div class="article-date">June 05, 2026</div>
//	  <div class="link">
//	    <a href="/page-name/">Page Title</a>
//	  </div>
//	</section>
//
// siteTitle can be empty — in that case it falls back to "{siteName}'s blog".
func regenerateSiteIndex(siteDir, siteName, siteTitle string) error {
	pages, err := listPages(siteDir)
	if err != nil {
		return fmt.Errorf("listing pages: %w", err)
	}

	title := siteTitle
	if title == "" {
		title = siteName + "'s blog"
	}

	// Ensure shared style.css exists
	if err := site.EnsureCSS(siteDir); err != nil {
		return fmt.Errorf("ensuring site style.css: %w", err)
	}

	var sb strings.Builder
	sb.WriteString("<!DOCTYPE html>\n")
	sb.WriteString("<html lang=\"en\">\n<head>\n")
	sb.WriteString(`<meta charset="utf-8">` + "\n")
	sb.WriteString(`<meta name="viewport" content="width=device-width, initial-scale=1">` + "\n")
	sb.WriteString("<title>" + htmlEscape(title) + "</title>\n")
	sb.WriteString(`<link rel="stylesheet" href="style.css?v=` + site.CSSVersion + `" />` + "\n")
	sb.WriteString("</head>\n<body>\n")

	if len(pages) == 0 {
		sb.WriteString("  <h1><a href=\"/\">" + htmlEscape(title) + "</a> </h1>\n")
		sb.WriteString("  <p>No pages yet.</p>\n")
	} else {
		sb.WriteString("  <h1><a href=\"/\">" + htmlEscape(title) + "</a> </h1>\n")
		sb.WriteString("  <section class=\"article-grid\">\n")

		var lastDate string
		for _, p := range pages {
			dateStr := ""
			if !p.Date.IsZero() {
				dateStr = p.Date.Format("January 02, 2006")
			}

			// Print date heading when it changes
			if dateStr != lastDate {
				sb.WriteString("\n")
				if dateStr != "" {
					sb.WriteString("    <div class=\"article-date\">" + htmlEscape(dateStr) + "</div>\n")
				} else {
					sb.WriteString("    <div class=\"article-date\">Undated</div>\n")
				}
				lastDate = dateStr
			}

			ptitle := p.Title
			if ptitle == "" {
				ptitle = p.Name
			}
			sb.WriteString("    <div class=\"link\">\n")
			sb.WriteString(fmt.Sprintf("      <a href=\"/%s/\">%s</a>\n",
				htmlEscape(p.Name), htmlEscape(ptitle)))
			sb.WriteString("    </div>\n")
		}

		sb.WriteString("  </section>\n")
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

// --- HTML cleanup and date helpers ---

// stripDateParagraphs removes <p class="date">...</p> elements from HTML
// content. These are generated by Emacs org-export as part of the postamble
// and don't suit our page layout.
func stripDateParagraphs(html []byte) []byte {
	return dateParagraphRe.ReplaceAll(html, nil)
}

// extractOrgDate reads the #+DATE: property from an Org file and returns
// the parsed date. Returns zero time if not found or unparseable.
func extractOrgDate(orgPath string) time.Time {
	data, err := os.ReadFile(orgPath)
	if err != nil {
		return time.Time{}
	}
	return parseOrgDate(string(data))
}

// parseOrgDate scans content for #+DATE: and tries to parse the value.
// Supports common Org timestamp formats:
//
//	#+DATE: <2007-06-18 Mon>
//	#+DATE: <2007-06-18>
//	#+DATE: <Monday, 18 June 2007>
//	#+DATE: <Mon, 18 Jun 2007>
//	#+DATE: <18 June 2007>
//	#+DATE: 2007-06-18
//	#+DATE: Monday, 18 June 2007
//	#+DATE: June 18, 2007
func parseOrgDate(content string) time.Time {
	for _, line := range strings.Split(content, "\n") {
		trimmed := strings.TrimSpace(line)
		// Org mode property keys are case-insensitive; match #+date: regardless of case.
		if !strings.HasPrefix(strings.ToUpper(trimmed), "#+DATE:") {
			continue
		}
		dateStr := strings.TrimSpace(trimmed[len("#+DATE:"):])
		if dateStr == "" {
			continue
		}
		// Try common Org date formats
		formats := []string{
			"<2006-01-02 Mon>",
			"<2006-01-02>",
			"<Monday, 2 January 2006>",
			"<Mon, 2 Jan 2006>",
			"<2 January 2006>",
			"2006-01-02",
			"Monday, 2 January 2006",
			"January 2, 2006",
			"2006-01-02 15:04:05",
		}
		for _, f := range formats {
			if t, err := time.Parse(f, dateStr); err == nil {
				return t
			}
		}
		// Found #+DATE: but couldn't parse — stop looking
		break
	}
	return time.Time{}
}

// embedDateComment prepends a machine-readable date comment to HTML content
// so the site index generator can use it for date-grouped rendering.
func embedDateComment(html []byte, date time.Time) []byte {
	comment := fmt.Sprintf("<!-- slingshot-date: %s -->\n", date.Format("2006-01-02"))
	return append([]byte(comment), html...)
}

// orgToHTMLFile converts an Org file to HTML using Emacs batch mode.
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
		// Suppress boilerplate that Org export emits by default:
		// - Postamble (<div id="postamble">) with date/author/validation links
		// - XML declaration (<?xml version="1.0" encoding="utf-8"?>)
		// - XHTML doctype — use clean HTML5 <!DOCTYPE html> instead
		// - Default inline <style> — we use a shared style.css instead
		"--eval", "(setq org-html-postamble nil)",
		"--eval", "(setq org-html-validation-link nil)",
		"--eval", "(setq org-html-xml-declaration nil)",
		"--eval", "(setq org-html-doctype \"html5\")",
		"--eval", "(setq org-html-head-include-default-style nil)",
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

// injectStyleLink inserts a <link rel="stylesheet" href="..."> tag into the
// <head> section of the HTML, right before </head>. If the link is already
// present it is a no-op.
func injectStyleLink(html []byte, href string) []byte {
	link := fmt.Sprintf(`<link rel="stylesheet" href="%s" />`, href)
	headEnd := bytes.Index(html, []byte("</head>"))
	if headEnd < 0 {
		// No </head> — prepend before </html> as fallback
		htmlEnd := bytes.Index(html, []byte("</html>"))
		if htmlEnd < 0 {
			return append(html, []byte(link+"\n")...)
		}
		return []byte(string(html[:htmlEnd]) + link + "\n" + string(html[htmlEnd:]))
	}
	// Check if the link is already present
	if bytes.Contains(html[:headEnd], []byte(href)) {
		return html
	}
	return []byte(string(html[:headEnd]) + "  " + link + "\n" + string(html[headEnd:]))
}

// injectSiteTitle inserts <h1><a href="/">title</a> </h1> right after <body>
// so every page has a clickable site-title link back to the homepage.
func injectSiteTitle(html []byte, title string) []byte {
	link := fmt.Sprintf("<h1><a href=\"/\">%s</a> </h1>\n", htmlEscape(title))

	bodyTag := []byte("<body")
	idx := bytes.Index(html, bodyTag)
	if idx < 0 {
		return html
	}

	// Find the closing > of the <body> tag
	rest := html[idx:]
	gtIdx := bytes.IndexByte(rest, '>')
	if gtIdx < 0 {
		return html
	}

	insertPos := idx + gtIdx + 1
	result := make([]byte, 0, len(html)+len(link)+1)
	result = append(result, html[:insertPos]...)
	result = append(result, '\n')
	result = append(result, link...)
	result = append(result, html[insertPos:]...)
	return result
}

// downgradeTitleHeading changes <h1 class="title"> to <h2 class="title">
// and the corresponding </h1> to </h2>. This is called after injectSiteTitle
// so that the page has exactly one <h1> (the site title link).
func downgradeTitleHeading(html []byte) []byte {
	startOld := []byte(`<h1 class="title">`)
	startNew := []byte(`<h2 class="title">`)
	closeOld := []byte(`</h1>`)
	closeNew := []byte(`</h2>`)

	// Find opening tag
	idx := bytes.Index(html, startOld)
	if idx < 0 {
		return html
	}

	// Build result: replace opening tag
	var buf bytes.Buffer
	buf.Write(html[:idx])
	buf.Write(startNew)
	rest := html[idx+len(startOld):]

	// Find matching closing tag
	closeIdx := bytes.Index(rest, closeOld)
	if closeIdx < 0 {
		buf.Write(rest)
		return buf.Bytes()
	}

	buf.Write(rest[:closeIdx])
	buf.Write(closeNew)
	buf.Write(rest[closeIdx+len(closeOld):])
	return buf.Bytes()
}

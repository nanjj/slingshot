package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"html"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"
	"unicode"

	"github.com/fatih/color"
	"github.com/spf13/cobra"

	cli "github.com/nanjj/slingshot/internal/cmd"
	"github.com/nanjj/slingshot/internal/config"
	"github.com/nanjj/slingshot/internal/highlight"
	"github.com/nanjj/slingshot/internal/i18n"
	"github.com/nanjj/slingshot/internal/mdtowx"
	"github.com/nanjj/slingshot/internal/site"
	u "github.com/nanjj/slingshot/internal/usage"
)

// imgSrcRe matches <img src="..."> and <img src='...'> attributes.
var imgSrcRe = regexp.MustCompile(`<img[^>]+src=["']([^"']+)["']`)

// dateParagraphRe matches <p class="date">...</p> inserted by Emacs org export.
// The class attribute is matched anywhere in the start tag, so a paragraph
// carrying extra attributes (id, style, ...) is still recognized.
var dateParagraphRe = regexp.MustCompile(`(?is)<p\s[^>]*class="date"[^>]*>.*?</p>\s*`)

// slingshotDateRe matches the embedded date comment written by page add.

// Format: <!-- slingshot-date: YYYY-MM-DD -->
var slingshotDateRe = regexp.MustCompile(`(?i)<!--\s*slingshot-date:\s*(\d{4}-\d{2}-\d{2})\s*-->`)

// dateCommentLineRe matches a slingshot-date comment plus one trailing
// newline, so stripping it does not leave a blank line behind.
var dateCommentLineRe = regexp.MustCompile(`(?i)<!--\s*slingshot-date:\s*\d{4}-\d{2}-\d{2}\s*-->\n?`)

// dateParagraphBodyRe captures the body of <p class="date">…</p> emitted by
// Emacs org export. Unlike dateParagraphRe it captures the text, because the
// date it carries is one of the clues resolvePageDate falls back to.
var dateParagraphBodyRe = regexp.MustCompile(`(?is)<p\s[^>]*class="date"[^>]*>(.*?)</p>`)

// tagRe matches a single HTML tag; used to reduce a <p> body to plain text.
var tagRe = regexp.MustCompile(`<[^>]*>`)

// pDateFormats are the lenient layouts accepted for <p class="date"> text.
var pDateFormats = []string{
	"January 2, 2006",
	"Jan 2, 2006",
	"Monday, January 2, 2006",
	"Mon, Jan 2, 2006",
	"2 January 2006",
	"2006-01-02",
}

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
	siteName, filePaths, err := c.parseArgs(cmd, args)
	if err != nil {
		return err
	}

	siteConfig, err := loadSiteConfig(siteName)
	if err != nil {
		return err
	}

	// Ensure site has an optimized style.css (creates if missing, upgrades if old)
	if _, err := site.UpgradeCSS(siteConfig.Dir, false); err != nil {
		return fmt.Errorf("ensuring site style.css: %w", err)
	}

	var processed int
	for _, filePath := range filePaths {
		pageName, err := c.processFile(cmd, filePath, siteConfig)
		if err != nil {
			return err
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
		if err := deploySite(siteName, siteConfig); err != nil {
			return err
		}
	}

	return nil
}

// deploySite runs the site's configured rsync command after a successful
// add/update. Extracted from run so the deploy step reads as one call.
func deploySite(siteName string, siteConfig config.Site) error {
	if siteConfig.Rsync == "" {
		return fmt.Errorf(i18n.G("site %q has no rsync command configured"), siteName)
	}
	fmt.Fprintf(color.Output, "\n%s %s\n", i18n.G("Deploying via rsync for site:"), color.GreenString(siteName))
	if err := doRsync(siteConfig.Dir, siteConfig.Rsync); err != nil {
		return err
	}
	fmt.Fprintf(color.Output, "%s %s\n", color.GreenString("\xe2\x9c\x93"), i18n.G("Rsync completed successfully."))
	return nil
}

// parseArgs parses the command line and returns the site name and the list of
// source files. Extracted from run to keep the entry point a step sequence.
func (c *cmdPageAdd) parseArgs(cmd *cobra.Command, args []string) (string, []string, error) {
	usage := pageAddUsage
	if c.update {
		usage = pageUpdateUsage
	}

	parsed, err := c.global.Parse(usage, cmd, args)
	if err != nil {
		return "", nil, err
	}
	if len(parsed) < 2 || parsed[0].Skipped {
		return "", nil, errors.New(i18n.G("expected site name"))
	}
	if parsed[1].Skipped || len(parsed[1].StringList) == 0 {
		return "", nil, errors.New(i18n.G("expected at least one file argument"))
	}
	return parsed[0].String, parsed[1].StringList, nil
}

// loadSiteConfig loads the configuration and returns the named site, failing
// when the site is missing or has no directory configured.
func loadSiteConfig(siteName string) (config.Site, error) {
	cfg, _, err := config.Load()
	if err != nil {
		return config.Site{}, fmt.Errorf("%s: %w", i18n.G("loading config"), err)
	}
	siteConfig, ok := config.GetSite(cfg, siteName)
	if !ok {
		return config.Site{}, fmt.Errorf(i18n.G("site %q not found"), siteName)
	}
	if siteConfig.Dir == "" {
		return config.Site{}, fmt.Errorf(i18n.G("site %q has no directory configured"), siteName)
	}
	return siteConfig, nil
}

// processFile runs the whole per-file pipeline for one source path: Org
// conversion, read, page-name derivation and validation, date resolution, HTML
// post-processing, index.html write and image copying. It returns the derived
// page name. Extracted from run so each step stays individually readable.
func (c *cmdPageAdd) processFile(cmd *cobra.Command, filePath string, siteConfig config.Site) (string, error) {
	// Validate source file exists
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		return "", fmt.Errorf(i18n.G("file not found: %s"), filePath)
	}
	// Save original path before possible .org -> .html conversion
	originalPath := filePath

	// If source is an Org file, convert to HTML using Emacs first
	var wasOrg bool
	if strings.HasSuffix(filePath, ".org") {
		wasOrg = true
		htmlPath, err := orgToHTMLFile(filePath)
		if err != nil {
			return "", err
		}
		fmt.Fprintf(cmd.OutOrStdout(), "%s %s -> %s\n",
			color.CyanString("\xe2\x86\x92"), filepath.Base(filePath), filepath.Base(htmlPath))
		filePath = htmlPath
	}

	// Read source HTML
	content, err := os.ReadFile(filePath)
	if err != nil {
		return "", fmt.Errorf("reading %q: %w", filePath, err)
	}

	// Derive page name from filename (strip extension) and validate it
	pageName := strings.TrimSuffix(filepath.Base(filePath), filepath.Ext(filePath))
	if pageName == "" {
		return "", errors.New(i18n.G("invalid page name derived from filename"))
	}
	if !isValidPageName(pageName) {
		return "", fmt.Errorf(i18n.G("invalid page name %q: use only letters, numbers, hyphens, and underscores"), pageName)
	}

	pageDir := filepath.Join(siteConfig.Dir, pageName)

	// Resolve and freeze the page date before any content rewrite.
	content = applyPageDate(content, wasOrg, originalPath, siteConfig.Dir, pageName)

	// Inject <link> to the shared stylesheet
	content = injectStyleLink(content, "/style.css?v="+site.CSSVersion)

	// Inject site title heading as a clickable link to the homepage
	content = injectSiteTitle(content, siteConfig.Title)
	// Downgrade the article's <h1 class="title"> to <h2>
	content = downgradeTitleHeading(content)

	// Create page directory
	if err := os.MkdirAll(pageDir, 0755); err != nil {
		return "", fmt.Errorf("creating page directory: %w", err)
	}

	// Remove spaces between non-ASCII (e.g., CJK) characters — common artifact from
	// markdown/org paragraph wrapping or manual editing (e.g., "相 信" -> "相信").
	content = mdtowx.RemoveCJCSpace(content)

	// Syntax-highlight code blocks in the HTML
	if highlighted, count, hlErr := highlight.HighlightPage(content); hlErr == nil && count > 0 {
		fmt.Fprintf(cmd.OutOrStdout(), "%s %d code block(s) highlighted\n",
			color.CyanString("\xe2\x86\x92"), count)
		content = highlighted
	}

	// Write index.html
	indexPath := filepath.Join(pageDir, "index.html")
	if err := os.WriteFile(indexPath, content, 0644); err != nil {
		return "", fmt.Errorf("writing index.html: %w", err)
	}

	// Extract and copy image assets
	sourceDir := filepath.Dir(filePath)
	copied, err := copyPageImages(content, sourceDir, pageDir)
	if err != nil {
		fmt.Fprintf(cmd.ErrOrStderr(), "%s %v\n",
			color.YellowString(i18n.G("Warning:")), err)
	}
	if copied > 0 {
		fmt.Fprintf(cmd.OutOrStdout(), "%s %d %s\n",
			color.CyanString("\xe2\x86\x92"), copied, i18n.G("image(s) copied"))
	}

	return pageName, nil
}

// applyPageDate resolves the page's stable date from every available clue and
// freezes it into the content as a machine-readable comment. The clue order and
// the stripping order are both load-bearing: the <p class="date"> paragraph is
// one of the clues, so it must be parsed before stripDateParagraphs removes it.
// Embedding replaces any existing comment, so repeated add/update cycles never
// stack duplicates; a zero date (every clue, including the source mtime, was
// unavailable) is skipped rather than stamped as 0001-01-01.
func applyPageDate(content []byte, wasOrg bool, originalPath, siteDir, pageName string) []byte {
	pageDate := resolvePageDate(
		orgDateOf(wasOrg, originalPath),
		content,
		readDateComment(filepath.Join(siteDir, pageName, "index.html")),
		sourceModTime(originalPath),
	)

	// Strip <p class="date">...</p> inserted by Emacs org export
	content = stripDateParagraphs(content)
	if !pageDate.IsZero() {
		content = embedDateComment(content, pageDate)
	}
	return content
}

// copyPageImages finds all <img src="..."> references in the HTML and copies
// local image files to the page directory, preserving subdirectory structure.
func copyPageImages(content []byte, sourceDir, pageDir string) (int, error) {
	htmlStr := string(content)
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

	// Ensure optimized style.css exists (creates if missing, upgrades if old)
	if _, err := site.UpgradeCSS(siteDir, false); err != nil {
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
		var lastYear int
		for _, p := range pages {
			dateStr := ""
			year := 0
			if !p.Date.IsZero() {
				dateStr = p.Date.Format("January 02, 2006")
				year = p.Date.Year()
			}

			// Print year heading when it changes
			if year != 0 && year != lastYear {
				sb.WriteString(fmt.Sprintf("\n    <h2 class=\"year-heading\">%d</h2>\n", year))
				lastYear = year
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

// isValidPageName checks that a page name is safe to join onto the site
// directory. Besides the character whitelist it rejects two path-shaped
// hazards that the whitelist alone would let through:
//
//   - names containing a path separator, which could escape the site root;
//   - names made only of dots (".", "..", "..."), which filepath.Join would
//     resolve to the site root itself (overwriting the site index.html) or to
//     its parent directory.
func isValidPageName(name string) bool {
	// Reject dot-only names ("." / ".." / "..."): trimming every dot leaves
	// nothing behind. filepath.Join would resolve them to the site root (or
	// its parent) instead of a page directory.
	if name == "" || strings.Trim(name, ".") == "" {
		return false
	}
	for _, r := range name {
		// The whitelist excludes path separators ('/', '\\'), so a name can
		// never escape the site directory.
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
func stripDateParagraphs(content []byte) []byte {
	return dateParagraphRe.ReplaceAll(content, nil)
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
//	#+DATE: <2007-06-18 Mon 12:30>
//	#+DATE: <2007-06-18 Mon>
//	#+DATE: <2007-06-18 12:30>
//	#+DATE: <2007-06-18>
//	#+DATE: <2007-06-18 二>              (Chinese locale)
//	#+DATE: <Monday, 18 June 2007>
//	#+DATE: <Mon, 18 Jun 2007>
//	#+DATE: <18 June 2007>
//	#+DATE: 2007-06-18
//	#+DATE: Monday, 18 June 2007
//	#+DATE: June 18, 2007
//	#+DATE: 2007-06-18 15:04:05
//	#+DATE: <12/19/21 Sunday 12AM>       (US-style MM/DD/YY)
//	#+DATE: <12/19/21 Sunday 12:30AM>
//	#+DATE: <12/19/21 12AM>
//	#+DATE: <12/19/21>
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
		// Strip trailing CJK weekday character and preceding space.
		// Org mode in Chinese locale: <2019-12-03 二> → <2019-12-03>
		if runes := []rune(dateStr); len(runes) >= 4 {
			if unicode.Is(unicode.Han, runes[len(runes)-2]) && runes[len(runes)-3] == ' ' {
				dateStr = string(runes[:len(runes)-3]) + string(runes[len(runes)-1])
			}
		}
		// Try common Org date formats. Time formats go first so they
		// match before their shorter counterparts (e.g. <... Mon 12:30>
		// before <... Mon>, since the latter would fail on trailing text).
		formats := []string{
			// Active timestamps with time (with and without weekday)
			"<2006-01-02 Mon 15:04>",
			"<2006-01-02 15:04>",
			"<2006-01-02 Mon 15:04:05>",
			"<2006-01-02 15:04:05>",
			// Active timestamps without time
			"<2006-01-02 Mon>",
			"<2006-01-02>",
			// Diary-style active timestamps
			"<Monday, 2 January 2006>",
			"<Mon, 2 Jan 2006>",
			"<2 January 2006>",
			// US-style (MM/DD/YY) with 12-hour time
			"<01/02/06 Monday 3:04PM>",
			"<01/02/06 Monday 3PM>",
			"<01/02/06 3:04PM>",
			"<01/02/06 3PM>",
			// US-style without time
			"<01/02/06 Monday>",
			"<01/02/06>",
			// Plain (non-bracketed) date values
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

// resolvePageDate picks the stable date for a page. The page add/update
// pipeline freezes it into the page as a machine-readable comment, which the
// site index (see extractPageDate) then uses for date-grouped rendering.
// Without a stable value the index falls back to the output file's mtime and
// the date drifts on every re-render.
//
// Clues are tried in order; the first one yielding a date wins:
//
//  1. orgDate       — the Org source #+DATE: property (most authoritative);
//  2. inputContent  — a slingshot-date comment already in the input; its value
//     is preserved instead of re-derived (hand-edited or re-added pages);
//  3. outputComment — the slingshot-date comment extracted from the page
//     currently on disk, so updating a source that carries no date keeps the
//     published date instead of drifting to the source mtime;
//  4. inputContent  — a <p class="date">…</p> paragraph from Emacs org export;
//  5. srcModTime    — the source file's mtime, frozen into the page on first add.
//
// The function is pure: every file read happens in the caller, which keeps the
// precedence chain testable without touching the filesystem.
func resolvePageDate(orgDate time.Time, inputContent, outputComment []byte, srcModTime time.Time) time.Time {
	if !orgDate.IsZero() {
		return orgDate
	}
	if t := parseDateComment(inputContent); !t.IsZero() {
		return t
	}
	if t := parseDateComment(outputComment); !t.IsZero() {
		return t
	}
	if t := parseDateParagraph(inputContent); !t.IsZero() {
		return t
	}
	return srcModTime
}

// parseDateComment extracts the date from a slingshot-date comment.
// Returns the zero time when the content holds no such comment.
func parseDateComment(content []byte) time.Time {
	m := slingshotDateRe.FindSubmatch(content)
	if len(m) < 2 {
		return time.Time{}
	}
	t, err := time.Parse("2006-01-02", string(m[1]))
	if err != nil {
		return time.Time{}
	}
	return t
}

// parseDateParagraph extracts the date from the first <p class="date">…</p>
// element. Returns the zero time when there is no such element or its text is
// not a recognizable date.
func parseDateParagraph(content []byte) time.Time {
	m := dateParagraphBodyRe.FindSubmatch(content)
	if len(m) < 2 {
		return time.Time{}
	}
	// Reduce the body to plain text: drop tags, decode entities, collapse
	// whitespace (org export wraps the value across lines).
	text := string(tagRe.ReplaceAll(m[1], nil))
	text = html.UnescapeString(text)
	text = strings.Join(strings.Fields(text), " ")
	if text == "" {
		return time.Time{}
	}
	for _, f := range pDateFormats {
		if t, err := time.Parse(f, text); err == nil {
			return t
		}
	}
	return time.Time{}
}

// orgDateOf returns the #+DATE: timestamp of an Org source, or the zero time
// when the input did not come from Org.
func orgDateOf(wasOrg bool, originalPath string) time.Time {
	if !wasOrg {
		return time.Time{}
	}
	return extractOrgDate(originalPath)
}

// readDateComment returns the slingshot-date comment of the HTML file at path.
// A missing or unreadable file is the normal first-add case, not an error, so
// it yields nil.
func readDateComment(path string) []byte {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	return slingshotDateRe.Find(data)
}

// sourceModTime returns the modification time of the page's source file, or
// the zero time when it cannot be stat'ed.
func sourceModTime(path string) time.Time {
	fi, err := os.Stat(path)
	if err != nil {
		return time.Time{}
	}
	return fi.ModTime()
}

// embedDateComment prepends a machine-readable date comment to HTML content
// so the site index generator can use it for date-grouped rendering.
//
// It is idempotent: any pre-existing slingshot-date comment is removed first,
// so re-running the pipeline replaces the old value instead of stacking a
// second comment on top (and a run over an already-correct page is a no-op).
func embedDateComment(content []byte, date time.Time) []byte {
	content = stripDateComments(content)
	comment := fmt.Sprintf("<!-- slingshot-date: %s -->\n", date.Format("2006-01-02"))
	return append([]byte(comment), content...)
}

// stripDateComments removes every slingshot-date comment from HTML content,
// together with one trailing newline so no blank line is left behind.
func stripDateComments(content []byte) []byte {
	return dateCommentLineRe.ReplaceAll(content, nil)
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
		// Disable sub/superscript conversion -- _ in filenames like sample_2.d2
		// must not become <sub> during HTML export.
		"--eval", "(setq org-export-with-sub-superscripts nil)",
		// Disable htmlize syntax highlighting — tree-sitter (HighlightPage)
		// handles all code blocks. htmlize outputs <span style="color:...">
		// that conflicts with subsequent tree-sitter highlighting.
		"--eval", "(setq org-html-htmlize-output-type nil)",
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
func injectStyleLink(content []byte, href string) []byte {
	link := fmt.Sprintf(`<link rel="stylesheet" href="%s" />`, href)
	headEnd := bytes.Index(content, []byte("</head>"))
	if headEnd < 0 {
		// No </head> — prepend before </html> as fallback
		htmlEnd := bytes.Index(content, []byte("</html>"))
		if htmlEnd < 0 {
			return append(content, []byte(link+"\n")...)
		}
		return []byte(string(content[:htmlEnd]) + link + "\n" + string(content[htmlEnd:]))
	}
	// Check if the link is already present
	if bytes.Contains(content[:headEnd], []byte(href)) {
		return content
	}
	return []byte(string(content[:headEnd]) + "  " + link + "\n" + string(content[headEnd:]))
}

// injectSiteTitle inserts <h1><a href="/">title</a> </h1> right after <body>
// so every page has a clickable site-title link back to the homepage.
func injectSiteTitle(content []byte, title string) []byte {
	link := fmt.Sprintf("<h1><a href=\"/\">%s</a> </h1>\n", htmlEscape(title))

	bodyTag := []byte("<body")
	idx := bytes.Index(content, bodyTag)
	if idx < 0 {
		return content
	}

	// Find the closing > of the <body> tag
	rest := content[idx:]
	gtIdx := bytes.IndexByte(rest, '>')
	if gtIdx < 0 {
		return content
	}

	insertPos := idx + gtIdx + 1
	result := make([]byte, 0, len(content)+len(link)+1)
	result = append(result, content[:insertPos]...)
	result = append(result, '\n')
	result = append(result, link...)
	result = append(result, content[insertPos:]...)
	return result
}

// downgradeTitleHeading changes <h1 class="title"> to <h2 class="title">
// and the corresponding </h1> to </h2>. This is called after injectSiteTitle
// so that the page has exactly one <h1> (the site title link).
func downgradeTitleHeading(content []byte) []byte {
	startOld := []byte(`<h1 class="title">`)
	startNew := []byte(`<h2 class="title">`)
	closeOld := []byte(`</h1>`)
	closeNew := []byte(`</h2>`)

	// Find opening tag
	idx := bytes.Index(content, startOld)
	if idx < 0 {
		return content
	}

	// Build result: replace opening tag
	var buf bytes.Buffer
	buf.Write(content[:idx])
	buf.Write(startNew)
	rest := content[idx+len(startOld):]

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

package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/fatih/color"
	"github.com/spf13/cobra"

	cli "github.com/nanjj/slingshot/internal/cmd"
	"github.com/nanjj/slingshot/internal/config"
	"github.com/nanjj/slingshot/internal/i18n"
	u "github.com/nanjj/slingshot/internal/usage"
)

// --- cmdPageSub ---

// cmdPageSub 是 page 的通用子命令模板（用于 list/remove 等无额外标志的子命令）。
type cmdPageSub struct {
	global  *cmdGlobal
	name    string
	usage   u.Usage
	short   string
	long    string
	minArgs int
	action  func(cfg *config.Config, parsed []*u.Parsed) error
}

func (s *cmdPageSub) command() *cobra.Command {
	cmd := &cobra.Command{}
	cmd.Use = s.name
	cmd.Short = s.short
	cmd.Long = cli.FormatSection(
		color.CyanString("Description:"),
		s.long,
	)
	cmd.RunE = s.run
	cmd.Args = cobra.ArbitraryArgs
	return cmd
}

func (s *cmdPageSub) run(cmd *cobra.Command, args []string) error {
	cfg, _, err := config.Load()
	if err != nil {
		return fmt.Errorf("%s: %w", i18n.G("loading config"), err)
	}

	if len(args) < s.minArgs && !s.global.flagExplain {
		return errors.New(i18n.G("not enough arguments"))
	}

	parsed, err := s.global.Parse(s.usage, cmd, args)
	if err != nil {
		return err
	}

	return s.action(cfg, parsed)
}

// Actions

func (c *cmdPage) doList(cfg *config.Config, parsed []*u.Parsed) error {
	if len(parsed) < 1 || parsed[0].Skipped {
		return errors.New(i18n.G("expected a site name argument"))
	}
	siteName := parsed[0].String

	site, ok := config.GetSite(cfg, siteName)
	if !ok {
		return fmt.Errorf(i18n.G("site %q not found"), siteName)
	}

	if site.Dir == "" {
		return fmt.Errorf(i18n.G("site %q has no directory configured"), siteName)
	}

	pages, err := listPages(site.Dir)
	if err != nil {
		return fmt.Errorf("listing pages: %w", err)
	}
	if len(pages) == 0 {
		fmt.Fprintf(color.Output, "%s\n",
			color.CyanString(i18n.G("No pages in site %q."), siteName))
		return nil
	}

	fmt.Fprintf(color.Output, "%s\n\n",
		color.CyanString(i18n.G("Pages in %s (%d total):"), siteName, len(pages)))

	for _, p := range pages {
		fmt.Fprintf(color.Output, "  %s\n", color.GreenString(p.Name))
		if p.Title != "" && p.Title != p.Name {
			fmt.Fprintf(color.Output, "    %s %s\n", color.CyanString(i18n.G("Title:")), p.Title)
		}
	}
	return nil
}

// pageInfo holds information about a page.
type pageInfo struct {
	Name  string // directory name
	Title string // extracted from index.html <title>
}

// listPages reads the site directory and returns page info for each subdirectory
// containing an index.html.
func listPages(siteDir string) ([]pageInfo, error) {
	entries, err := os.ReadDir(siteDir)
	if err != nil {
		return nil, err
	}

	var pages []pageInfo
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		// Skip hidden directories
		if strings.HasPrefix(entry.Name(), ".") {
			continue
		}
		indexPath := filepath.Join(siteDir, entry.Name(), "index.html")
		if _, err := os.Stat(indexPath); os.IsNotExist(err) {
			continue
		}

		title := extractPageTitle(indexPath, entry.Name())
		pages = append(pages, pageInfo{
			Name:  entry.Name(),
			Title: title,
		})
	}

	sort.Slice(pages, func(i, j int) bool {
		return pages[i].Name < pages[j].Name
	})

	return pages, nil
}

// extractPageTitle reads an HTML file and extracts the <title> tag content.
// Uses the existing extractTitle helper from draft_extract.go.
func extractPageTitle(path, fallback string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return fallback
	}
	// extractTitle expects a file path for fallback; pass a synthetic one
	return extractTitle(string(data), fallback+".html")
}

package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/fatih/color"

	"github.com/nanjj/slingshot/internal/config"
	"github.com/nanjj/slingshot/internal/i18n"
	u "github.com/nanjj/slingshot/internal/usage"
)

// Actions

func (c *cmdPage) doRemove(cfg *config.Config, parsed []*u.Parsed) error {
	if len(parsed) < 2 || parsed[1].Skipped {
		return errors.New(i18n.G("expected site and page arguments"))
	}
	siteName := parsed[0].String
	pageName := parsed[1].String

	site, ok := config.GetSite(cfg, siteName)
	if !ok {
		return fmt.Errorf(i18n.G("site %q not found"), siteName)
	}

	if site.Dir == "" {
		return fmt.Errorf(i18n.G("site %q has no directory configured"), siteName)
	}

	pageDir := filepath.Join(site.Dir, pageName)

	// Check page exists
	if _, err := os.Stat(pageDir); os.IsNotExist(err) {
		return fmt.Errorf(i18n.G("page %q not found in site %q"), pageName, siteName)
	}

	// Remove page directory
	if err := os.RemoveAll(pageDir); err != nil {
		return fmt.Errorf("removing page directory: %w", err)
	}

	// Regenerate site index
	if err := regenerateSiteIndex(site.Dir, siteName); err != nil {
		return fmt.Errorf("regenerating site index: %w", err)
	}

	fmt.Fprintf(color.Output, "%s %s/%s\n",
		color.GreenString(i18n.G("Removed:")), siteName, color.GreenString(pageName))
	return nil
}

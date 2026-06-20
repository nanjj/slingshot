package main

import (
	"fmt"
	"os"
	"sort"

	"github.com/fatih/color"
	"github.com/spf13/cobra"

	"github.com/nanjj/slingshot/internal/i18n"
)

// cmdI18nAdd implements "slingshot i18n add".
type cmdI18nAdd struct {
	global  *cmdGlobal
	i18nCmd *cmdI18n
}

func (c *cmdI18nAdd) command() *cobra.Command {
	cmd := &cobra.Command{}
	cmd.Use = "add <locale>"
	cmd.Short = i18n.G("Initialize a new locale from en_US template")
	cmd.Long = i18n.G(`Initialize a new locale from the en_US template.

Creates a new locale directory and .po file with all msgids from en_US,
all with empty msgstr (untranslated).

Example:
  slingshot i18n add ja        # Create ja locale from en_US
  slingshot i18n add zh_TW -d path/to/locales`)
	cmd.RunE = c.run
	cmd.Args = cobra.ExactArgs(1)
	return cmd
}

func (c *cmdI18nAdd) run(cmd *cobra.Command, args []string) error {
	locale := args[0]
	dir := c.i18nCmd.resolveDir()

	// Load en_US entries
	enUSEntries, err := loadPOFull(dir, "en_US")
	if err != nil {
		return fmt.Errorf("loading en_US: %w", err)
	}

	// Check if locale already exists
	existing, err := loadPOFull(dir, locale)
	if err == nil && len(existing) > 0 {
		return fmt.Errorf("locale %q already exists with %d entries", locale, len(existing))
	}

	// Build new entries: copy header and all msgids from en_US
	var entries []poEntry

	// Find header entry
	var hasHeader bool
	for _, e := range enUSEntries {
		if e.Msgid == "" {
			// Copy header, updating Language metadata
			header := e
			header.Comments = nil // Don't carry en_US comments
			// Replace Language in metadata
			header.Msgstr = setPOLanguage(header.Msgstr, locale)
			entries = append(entries, header)
			hasHeader = true
			break
		}
	}

	if !hasHeader {
		// Create a minimal header if en_US doesn't have one
		entries = append(entries, poEntry{
			Msgid:  "",
			Msgstr: fmt.Sprintf("Project-Id-Version: slingshot 0.1.0\nLanguage: %s\nMIME-Version: 1.0\nContent-Type: text/plain; charset=UTF-8\nContent-Transfer-Encoding: 8bit\n", locale),
		})
	}

	// Collect all msgids (skip header)
	var msgids []string
	for _, e := range enUSEntries {
		if e.Msgid != "" {
			msgids = append(msgids, e.Msgid)
		}
	}
	sort.Strings(msgids)

	// Add entries with empty msgstr (untranslated)
	for _, mid := range msgids {
		entries = append(entries, poEntry{
			Msgid:  mid,
			Msgstr: "",
		})
	}

	// Write
	if err := savePO(dir, locale, entries); err != nil {
		return fmt.Errorf("writing %s: %w", locale, err)
	}

	fmt.Fprintf(color.Output, "%s %s %s (%d %s)\n",
		color.GreenString("✓"),
		i18n.G("Created"),
		locale,
		syncedEntryCount(entries),
		i18n.G("entries"))

	fmt.Fprintln(os.Stderr, i18n.G("  All entries are untranslated. Use a .po editor to add translations."))

	return nil
}

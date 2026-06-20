package main

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/fatih/color"
	"github.com/spf13/cobra"

	"github.com/nanjj/slingshot/internal/i18n"
)

// cmdI18nShow implements "slingshot i18n show".
// Shows full details of an untranslated entry by its numbered ID,
// as displayed by "slingshot i18n check".
type cmdI18nShow struct {
	global  *cmdGlobal
	i18nCmd *cmdI18n
}

func (c *cmdI18nShow) command() *cobra.Command {
	cmd := &cobra.Command{}
	cmd.Use = "show <locale> <id>"
	cmd.Short = i18n.G("Show full details of an untranslated entry by ID")
	cmd.Long = i18n.G(`Show the full msgid and metadata for an untranslated entry.

The ID corresponds to the number shown by "slingshot i18n check <locale>".
ID numbering is stable — entries are sorted alphabetically.

Example:
  slingshot i18n show zh_CN 3`)

	cmd.RunE = c.run
	cmd.Args = cobra.ExactArgs(2)
	cmd.ValidArgs = []string{"locale", "id"}
	return cmd
}

func (c *cmdI18nShow) run(cmd *cobra.Command, args []string) error {
	locale := args[0]
	id, err := strconv.Atoi(args[1])
	if err != nil {
		return fmt.Errorf("invalid id %q: must be a number", args[1])
	}
	if id < 1 {
		return fmt.Errorf("invalid id %d: must be positive", id)
	}

	dir := c.i18nCmd.resolveDir()

	// Load en_US sentinel
	enUS, err := loadEnUS(dir)
	if err != nil {
		return fmt.Errorf("loading en_US: %w", err)
	}

	// Load the locale's flat table
	table, err := loadPO(dir, locale)
	if err != nil {
		return fmt.Errorf("loading %s: %w", locale, err)
	}

	// Analyse to get sorted untranslated list (same order as check)
	analysis := analyseLocale(locale, table, enUS)
	sort.Strings(analysis.UntranslatedList)

	if id > len(analysis.UntranslatedList) {
		return fmt.Errorf("id %d out of range: %s has only %d untranslated entries",
			id, locale, len(analysis.UntranslatedList))
	}

	msgid := analysis.UntranslatedList[id-1]

	// Load full entries to get comments
	entries, err := loadPOFull(dir, locale)
	if err != nil {
		return fmt.Errorf("loading full %s: %w", locale, err)
	}
	emap := poEntryMap(entries)

	entry, ok := emap[msgid]
	if !ok {
		return fmt.Errorf("internal error: msgid %q not found in full entries", msgid)
	}

	// Build display
	var b strings.Builder

	b.WriteString("\n")
	b.WriteString(color.CyanString(i18n.G("Entry #%d\n"), id))
	b.WriteString("─────────────────────────────────────────────────────\n")
	b.WriteString(color.YellowString(i18n.G("Full msgid")))
	b.WriteString(fmt.Sprintf(" (%d %s):\n", len(msgid), i18n.G("chars")))
	b.WriteString(fmt.Sprintf("  %q\n", msgid))

	if len(entry.Comments) > 0 {
		b.WriteString("\n")
		b.WriteString(color.YellowString(i18n.G("Comments")) + ":\n")
		for _, comment := range entry.Comments {
			b.WriteString(fmt.Sprintf("  %s\n", comment))
		}
	}

	b.WriteString(fmt.Sprintf("\n%s: ", color.YellowString(i18n.G("Current translation"))))
	if entry.Msgstr == "" {
		b.WriteString(color.RedString(i18n.G("(empty)")))
	} else {
		b.WriteString(entry.Msgstr)
	}
	b.WriteString("\n")
	b.WriteString("─────────────────────────────────────────────────────\n\n")

	fmt.Fprint(color.Output, b.String())
	return nil
}

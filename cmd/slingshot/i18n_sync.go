package main

import (
	"fmt"
	"os"
	"sort"

	"github.com/fatih/color"
	"github.com/spf13/cobra"

	"github.com/nanjj/slingshot/internal/i18n"
)

// cmdI18nSync implements "slingshot i18n sync".
type cmdI18nSync struct {
	global  *cmdGlobal
	i18nCmd *cmdI18n
	delete  bool // --delete flag: remove orphaned entries
}

func (c *cmdI18nSync) command() *cobra.Command {
	cmd := &cobra.Command{}
	cmd.Use = "sync"
	cmd.Short = i18n.G("Synchronize .po files with source code")
	cmd.Long = i18n.G(`Synchronize translation (.po) files with i18n.G() calls in Go source code.

Scans all .go files (excluding vendor/ and hidden directories) for i18n.G()
calls, adds new msgids to en_US, and propagates new entries to all other
locales with empty msgstr (untranslated).

Existing translations are never overwritten.

With --delete, removes orphaned entries (msgids that exist in .po files
but no longer appear in source code).`)
	cmd.Flags().BoolVar(&c.delete, "delete", false,
		"Delete orphaned entries (in .po but not in source code)")
	cmd.RunE = c.run
	return cmd
}

func (c *cmdI18nSync) run(cmd *cobra.Command, args []string) error {
	dir := c.i18nCmd.resolveDir()

	// 1. Extract msgids from source code
	color.New(color.Faint).Fprintln(os.Stderr, i18n.G("Scanning source code for i18n.G() calls..."))
	sourceMsgids, err := extractMsgids(c.i18nCmd.resolveRoot())

	if err != nil {
		return fmt.Errorf("extracting msgids from source: %w", err)
	}
	if len(sourceMsgids) == 0 {
		return fmt.Errorf("no i18n.G() calls found in source code")
	}

	// 2. Load en_US.po as entries
	enUSEntries, err := loadPOFull(dir, "en_US")
	if err != nil {
		return fmt.Errorf("loading en_US: %w", err)
	}

	// 3. Deduplicate entries before sync to prevent duplicates
	// arising from different .po escaping producing the same msgid.
	enUSEntries = dedupEntries(enUSEntries)
	beforeCount := syncedEntryCount(enUSEntries)

	// 4. Sync en_US: add new entries, optionally delete orphans
	enUSChanged := syncEnUS(&enUSEntries, sourceMsgids, c.delete)
	afterCount := syncedEntryCount(enUSEntries)

	// 5. Write en_US if changed
	if enUSChanged {
		if err := savePO(dir, "en_US", enUSEntries); err != nil {
			return fmt.Errorf("writing en_US: %w", err)
		}
		added := afterCount - beforeCount
		removed := beforeCount - afterCount
		msg := i18n.G("Updated en_US") + fmt.Sprintf(" (%d %s)", afterCount, i18n.G("entries"))
		if added > 0 {
			msg += fmt.Sprintf(", +%d %s", added, i18n.G("new"))
		}
		if removed > 0 {
			msg += fmt.Sprintf(", -%d %s", removed, i18n.G("removed"))
		}
		fmt.Fprintf(color.Output, "%s %s\n", color.GreenString("\u2713"), msg)
	} else {
		fmt.Fprintf(color.Output, "%s en_US %s (%d %s)\n",
			color.GreenString("\u2713"),
			i18n.G("is up to date"),
			afterCount,
			i18n.G("entries"))
	}

	// 6. Build en_US msgid set (excluding header)
	enUSMap := poEntryMap(enUSEntries)

	// 7. Sync other locales
	locales, err := listLocales(dir)
	if err != nil {
		return fmt.Errorf("listing locales: %w", err)
	}

	for _, loc := range locales {
		if loc == "en_US" {
			continue
		}

		entries, err := loadPOFull(dir, loc)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Warning: skipping %s: %v\n", loc, err)
			continue
		}

		entries = dedupEntries(entries)
		localeChanged := syncLocale(&entries, enUSMap, c.delete)

		if localeChanged {
			if err := savePO(dir, loc, entries); err != nil {
				fmt.Fprintf(os.Stderr, "Warning: writing %s: %v\n", loc, err)
				continue
			}
			fmt.Fprintf(color.Output, "  %s %s %s (%d %s)\n",
				color.GreenString("\u2713"),
				i18n.G("Updated"),
				loc,
				syncedEntryCount(entries),
				i18n.G("entries"))
		} else {
			fmt.Fprintf(color.Output, "  %s %s %s (%d %s)\n",
				color.GreenString("\u2713"),
				loc,
				i18n.G("is up to date"),
				syncedEntryCount(entries),
				i18n.G("entries"))
		}
	}

	return nil
}

// dedupEntries removes duplicate entries, keeping only the last occurrence
// of each msgid. The header entry (msgid "") is always preserved.
func dedupEntries(entries []poEntry) []poEntry {
	seen := make(map[string]int) // msgid → last index
	// First pass: find last index for each msgid
	for i, e := range entries {
		seen[e.Msgid] = i
	}
	// Second pass: keep only the last occurrence
	result := make([]poEntry, 0, len(seen))
	// Collect indices and sort them to preserve original order
	indices := make([]int, 0, len(seen))
	for _, idx := range seen {
		indices = append(indices, idx)
	}
	sort.Ints(indices)
	for _, idx := range indices {
		result = append(result, entries[idx])
	}
	return result
}

// syncEnUS updates en_US entries to match sourceMsgids.
// sourceMsgids are already unescaped by the AST extractor.
// Returns true if any changes were made.
func syncEnUS(entries *[]poEntry, sourceMsgids map[string]bool, delete bool) bool {
	changed := false

	// Build map of current msgids
	current := poEntryMap(*entries)

	// Find new msgids: in source but not in current
	var newIDs []string
	for mid := range sourceMsgids {
		if _, ok := current[mid]; !ok {
			newIDs = append(newIDs, mid)
		}
	}

	if len(newIDs) > 0 {
		sort.Strings(newIDs)
		for _, mid := range newIDs {
			// For en_US, msgstr = msgid (English text)
			*entries = append(*entries, poEntry{
				Msgid:  mid,
				Msgstr: mid,
			})
		}
		changed = true
	}

	// Remove orphaned entries if --delete
	if delete {
		var filtered []poEntry
		for _, e := range *entries {
			// Keep header entry (msgid "") and entries still in source
			if e.Msgid == "" || sourceMsgids[e.Msgid] {
				filtered = append(filtered, e)
			}
		}
		if len(filtered) != len(*entries) {
			changed = true
		}
		*entries = filtered
	}

	return changed
}

// syncLocale updates a non-en_US locale's entries to match en_US.
// Never overwrites existing translations (msgstr is preserved).
// Returns true if any changes were made.
func syncLocale(entries *[]poEntry, enUS map[string]*poEntry, delete bool) bool {
	changed := false

	// Build map of current msgids
	current := poEntryMap(*entries)

	// Find new msgids: in en_US but not in current
	var newIDs []string
	for mid := range enUS {
		if _, ok := current[mid]; !ok {
			newIDs = append(newIDs, mid)
		}
	}

	if len(newIDs) > 0 {
		sort.Strings(newIDs)
		for _, mid := range newIDs {
			// New entries start untranslated (empty msgstr)
			if mid == "" {
				continue
			}
			*entries = append(*entries, poEntry{
				Msgid:  mid,
				Msgstr: "", // untranslated — never overwrite existing
			})
		}
		changed = true
	}

	// Remove orphaned entries if --delete
	if delete {
		var filtered []poEntry
		for _, e := range *entries {
			// Keep header entry and entries still in en_US
			if e.Msgid == "" {
				filtered = append(filtered, e)
			} else if _, ok := enUS[e.Msgid]; ok {
				filtered = append(filtered, e)
			}
		}
		if len(filtered) != len(*entries) {
			changed = true
		}
		*entries = filtered
	}

	return changed
}

package main

import (
	"fmt"
	"os"
	"sort"

	"github.com/fatih/color"
	"github.com/spf13/cobra"

	"github.com/nanjj/slingshot/internal/i18n"
)

// cmdI18nVerify implements "slingshot i18n verify".
//
// i18n check compares locales against each other. i18n verify compares
// every locale against the source code: each i18n.G() argument in the
// tree must have an entry in each .po file.
//
// i18n.G itself falls back to the msgid when an entry is missing, so a
// gap here shows up as an untranslated string, never as a crash. That
// makes verify a quality gate rather than a safety net: it keeps the
// shipped binary from silently regressing to English in one locale.
type cmdI18nVerify struct {
	global        *cmdGlobal
	i18nCmd       *cmdI18n
	strictOrphans bool // --strict-orphans: fail on stale entries too
}

func (c *cmdI18nVerify) command() *cobra.Command {
	cmd := &cobra.Command{}
	cmd.Use = "verify [locale...]"
	cmd.Short = i18n.G("Verify every i18n.G() call has an entry in every locale")
	cmd.Long = i18n.G(`Verify that every msgid used in source code has an entry in
each locale's .po file.

"i18n check" compares locales against each other. "i18n verify" compares
each locale against the source tree, which is what i18n.G() actually
consults at runtime. i18n.G() falls back to the msgid when an entry is
missing, so a gap here degrades to an untranslated string. This check
belongs in the build gate to keep that from shipping unnoticed.

Orphaned entries (in a .po but no longer used in source) are reported
but do not fail the check, because a dynamically built msgid can look
orphaned to the AST scanner. Use --strict-orphans to fail on them too.

Without arguments, verifies every locale. With locale arguments, checks
only those locales. Exits non-zero if any locale is missing a msgid.`)
	cmd.Flags().BoolVar(&c.strictOrphans, "strict-orphans", false,
		i18n.G("Fail on orphaned entries too: msgids no longer used in source"))
	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		return c.run(cmd, args)
	}
	cmd.Args = cobra.ArbitraryArgs
	return cmd
}

func (c *cmdI18nVerify) run(cmd *cobra.Command, args []string) error {
	dir := c.i18nCmd.resolveDir()
	root := c.i18nCmd.resolveRoot()

	sourceInfos, err := extractMsgids(root)
	if err != nil {
		return fmt.Errorf("extracting msgids from source: %w", err)
	}
	if len(sourceInfos) == 0 {
		fmt.Fprintln(os.Stderr, i18n.G("No i18n.G() calls found in source."))
		return nil
	}

	sourceMsgids := make(map[string]bool, len(sourceInfos))
	for msgid := range sourceInfos {
		sourceMsgids[msgid] = true
	}

	allLocales, err := loadAllLocales(dir)
	if err != nil {
		return fmt.Errorf("loading locales: %w", err)
	}

	targets := args
	if len(targets) == 0 {
		for loc := range allLocales {
			targets = append(targets, loc)
		}
		sort.Strings(targets)
	}

	var totalMissing, totalOrphaned int
	for _, loc := range targets {
		table, ok := allLocales[loc]
		if !ok {
			fmt.Fprintf(os.Stderr, "Warning: locale %q not found\n", loc)
			continue
		}
		missing, orphaned := compareWithSource(table, sourceMsgids)
		totalMissing += len(missing)
		totalOrphaned += len(orphaned)

		if len(missing) == 0 && len(orphaned) == 0 {
			fmt.Fprintf(color.Output, "%s %s\n", color.GreenString("✓"), loc)
			continue
		}
		if len(missing) > 0 {
			fmt.Fprintf(color.Output, "%s %s: %s %d\n",
				color.RedString("!"), loc,
				color.RedString(i18n.G("missing")), len(missing))
			for _, m := range missing {
				fmt.Fprintf(color.Output, "    - %q\n", truncateMsgid(m, 72))
			}
		}
		if len(orphaned) > 0 {
			fmt.Fprintf(color.Output, "%s %s: %s %d\n",
				color.YellowString("?"), loc,
				color.YellowString(i18n.G("orphaned")), len(orphaned))
			for _, m := range orphaned {
				fmt.Fprintf(color.Output, "    - %q\n", truncateMsgid(m, 72))
			}
		}
	}

	if totalMissing > 0 {
		return fmt.Errorf(i18n.G("i18n verify: %d missing entries; run: slingshot i18n sync"), totalMissing)
	}
	if totalOrphaned > 0 && c.strictOrphans {
		return fmt.Errorf(i18n.G("i18n verify: %d orphaned entries; run: slingshot i18n sync --delete"), totalOrphaned)
	}
	return nil
}

// compareWithSource splits a locale table against the source msgid set.
// missing: used in source, absent from the locale — ships untranslated.
// orphaned: present in the locale, no longer used in source — stale text.
// The header entry (empty msgid) is skipped.
func compareWithSource(table map[string]string, sourceMsgids map[string]bool) (missing, orphaned []string) {
	for msgid := range sourceMsgids {
		if _, ok := table[msgid]; !ok {
			missing = append(missing, msgid)
		}
	}
	for msgid := range table {
		if msgid != "" && !sourceMsgids[msgid] {
			orphaned = append(orphaned, msgid)
		}
	}
	sort.Strings(missing)
	sort.Strings(orphaned)
	return missing, orphaned
}

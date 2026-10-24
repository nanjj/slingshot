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
// tree must have an entry in each .po file. A missing entry there is
// what i18n.G would panic on at runtime, so this command is the place
// to catch it — before the binary is built, not after it is shipped.
type cmdI18nVerify struct {
	global  *cmdGlobal
	i18nCmd *cmdI18n
}

func (c *cmdI18nVerify) command() *cobra.Command {
	cmd := &cobra.Command{}
	cmd.Use = "verify [locale...]"
	cmd.Short = i18n.G("Verify every i18n.G() call has an entry in every locale")
	cmd.Long = i18n.G(`Verify that every msgid used in source code has an entry in
each locale's .po file.

"i18n check" compares locales against each other. "i18n verify" compares
each locale against the source tree, which is what i18n.G() actually
calls at runtime. A missing entry here is a panic waiting for the first
user who runs the command, so this check belongs in the build gate.

Without arguments, verifies every locale. With locale arguments, checks
only those locales. Exits non-zero if any locale is missing a msgid.`)
	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		return c.run(cmd, args)
	}
	cmd.Args = cobra.ArbitraryArgs
	return cmd
}

func (c *cmdI18nVerify) run(cmd *cobra.Command, args []string) error {
	dir := c.i18nCmd.resolveDir()
	root := c.i18nCmd.resolveRoot()

	sourceMsgids, err := extractMsgids(root)
	if err != nil {
		return fmt.Errorf("extracting msgids from source: %w", err)
	}
	if len(sourceMsgids) == 0 {
		fmt.Fprintln(os.Stderr, i18n.G("No i18n.G() calls found in source."))
		return nil
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

	var totalMissing int
	for _, loc := range targets {
		table, ok := allLocales[loc]
		if !ok {
			fmt.Fprintf(os.Stderr, "Warning: locale %q not found\n", loc)
			continue
		}
		var missing []string
		for msgid := range sourceMsgids {
			if _, ok := table[msgid]; !ok {
				missing = append(missing, msgid)
			}
		}
		sort.Strings(missing)
		totalMissing += len(missing)

		if len(missing) == 0 {
			fmt.Fprintf(color.Output, "%s %s\n", color.GreenString("✓"), loc)
			continue
		}
		fmt.Fprintf(color.Output, "%s %s: %s %d\n",
			color.RedString("!"), loc,
			color.RedString(i18n.G("missing")), len(missing))
		for _, m := range missing {
			fmt.Fprintf(color.Output, "    - %q\n", truncateMsgid(m, 72))
		}
	}

	if totalMissing > 0 {
		return fmt.Errorf(i18n.G("i18n verify: %d missing entries; run: slingshot i18n sync"), totalMissing)
	}
	return nil
}

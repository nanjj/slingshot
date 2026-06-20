package main

import (
	"errors"
	"fmt"
	"os"
	"sort"

	"github.com/fatih/color"
	"github.com/spf13/cobra"
)

// cmdI18nCheck implements "slingshot i18n check".
// Uses plain English — not i18n.G() — to avoid circular dependency.
type cmdI18nCheck struct {
	global   *cmdGlobal
	i18nCmd  *cmdI18n
	exitCode bool // --exit-code flag
}

func (c *cmdI18nCheck) command() *cobra.Command {
	cmd := &cobra.Command{}
	cmd.Use = "check [locale...]"
	cmd.Short = "Check locale consistency for missing, untranslated, and orphaned entries"
	cmd.Long = `Check .po file consistency across locales.

Compares each locale against the en_US sentinel and reports:
  - Missing entries: in en_US but not in the locale (would panic at runtime)
  - Untranslated entries: msgstr is empty (tolerated at runtime)
  - Orphaned entries: in the locale but not in en_US (stale/outdated)

Without arguments, checks all locales except en_US.
With locale arguments, checks only the specified locales.

Exit code:
  Without --exit-code: always 0
  With --exit-code:    1 if any locale has missing or orphaned entries`
	cmd.Flags().BoolVar(&c.exitCode, "exit-code", false,
		"Exit with non-zero status if any issues are found")
	cmd.RunE = c.run
	cmd.Args = cobra.ArbitraryArgs
	return cmd
}

func (c *cmdI18nCheck) run(cmd *cobra.Command, args []string) error {
	dir := c.i18nCmd.resolveDir()

	// Load en_US sentinel
	enUS, err := loadEnUS(dir)
	if err != nil {
		return fmt.Errorf("loading en_US: %w", err)
	}

	// Load all locales
	allLocales, err := loadAllLocales(dir)
	if err != nil {
		return fmt.Errorf("loading locales: %w", err)
	}

	// Determine which locales to check
	targets := args
	if len(targets) == 0 {
		for loc := range allLocales {
			if loc != "en_US" {
				targets = append(targets, loc)
			}
		}
		sort.Strings(targets)
	}

	// Analyse each target locale
	type result struct {
		locale   string
		analysis *poAnalysis
	}
	var results []result
	for _, loc := range targets {
		table, ok := allLocales[loc]
		if !ok {
			fmt.Fprintf(os.Stderr, "Warning: locale %q not found\n", loc)
			continue
		}
		analysis := analyseLocale(loc, table, enUS)
		results = append(results, result{locale: loc, analysis: analysis})
	}

	if len(results) == 0 {
		fmt.Fprintln(os.Stderr, "No locales to check.")
		return nil
	}

	// Sort results: locales with issues first
	sort.Slice(results, func(i, j int) bool {
		ii := results[i].analysis.hasIssues()
		jj := results[j].analysis.hasIssues()
		if ii != jj {
			return ii // issues first
		}
		return results[i].locale < results[j].locale
	})

	// Display results
	hasIssues := false
	for _, r := range results {
		a := r.analysis
		if !a.hasIssues() && a.Untranslated == 0 {
			fmt.Fprintf(color.Output, "%s %s\n",
				color.GreenString("✓"), r.locale)
			continue
		}

		if a.hasIssues() {
			hasIssues = true
		}

		fmt.Fprintf(color.Output, "%s %s\n",
			color.YellowString("!"), r.locale)

		// Missing entries (would panic)
		if len(a.Missing) > 0 {
			sort.Strings(a.Missing)
			fmt.Fprintf(color.Output, "  %s %s\n",
				color.RedString("Missing entries:"),
				color.RedString("%d", len(a.Missing)))
			for _, m := range a.Missing {
				display := truncateMsgid(m, 72)
				fmt.Fprintf(color.Output, "    - %q\n", display)
			}
		}

		// Orphaned entries
		if len(a.Orphaned) > 0 {
			sort.Strings(a.Orphaned)
			fmt.Fprintf(color.Output, "  %s %s\n",
				color.MagentaString("Orphaned entries:"),
				color.MagentaString("%d", len(a.Orphaned)))
			for _, o := range a.Orphaned {
				display := truncateMsgid(o, 72)
				fmt.Fprintf(color.Output, "    - %q\n", display)
			}
		}

		// Untranslated entries
		if a.Untranslated > 0 {
			sort.Strings(a.UntranslatedList)
			fmt.Fprintf(color.Output, "  %s %s\n",
				color.CyanString("Untranslated entries:"),
				color.CyanString("%d", a.Untranslated))
			for _, u := range a.UntranslatedList {
				display := truncateMsgid(u, 72)
				fmt.Fprintf(color.Output, "    - %q\n", display)
			}
		}

		// Summary line
		fmt.Fprintf(color.Output, "    %s\n", summaryLine(a))
	}

	// Exit with code if --exit-code
	if c.exitCode && hasIssues {
		return errors.New("i18n check found issues")
	}

	return nil
}

// summaryLine returns a one-line summary of the analysis.
func summaryLine(a *poAnalysis) string {
	return fmt.Sprintf("total: %d, translated: %d, untranslated: %d, missing: %d, orphaned: %d",
		a.Total, a.Translated, a.Untranslated, len(a.Missing), len(a.Orphaned))
}

// truncateMsgid truncates long msgids for display.
func truncateMsgid(s string, maxLen int) string {
	s = singleLine(s)
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}

// singleLine replaces newlines and tabs with spaces.
func singleLine(s string) string {
	result := make([]byte, 0, len(s))
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '\n', '\r', '\t':
			result = append(result, ' ')
		default:
			result = append(result, s[i])
		}
	}
	return string(result)
}

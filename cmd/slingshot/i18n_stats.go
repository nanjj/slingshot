package main

import (
	"fmt"
	"sort"

	"github.com/fatih/color"
	"github.com/spf13/cobra"

	"github.com/nanjj/slingshot/internal/i18n"
)

// cmdI18nStats implements "slingshot i18n stats".
type cmdI18nStats struct {
	global  *cmdGlobal
	i18nCmd *cmdI18n
}

func (c *cmdI18nStats) command() *cobra.Command {
	cmd := &cobra.Command{}
	cmd.Use = "stats"
	cmd.Short = i18n.G("Show translation statistics per locale")
	cmd.Long = i18n.G(`Show translation statistics for each locale.

Displays total entries, translated count, untranslated count,
missing entries (vs en_US), and coverage percentage.`)
	cmd.RunE = c.run
	return cmd
}

func (c *cmdI18nStats) run(cmd *cobra.Command, args []string) error {
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

	// Collect locale names and sort
	names := make([]string, 0, len(allLocales))
	for loc := range allLocales {
		names = append(names, loc)
	}
	sort.Strings(names)

	// Analyse each locale
	type localeStat struct {
		name         string
		total        int
		translated   int
		untranslated int
		missing      int
		coverage     float64
	}

	var stats []localeStat
	for _, loc := range names {
		table := allLocales[loc]
		a := analyseLocale(loc, table, enUS)
		coverage := 0.0
		if len(enUS) > 0 {
			covered := len(enUS) - len(a.Missing)
			coverage = float64(covered) / float64(len(enUS)) * 100
		}
		stats = append(stats, localeStat{
			name:         a.Locale,
			total:        a.Total,
			translated:   a.Translated,
			untranslated: a.Untranslated,
			missing:      len(a.Missing),
			coverage:     coverage,
		})
	}

	// Calculate column widths
	maxName := 8
	for _, s := range stats {
		if len(s.name) > maxName {
			maxName = len(s.name)
		}
	}

	// Print header
	header := fmt.Sprintf("%-*s  %5s  %10s  %12s  %7s  %8s",
		maxName, i18n.G("Locale"),
		i18n.G("Total"),
		i18n.G("Translated"),
		i18n.G("Untranslated"),
		i18n.G("Missing"),
		i18n.G("Coverage"))

	fmt.Fprintln(color.Output, color.CyanString(header))

	// Print separator
	sep := fmt.Sprintf("%s  %5s  %10s  %12s  %7s  %8s",
		stringsRepeat("-", maxName),
		"-----", "----------", "------------", "-------", "--------")
	fmt.Fprintln(color.Output, sep)

	// Print each locale
	for _, s := range stats {
		line := fmt.Sprintf("%-*s  %5d  %10d  %12d  %7d  %7.1f%%",
			maxName, s.name,
			s.total,
			s.translated,
			s.untranslated,
			s.missing,
			s.coverage)

		// Color based on coverage
		if s.missing > 0 {
			fmt.Fprintln(color.Output, color.YellowString(line))
		} else if s.coverage >= 100 {
			fmt.Fprintln(color.Output, color.GreenString(line))
		} else {
			fmt.Fprintln(color.Output, line)
		}
	}

	fmt.Fprintln(color.Output)
	fmt.Fprintln(color.Output,
		i18n.G("Note: en_US is the sentinel — all msgids must be registered there first."))

	return nil
}

// stringsRepeat returns a string consisting of count copies of s.
func stringsRepeat(s string, count int) string {
	b := make([]byte, len(s)*count)
	for i := range b {
		b[i] = s[i%len(s)]
	}
	return string(b)
}

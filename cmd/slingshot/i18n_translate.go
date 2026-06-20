package main

import (
	"fmt"
	"os"

	"github.com/fatih/color"
	"github.com/spf13/cobra"

	"github.com/nanjj/slingshot/internal/i18n"
)

// cmdI18nTranslate implements "slingshot i18n translate".
// Sets the translation for a single msgid in a locale's .po file.
// This is the precise, one-at-a-time alternative to batch translation
// scripts — every edit is reviewed and applied individually.
type cmdI18nTranslate struct {
	global  *cmdGlobal
	i18nCmd *cmdI18n
	msgid   string // --msgid flag: .po-escaped msgid to find
	msgstr  string // --msgstr flag: .po-escaped translation to set
}

func (c *cmdI18nTranslate) command() *cobra.Command {
	cmd := &cobra.Command{}
	cmd.Use = "translate <locale>"
	cmd.Short = i18n.G("Set translation for a single msgid")
	cmd.Long = i18n.G(`Set the translation (msgstr) for a single msgid in a locale's .po file.

This is the precise, one-at-a-time translation command.  It handles
.po escaping correctly for all edge cases — multi-line entries,
embedded quotes, backslashes, and newlines all round-trip faithfully.

The --msgid and --msgstr values must be given in .po-escaped form:
  \\   →  literal backslash
  \"   →  literal double-quote
  \n   →  newline

To copy the exact msgid from a .po file, take the string content
between the quotes of the msgid line (including any \n, \\, \" escapes).

Examples:
  slingshot i18n translate zh_CN --msgid "Hello" --msgstr "你好"
  slingshot i18n translate zh_CN --msgid "Error: %v" --msgstr "错误：%v"
  slingshot i18n translate zh_CN --msgid "Line1\nLine2" --msgstr "行1\n行2"

Workflow:
  1. slingshot i18n check <locale>       — list untranslated entries
  2. slingshot i18n show <locale> <id>   — inspect a specific entry
  3. slingshot i18n translate <locale> \
      --msgid "<exact msgid>" --msgstr "<translation>"`)
	cmd.Flags().StringVar(&c.msgid, "msgid", "",
		"Exact msgid to translate (required, .po-escaped form)")
	cmd.Flags().StringVar(&c.msgstr, "msgstr", "",
		"Translation text (.po-escaped form); empty string clears the translation")
	cmd.RunE = c.run
	cmd.Args = cobra.ExactArgs(1)
	return cmd
}

func (c *cmdI18nTranslate) run(cmd *cobra.Command, args []string) error {
	locale := args[0]
	dir := c.i18nCmd.resolveDir()

	if c.msgid == "" {
		return fmt.Errorf("--msgid is required")
	}

	// Load full .po entries with comments and ordering preserved.
	entries, err := loadPOFull(dir, locale)
	if err != nil {
		return fmt.Errorf("loading %s .po file: %w", locale, err)
	}

	// Convert user-provided .po-escaped msgid to internal (unescaped) form
	// so it matches the entries loaded by parsePOFull.
	targetMsgid := unescapePO(c.msgid)

	for i := range entries {
		if entries[i].Msgid != targetMsgid {
			continue
		}
		if entries[i].Msgid == "" {
			return fmt.Errorf("cannot translate the header entry (empty msgid)")
		}

		oldMsgstr := entries[i].Msgstr
		// Store the unescaped form internally; writePO → escapePO will
		// re-escape it correctly when saving to the .po file.
		entries[i].Msgstr = unescapePO(c.msgstr)

		if err := savePO(dir, locale, entries); err != nil {
			return fmt.Errorf("writing %s .po file: %w", locale, err)
		}

		// --- Rich summary ---
		fmt.Fprintf(color.Output, "%s  %s\n",
			color.GreenString("✓"),
			color.GreenString(i18n.G("Translation updated for %s"), locale))
		fmt.Fprintf(color.Output, "\n  %s %s\n",
			color.YellowString("msgid:"), c.msgid)
		if oldMsgstr != "" {
			fmt.Fprintf(color.Output, "  %s %s\n",
				color.RedString(i18n.G("was:")), oldMsgstr)
		}
		if c.msgstr != "" {
			fmt.Fprintf(color.Output, "  %s %s\n",
				color.GreenString(i18n.G("now:")), c.msgstr)
		} else {
			fmt.Fprintf(color.Output, "  %s %s\n",
				color.RedString(i18n.G("now:")),
				color.RedString(i18n.G("(empty — translation cleared)")))
		}
		return nil
	}

	// --- msgid not found — helpful diagnostics ---
	return c.msgidNotFound(entries, locale, targetMsgid)
}

// msgidNotFound displays suggestions when the exact msgid is not in the .po file.
func (c *cmdI18nTranslate) msgidNotFound(entries []poEntry, locale string, targetMsgid string) error {
	fmt.Fprintf(color.Output, "%s  %s\n",
		color.RedString("✗"),
		color.RedString(i18n.G("msgid %q not found in %s .po file"), c.msgid, locale))

	// Gather untranslated entries as a quick reference.
	var untranslated []string
	for _, e := range entries {
		if e.Msgid != "" && e.Msgstr == "" {
			untranslated = append(untranslated, e.Msgid)
		}
	}
	if len(untranslated) > 0 {
		fmt.Fprintf(os.Stderr, "\n  %s\n",
			color.CyanString(i18n.G("Untranslated entries in %s (%d total):"),
				locale, len(untranslated)))
		show := 10
		if len(untranslated) < show {
			show = len(untranslated)
		}
		for i, u := range untranslated[:show] {
			display := truncateMsgid(u, 60)
			fmt.Fprintf(os.Stderr, "    %3d.  %s\n", i+1, display)
		}
		if len(untranslated) > show {
			fmt.Fprintf(os.Stderr, i18n.G("    ... and %d more\n"),
				len(untranslated)-show)
		}
		fmt.Fprintf(os.Stderr, "\n  %s  slingshot i18n show %s <id>\n",
			color.YellowString(i18n.G("Tip:")), locale)
	}
	return fmt.Errorf("msgid %q not found in %s .po file", c.msgid, locale)
}

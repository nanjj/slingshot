package main

import (
	"github.com/fatih/color"
	"github.com/spf13/cobra"

	cli "github.com/nanjj/slingshot/internal/cmd"
)

// cmdI18n implements the "slingshot i18n" parent command.
// Note: this command intentionally uses plain English strings instead of
// i18n.G() because it manages .po files — using i18n would create a
// circular dependency (the command must work even when .po files are broken).
type cmdI18n struct {
	global *cmdGlobal
	dir    string // --dir flag: path to locales directory
}

func (c *cmdI18n) command() *cobra.Command {
	cmd := &cobra.Command{}
	cmd.Use = "i18n"
	cmd.Short = "Manage translation (.po) files"
	cmd.Long = cli.FormatSection(
		color.CyanString("Description:"),
		`Manage translation (.po) files for the slingshot i18n system.

Subcommands:
  check     Check locale consistency (missing/untranlated/orphaned entries)
  stats     Show translation statistics per locale
  sync      Synchronize .po files with i18n.G() calls in Go source code
  show      Show full details of an untranslated entry by ID
  add       Initialize a new locale from en_US template`,
	)

	cmd.PersistentFlags().StringVarP(&c.dir, "dir", "d", "",
		"Locales directory path (default: internal/i18n/locales)")
	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		return cmd.Help()
	}

	cmd.AddCommand(
		c.cmdCheck().command(),
		c.cmdStats().command(),
		c.cmdSync().command(),
		c.cmdShow().command(),
		c.cmdAdd().command(),
	)

	return cmd
}

// resolveDir returns the locales directory path.
func (c *cmdI18n) resolveDir() string {
	if c.dir != "" {
		return c.dir
	}
	return "internal/i18n/locales"
}

// --- Factory methods ---

func (c *cmdI18n) cmdCheck() *cmdI18nCheck {
	return &cmdI18nCheck{
		global:  c.global,
		i18nCmd: c,
	}
}

func (c *cmdI18n) cmdStats() *cmdI18nStats {
	return &cmdI18nStats{
		global:  c.global,
		i18nCmd: c,
	}
}

func (c *cmdI18n) cmdSync() *cmdI18nSync {
	return &cmdI18nSync{
		global:  c.global,
		i18nCmd: c,
	}
}

func (c *cmdI18n) cmdShow() *cmdI18nShow {
	return &cmdI18nShow{
		global:  c.global,
		i18nCmd: c,
	}
}

func (c *cmdI18n) cmdAdd() *cmdI18nAdd {
	return &cmdI18nAdd{
		global:  c.global,
		i18nCmd: c,
	}
}

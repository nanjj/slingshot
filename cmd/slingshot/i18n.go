package main

import (
	"path/filepath"

	"github.com/fatih/color"
	"github.com/spf13/cobra"

	cli "github.com/nanjj/slingshot/internal/cmd"
	"github.com/nanjj/slingshot/internal/i18n"
)

// cmdI18n implements the "slingshot i18n" parent command.
// Uses i18n.G() for user-facing strings. The workflow ensures safety:
//  1. i18n.G() calls are added to source code
//  2. slingshot i18n sync (old binary) extracts msgids to .po
//  3. Translations are added to .po files
//  4. New binary is built — .po has translations for every call site
type cmdI18n struct {
	global *cmdGlobal
	dir    string // --dir flag: path to locales directory
}

func (c *cmdI18n) command() *cobra.Command {
	cmd := &cobra.Command{}
	cmd.Use = "i18n"
	cmd.Short = i18n.G("Manage translation (.po) files")
	cmd.Long = cli.FormatSection(
		color.CyanString(i18n.G("Description:")),
		i18n.G(`Manage translation (.po) files for the slingshot i18n system.

`),
	)

	cmd.PersistentFlags().StringVarP(&c.dir, "dir", "d", "",
		i18n.G("Locales directory path (default: internal/i18n/locales)"))
	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		return cmd.Help()
	}

	cmd.AddCommand(
		c.cmdCheck().command(),
		c.cmdVerify().command(),
		c.cmdStats().command(),
		c.cmdSync().command(),
		c.cmdShow().command(),
		c.cmdTranslate().command(),
		c.cmdAdd().command(),
		c.cmdInit().command(),
		c.cmdPot().command(),
	)

	return cmd
}

// resolveDir returns the locales directory path.
// The default "internal/i18n/locales" is resolved relative to the project root
// (found by walking up for go.mod), so it works from any subdirectory.
// If --dir is explicitly given, it's returned as-is (unchanged behavior).
func (c *cmdI18n) resolveDir() string {
	if c.dir != "" {
		return c.dir
	}
	root, err := findGoModRoot()
	if err != nil {
		return "internal/i18n/locales" // fallback to CWD-relative
	}
	return filepath.Join(root, "internal/i18n/locales")
}

// resolveRoot returns the Go module root directory (containing go.mod).
// Falls back to "." when go.mod cannot be found.
func (c *cmdI18n) resolveRoot() string {
	root, err := findGoModRoot()
	if err != nil {
		return "."
	}
	return root
}

// --- Factory methods ---

func (c *cmdI18n) cmdCheck() *cmdI18nCheck {
	return &cmdI18nCheck{
		global:  c.global,
		i18nCmd: c,
	}
}

func (c *cmdI18n) cmdVerify() *cmdI18nVerify {
	return &cmdI18nVerify{
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

func (c *cmdI18n) cmdTranslate() *cmdI18nTranslate {
	return &cmdI18nTranslate{
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

func (c *cmdI18n) cmdInit() *cmdI18nInit {
	return &cmdI18nInit{
		global:  c.global,
		i18nCmd: c,
	}
}

func (c *cmdI18n) cmdPot() *cmdI18nPot {
	return &cmdI18nPot{
		global:  c.global,
		i18nCmd: c,
	}
}

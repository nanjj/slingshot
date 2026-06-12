package main

import (
	"github.com/fatih/color"
	"github.com/spf13/cobra"

	cli "github.com/nanjj/slingshot/internal/cmd"
	"github.com/nanjj/slingshot/internal/i18n"
	u "github.com/nanjj/slingshot/internal/usage"
)

// page 子命令语法
var pageListUsage = u.Usage{
	u.Name,
}

var pageAddUsage = u.Usage{
	u.Name,
	u.File.List(1),
}

var pageUpdateUsage = u.Usage{
	u.Name,
	u.File.List(1),
}

var pageRemoveUsage = u.Usage{
	u.Name,
	u.Placeholder("page"),
}

// cmdPage 是 page 的父命令。
type cmdPage struct {
	global *cmdGlobal
}

func (c *cmdPage) command() *cobra.Command {
	cmd := &cobra.Command{}
	cmd.Use = "page"
	cmd.Short = i18n.G("Manage site pages")
	cmd.Long = cli.FormatSection(
		color.CyanString("Description:"),
		i18n.G(`Manage static site pages.

Each page is a subdirectory under the site's directory containing an
index.html and its assets (images, etc.).

Subcommands:
  list    <site>            List all pages in a site
  add     <site> <file>...  Add new pages from HTML/Org files
  update  <site> <file>...  Update existing pages from HTML/Org files
  remove  <site> <page>     Remove a page
`),
	)
	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		return cmd.Help()
	}

	cmd.AddCommand(
		c.cmdList().command(),
		c.cmdAdd().command(),
		c.cmdUpdate().command(),
		c.cmdRemove().command(),
	)

	return cmd
}

func (c *cmdPage) cmdList() *cmdPageSub {
	return &cmdPageSub{
		global:  c.global,
		name:    "list",
		usage:   pageListUsage,
		short:   i18n.G("List pages in a site"),
		long:    i18n.G("List all pages in a deployment site."),
		minArgs: 1,
		action:  c.doList,
	}
}

func (c *cmdPage) cmdAdd() *cmdPageAdd {
	return &cmdPageAdd{
		global: c.global,
		update: false,
	}
}

func (c *cmdPage) cmdUpdate() *cmdPageAdd {
	return &cmdPageAdd{
		global: c.global,
		update: true,
	}
}

func (c *cmdPage) cmdRemove() *cmdPageSub {
	return &cmdPageSub{
		global:  c.global,
		name:    "remove",
		usage:   pageRemoveUsage,
		short:   i18n.G("Remove a page"),
		long:    i18n.G("Remove a page and its assets from a site."),
		minArgs: 2,
		action:  c.doRemove,
	}
}

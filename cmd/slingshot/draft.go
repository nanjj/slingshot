package main

import (
	"github.com/fatih/color"
	"github.com/spf13/cobra"

	cli "github.com/nanjj/slingshot/internal/cmd"
	"github.com/nanjj/slingshot/internal/i18n"
	u "github.com/nanjj/slingshot/internal/usage"
)

// draft 子命令语法定义
// 注意: cobra 已处理子命令名 (list/add/update/remove/show/convert),
// 这里只定义子命令后的参数。所有用法使用顶层 atom 序列。

var draftListUsage = u.Usage{}

var draftAddUsage = u.Usage{
	u.File,
}

var draftUpdateUsage = u.Usage{
	u.File,
}



var draftRemoveUsage = u.Usage{
	u.ID,
}

var draftShowUsage = u.Usage{
	u.ID,
}

// cmdDraft 是 draft 的父命令。
type cmdDraft struct {
	global *cmdGlobal
}

func (c *cmdDraft) command() *cobra.Command {
	cmd := &cobra.Command{}
	cmd.Use = "draft"
	cmd.Short = i18n.G("Manage WeChat drafts")
	cmd.Long = cli.FormatSection(
		color.CyanString("Description:"),
	i18n.G(`Manage WeChat public account drafts.

The "add" command saves the returned media_id to a sidecar YAML file
(<file>.yaml) alongside the HTML file. Subsequent "update" commands
can then use just the HTML file path — the media_id is read from the
sidecar YAML automatically. If no sidecar exists, the first draft in
the list is used as a default.

Subcommands:
  list              List all drafts
  add    <file>     Create a new draft from HTML file
  update <file>     Update a draft (auto-detect from sidecar YAML or first draft)
  remove <id>       Remove a draft (id, index, or file with sidecar YAML)
  show   <id>       Show a draft's details (id, index, or file with sidecar YAML)
  convert <file>    Convert Markdown to WeChat HTML format
`),
	)
	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		return cmd.Help()
	}

	// 注册子命令
	cmd.AddCommand(
		c.cmdList().command(),
		c.cmdAdd().command(),
		c.cmdUpdate().command(),
		c.cmdRemove().command(),
		c.cmdShow().command(),
		c.cmdConvert().command(),
	)

	return cmd
}

func (c *cmdDraft) cmdList() *cmdDraftSub {
	return &cmdDraftSub{
		global: c.global,
		name:   "list",
		usage:  draftListUsage,
		short:  i18n.G("List all drafts"),
		long:   i18n.G(`List all WeChat public account drafts with their IDs and titles.`),
		action: c.doList,
	}
}

func (c *cmdDraft) cmdAdd() *cmdDraftAdd {
	return &cmdDraftAdd{
		global: c.global,
	}
}

func (c *cmdDraft) cmdUpdate() *cmdDraftUpdate {
	return &cmdDraftUpdate{
		global: c.global,
	}
}

func (c *cmdDraft) cmdRemove() *cmdDraftSub {
	return &cmdDraftSub{
		global: c.global,
		name:   "remove",
		usage:  draftRemoveUsage,
		short:  i18n.G("Remove a draft"),
		long:   i18n.G(`Remove (delete) a WeChat draft by ID or 1-based index from "list".`),
		action: c.doRemove,
	}
}

func (c *cmdDraft) cmdShow() *cmdDraftSub {
	return &cmdDraftSub{
		global: c.global,
		name:   "show",
		usage:  draftShowUsage,
		short:  i18n.G("Show a draft's details"),
		long:   i18n.G(`Show detailed information about a WeChat draft by ID or 1-based index from "list".`),
		action: c.doShow,
	}
}

func (c *cmdDraft) cmdConvert() *cmdDraftConvert {
	return &cmdDraftConvert{
		global: c.global,
	}
}

package main

import (
	"github.com/fatih/color"
	"github.com/spf13/cobra"

	cli "github.com/nanjj/slingshot/internal/cmd"
	"github.com/nanjj/slingshot/internal/i18n"
)

// cmdMeterial 是 meterial 的父命令。
type cmdMeterial struct {
	global *cmdGlobal
}

func (c *cmdMeterial) command() *cobra.Command {
	cmd := &cobra.Command{}
	cmd.Use = "meterial"
	cmd.Short = i18n.G("Manage WeChat permanent materials")
	cmd.Long = cli.FormatSection(
		color.CyanString("Description:"),
		i18n.G(`Manage WeChat public account permanent materials.

Subcommands:
  add    <file>     Upload a permanent material
  list              List permanent materials
  remove <id>       Remove a permanent material
  show   <id>       Show a permanent material's details
`),
	)
	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		return cmd.Help()
	}

	cmd.AddCommand(
		c.cmdAdd().command(),
		c.cmdList().command(),
		c.cmdRemove().command(),
		c.cmdShow().command(),
	)

	return cmd
}

func (c *cmdMeterial) cmdAdd() *cmdMeterialAdd {
	return &cmdMeterialAdd{
		global: c.global,
	}
}

func (c *cmdMeterial) cmdList() *cmdMeterialList {
	return &cmdMeterialList{
		global: c.global,
	}
}

func (c *cmdMeterial) cmdRemove() *cmdMeterialRemove {
	return &cmdMeterialRemove{
		global: c.global,
	}
}

func (c *cmdMeterial) cmdShow() *cmdMeterialShow {
	return &cmdMeterialShow{
		global: c.global,
	}
}

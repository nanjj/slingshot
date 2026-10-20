package main

import (
	"github.com/fatih/color"
	"github.com/spf13/cobra"

	cli "github.com/nanjj/slingshot/internal/cmd"
	"github.com/nanjj/slingshot/internal/i18n"
)

// cmdDeepseek 是 deepseek 的父命令。
type cmdDeepseek struct {
	global *cmdGlobal
}

func (c *cmdDeepseek) command() *cobra.Command {
	cmd := &cobra.Command{}
	cmd.Use = "deepseek"
	cmd.Short = i18n.G("Search the web and fetch pages via the DeepSeek tools API")
	cmd.Long = cli.FormatSection(
		color.CyanString("Description:"),
		i18n.G(`Search the web and fetch pages as Markdown via the DeepSeek
tools API.

Subcommands:
  search  <word>...       Search the web, print title, url and summary
  fetch   <url>           Fetch a page, print its Markdown body

Configuration:
  slingshot config set deepseek.key <key>   # or export DEEPSEEK_API_KEY`),
	)
	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		return cmd.Help()
	}

	cmd.AddCommand(
		(&cmdDeepseekSearch{global: c.global}).command(),
		(&cmdDeepseekFetch{global: c.global}).command(),
	)
	return cmd
}

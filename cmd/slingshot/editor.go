package main

import (
	"github.com/fatih/color"
	"github.com/spf13/cobra"

	cli "github.com/nanjj/slingshot/internal/cmd"
)

// cmdEditor implements the "slingshot editor" parent command.
// The editor command group exposes code editing capabilities via MCP stdio.
type cmdEditor struct {
	global *cmdGlobal
}

func (c *cmdEditor) command() *cobra.Command {
	cmd := &cobra.Command{}
	cmd.Use = "editor"
	cmd.Short = "Code editor with tree-sitter support"
	cmd.Long = cli.FormatSection(
		color.CyanString("Description:"),
		`Start an MCP stdio server for AI code editing.

The editor command uses tree-sitter to provide code structure analysis
and editing capabilities through the Model Context Protocol (MCP).

Subcommands:
  serve    Start MCP stdio server for AI code editing`,
	)
	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		return cmd.Help()
	}
	cmd.SilenceUsage = true

	cmd.AddCommand(
		c.cmdServe().command(),
	)

	return cmd
}

// --- Factory methods ---

func (c *cmdEditor) cmdServe() *cmdEditorServe {
	return &cmdEditorServe{
		global: c.global,
		editor: c,
	}
}

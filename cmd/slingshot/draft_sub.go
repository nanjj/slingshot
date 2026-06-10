package main

import (
	"github.com/fatih/color"
	cli "github.com/nanjj/slingshot/internal/cmd"
	u "github.com/nanjj/slingshot/internal/usage"
	"github.com/spf13/cobra"
)

// --- cmdDraftSub ---

// cmdDraftSub 是 draft 的子命令模板（用于 list/remove/show 等无额外标志的子命令）。
type cmdDraftSub struct {
	global *cmdGlobal
	name   string
	usage  u.Usage
	short  string
	long   string
	action func(parsed []*u.Parsed, cmd *cobra.Command) error
}

func (s *cmdDraftSub) command() *cobra.Command {
	cmd := &cobra.Command{}
	cmd.Use = s.name
	cmd.Short = s.short
	cmd.Long = cli.FormatSection(
		color.CyanString("Description:"),
		s.long,
	)
	cmd.RunE = s.run
	cmd.Args = cobra.ArbitraryArgs
	return cmd
}

func (s *cmdDraftSub) run(cmd *cobra.Command, args []string) error {
	parsed, err := s.global.Parse(s.usage, cmd, args)
	if err != nil {
		return err
	}
	return s.action(parsed, cmd)
}

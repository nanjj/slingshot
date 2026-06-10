package main

import (
	"errors"
	"fmt"

	"github.com/fatih/color"
	cli "github.com/nanjj/slingshot/internal/cmd"
	"github.com/nanjj/slingshot/internal/i18n"
	"github.com/nanjj/slingshot/internal/material"
	u "github.com/nanjj/slingshot/internal/usage"
	"github.com/spf13/cobra"
)

// --- cmdMeterialRemove ---

// cmdMeterialRemove implements "slingshot meterial remove <id>".
type cmdMeterialRemove struct {
	global *cmdGlobal
}

func (c *cmdMeterialRemove) command() *cobra.Command {
	cmd := &cobra.Command{}
	cmd.Use = "remove " + u.ID.Render()
	cmd.Short = i18n.G("Remove a permanent material")
	cmd.Long = cli.FormatSection(
		color.CyanString("Description:"),
		i18n.G(`Remove (delete) a permanent material by its media ID.

Example:
  slingshot meterial remove <media_id>
`),
	)
	cmd.RunE = c.run
	cmd.Args = cobra.ExactArgs(1)
	return cmd
}

func (c *cmdMeterialRemove) run(cmd *cobra.Command, args []string) error {
	parsed, err := c.global.Parse(u.Usage{u.ID}, cmd, args)
	if err != nil {
		return err
	}
	if len(parsed) < 1 || parsed[0].Skipped {
		return errors.New(i18n.G("expected a media_id argument"))
	}
	mediaID := parsed[0].String

	token, err := loadToken()
	if err != nil {
		return err
	}

	if err := material.Remove(token, mediaID); err != nil {
		return fmt.Errorf("removing material: %w", err)
	}

	fmt.Fprintf(cmd.OutOrStdout(), i18n.G("Material removed: %s\n"), color.GreenString(mediaID))
	return nil
}

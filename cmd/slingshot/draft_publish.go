package main

import (
	"errors"
	"fmt"

	"github.com/fatih/color"
	cli "github.com/nanjj/slingshot/internal/cmd"
	"github.com/nanjj/slingshot/internal/draft"
	"github.com/nanjj/slingshot/internal/i18n"
	u "github.com/nanjj/slingshot/internal/usage"
	"github.com/spf13/cobra"
)

// --- cmdDraftPublish ---

// cmdDraftPublish implements "slingshot draft publish <media_id>".
type cmdDraftPublish struct {
	global *cmdGlobal
}

func (c *cmdDraftPublish) command() *cobra.Command {
	cmd := &cobra.Command{}
	cmd.Use = "publish " + u.ID.Render()
	cmd.Short = i18n.G("Publish a draft to WeChat")
	cmd.Long = cli.FormatSection(
		color.CyanString("Description:"),
		i18n.G(`Submit a draft for publishing via the WeChat FreePublish API.

Only one thing: submit. No polling, no retry, no status check.

The publish_id is returned for tracking the submission status.
Use it with the WeChat freepublish/get API to check the result.`),
	)
	cmd.RunE = c.run
	cmd.Args = cobra.ArbitraryArgs
	return cmd
}

func (c *cmdDraftPublish) run(cmd *cobra.Command, args []string) error {
	parsed, err := c.global.Parse(draftPublishUsage, cmd, args)
	if err != nil {
		return err
	}
	if len(parsed) < 1 || parsed[0].Skipped {
		return errors.New(i18n.G("expected a media_id argument"))
	}
	mediaID := parsed[0].String

	// Load config and get token
	token, err := loadToken()
	if err != nil {
		return err
	}

	// Submit for publishing
	resp, err := draft.Publish(token, mediaID)
	if err != nil {
		return fmt.Errorf("publishing draft: %w", err)
	}

	fmt.Fprintf(cmd.OutOrStdout(), i18n.G("Draft published: %s  publish_id: %s\n"),
		color.GreenString(mediaID), color.YellowString(resp.PublishID))
	return nil
}

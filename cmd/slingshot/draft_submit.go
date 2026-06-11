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

// --- cmdDraftSubmit ---

// cmdDraftSubmit implements "slingshot draft submit <media_id>".
// Uses the FreePublish API to submit a draft for publishing without
// sending to subscribers.
type cmdDraftSubmit struct {
	global *cmdGlobal
}

func (c *cmdDraftSubmit) command() *cobra.Command {
	cmd := &cobra.Command{}
	cmd.Use = "submit " + u.ID.Render()
	cmd.Short = i18n.G("Submit a draft for publishing (FreePublish, no mass send)")
	cmd.Long = cli.FormatSection(
		color.CyanString("Description:"),
		i18n.G(`Submit a draft for publishing via the WeChat FreePublish API.

This publishes the article to your account's public timeline without
sending a mass notification to subscribers. For mass send with user
targeting options, use "draft publish".

Only one thing: submit. No polling, no retry, no status check.

The publish_id is returned for tracking the submission status.
Use it with the WeChat freepublish/get API to check the result.`),
	)
	cmd.RunE = c.run
	cmd.Args = cobra.ArbitraryArgs
	return cmd
}

func (c *cmdDraftSubmit) run(cmd *cobra.Command, args []string) error {
	parsed, err := c.global.Parse(draftSubmitUsage, cmd, args)
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
		return fmt.Errorf("submitting draft: %w", err)
	}

	fmt.Fprintf(cmd.OutOrStdout(), i18n.G("Draft submitted: %s  publish_id: %s\n"),
		color.GreenString(mediaID), color.YellowString(resp.PublishID))
	return nil
}

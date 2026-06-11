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
// Uses the SendAll API (群发推送) to publish and optionally send to subscribers.
type cmdDraftPublish struct {
	global            *cmdGlobal
	toAll             bool
	tagID             int
	sendIgnoreReprint bool
	clientMsgID       string
}

func (c *cmdDraftPublish) command() *cobra.Command {
	cmd := &cobra.Command{}
	cmd.Use = "publish " + u.ID.Render()
	cmd.Short = i18n.G("Publish and mass send a draft to subscribers")
	cmd.Long = cli.FormatSection(
		color.CyanString("Description:"),
		i18n.G(`Publish a draft article and send it to subscribers via the
WeChat mass send API (SendAll).

By default, the article is sent to all subscribers (--to-all=true).
Use --tag-id to target a specific user group instead.

This matches the "发布" behavior in the WeChat web backend where
mass send (群发推送) settings are configurable.

The msg_id is returned for tracking the send status via
the masssend/get API.

To publish without sending to subscribers, use "draft submit".`),
	)
	cmd.Flags().BoolVarP(&c.toAll, "to-all", "", true,
		i18n.G("Send to all subscribers (default true). Set --to-all=false and --tag-id to target a group"))
	cmd.Flags().IntVarP(&c.tagID, "tag-id", "", 0,
		i18n.G("Send to a specific tag group (requires --to-all=false)"))
	cmd.Flags().BoolVarP(&c.sendIgnoreReprint, "send-ignore-reprint", "", false,
		i18n.G("Continue sending even if the article is deemed a reprint"))
	cmd.Flags().StringVarP(&c.clientMsgID, "clientmsgid", "", "",
		i18n.G("Client message ID for deduplication (max 32 bytes, prevents duplicate sends within 24h)"))
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

	// Determine send target
	var tagID *int
	if c.tagID > 0 {
		tagID = &c.tagID
	}

	sendIgnore := 0
	if c.sendIgnoreReprint {
		sendIgnore = 1
	}

	resp, err := draft.SendAll(token, mediaID, c.toAll, tagID, sendIgnore, c.clientMsgID)
	if err != nil {
		return fmt.Errorf("sending draft: %w", err)
	}

	fmt.Fprintf(cmd.OutOrStdout(), i18n.G("Draft published: %s  msg_id: %d\n"),
		color.GreenString(mediaID), resp.MsgID)
	return nil
}

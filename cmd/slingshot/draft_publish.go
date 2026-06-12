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
// Calls FreePublish (发布到时间线) always. When --mass is set, also calls
// SendAll (群发推送) — matching the web UI's "发布" behavior.
type cmdDraftPublish struct {
	global            *cmdGlobal
	mass              bool
	toAll             bool
	tagID             int
	sendIgnoreReprint bool
	clientMsgID       string
}

func (c *cmdDraftPublish) command() *cobra.Command {
	cmd := &cobra.Command{}
	cmd.Use = "publish " + u.ID.Render()
	cmd.Short = i18n.G("Publish a draft to WeChat")
	cmd.Long = cli.FormatSection(
		color.CyanString("Description:"),
		i18n.G(`Publish a draft article to your account's public timeline.
+
By default, this calls the FreePublish API to make the article visible
on your official account's timeline (no push notification to subscribers).
+
To also mass-send (群发推送) to subscribers, add --mass.
Use --tag-id to target a specific user group instead of all subscribers.
+
This matches the "发布" behavior in the WeChat web backend, where
mass send settings are configurable options.
+
Without --mass: returns publish_id for tracking via freepublish/get.
With --mass: also returns msg_id for tracking via masssend/get.`),
	)
	cmd.Flags().BoolVarP(&c.mass, "mass", "m", false,
		i18n.G("Also mass send (群发推送) to subscribers after publishing"))
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

	token, err := loadToken()
	if err != nil {
		return err
	}

	// Step 1: FreePublish — publish to timeline (always)
	pubResp, err := draft.Publish(token, mediaID)
	if err != nil {
		return fmt.Errorf("publishing draft: %w", err)
	}

	fmt.Fprintf(cmd.OutOrStdout(), i18n.G("Draft published: %s  publish_id: %s\n"),
		color.GreenString(mediaID), color.YellowString(pubResp.PublishID))

	// Step 2: if --mass, also SendAll
	if c.mass {
		var tagID *int
		if c.tagID > 0 {
			tagID = &c.tagID
		}
		sendIgnore := 0
		if c.sendIgnoreReprint {
			sendIgnore = 1
		}
		sendResp, err := draft.SendAll(token, mediaID, c.toAll, tagID, sendIgnore, c.clientMsgID)
		if err != nil {
			return fmt.Errorf("mass sending draft: %w", err)
		}
		fmt.Fprintf(cmd.OutOrStdout(), i18n.G("Mass send: msg_id: %d  msg_data_id: %d\n"),
			sendResp.MsgID, sendResp.MsgDataID)
	}

	return nil
}

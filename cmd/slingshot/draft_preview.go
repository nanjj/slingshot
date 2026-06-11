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

// --- cmdDraftPreview ---

// cmdDraftPreview implements "slingshot draft preview <media_id>".
// Uses the Preview API to send a draft to a specific user for preview.
type cmdDraftPreview struct {
	global  *cmdGlobal
	touser  string
	towxname string
}

func (c *cmdDraftPreview) command() *cobra.Command {
	cmd := &cobra.Command{}
	cmd.Use = "preview " + u.ID.Render()
	cmd.Short = i18n.G("Preview a draft by sending to a specific user")
	cmd.Long = cli.FormatSection(
		color.CyanString("Description:"),
		i18n.G(`Send a draft article to a specific user for preview before publishing.

Uses the WeChat preview API. Provide either:
  --touser   <openid>    Send to a user by their OpenID
  --towxname <wxname>    Send to a user by their WeChat ID (limited to 100/day)

The preview lets you check the article layout and formatting
in the mobile client before mass sending.`),
	)
	cmd.Flags().StringVarP(&c.touser, "touser", "", "",
		i18n.G("User's OpenID to receive the preview"))
	cmd.Flags().StringVarP(&c.towxname, "towxname", "", "",
		i18n.G("User's WeChat ID to receive the preview (100/day limit)"))
	cmd.RunE = c.run
	cmd.Args = cobra.ArbitraryArgs
	return cmd
}

func (c *cmdDraftPreview) run(cmd *cobra.Command, args []string) error {
	parsed, err := c.global.Parse(draftPreviewUsage, cmd, args)
	if err != nil {
		return err
	}
	if len(parsed) < 1 || parsed[0].Skipped {
		return errors.New(i18n.G("expected a media_id argument"))
	}
	mediaID := parsed[0].String

	if c.touser == "" && c.towxname == "" {
		return errors.New(i18n.G("either --touser or --towxname is required for preview"))
	}

	// Load config and get token
	token, err := loadToken()
	if err != nil {
		return err
	}

	resp, err := draft.Preview(token, mediaID, c.touser, c.towxname)
	if err != nil {
		return fmt.Errorf("previewing draft: %w", err)
	}

	fmt.Fprintf(cmd.OutOrStdout(), i18n.G("Preview sent for %s\n"),
		color.GreenString(mediaID))
	if resp.ErrCode != 0 {
		return fmt.Errorf("preview failed: code %d, %s", resp.ErrCode, resp.ErrMsg)
	}
	return nil
}

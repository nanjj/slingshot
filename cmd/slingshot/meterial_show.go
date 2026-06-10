package main

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/fatih/color"
	cli "github.com/nanjj/slingshot/internal/cmd"
	"github.com/nanjj/slingshot/internal/config"
	"github.com/nanjj/slingshot/internal/getaccesstoken"
	"github.com/nanjj/slingshot/internal/i18n"
	"github.com/nanjj/slingshot/internal/material"
	u "github.com/nanjj/slingshot/internal/usage"
	"github.com/spf13/cobra"
)

// --- cmdMeterialShow ---

// cmdMeterialShow implements "slingshot meterial show <id>".
type cmdMeterialShow struct {
	global *cmdGlobal
	output string
}

func (c *cmdMeterialShow) command() *cobra.Command {
	cmd := &cobra.Command{}
	cmd.Use = "show " + u.ID.Render()
	cmd.Short = i18n.G("Show a permanent material's details")
	cmd.Long = cli.FormatSection(
		color.CyanString("Description:"),
		i18n.G(`Show detailed information about a permanent material by media ID.

For image and voice materials, the raw content can be saved to a file with --output.
For news and video materials, the metadata is displayed as text.

Examples:
  slingshot meterial show <media_id>
  slingshot meterial show <media_id> --output image.jpg
`),
	)
	cmd.Flags().StringVarP(&c.output, "output", "o", "",
		i18n.G("Save to file (for image/voice material)"))
	cmd.RunE = c.run
	cmd.Args = cobra.ArbitraryArgs
	return cmd
}

func (c *cmdMeterialShow) run(cmd *cobra.Command, args []string) error {
	parsed, err := c.global.Parse(u.Usage{u.ID}, cmd, args)
	if err != nil {
		return err
	}
	if len(parsed) < 1 || parsed[0].Skipped {
		return errors.New(i18n.G("expected a media_id argument"))
	}
	mediaID := parsed[0].String

	cfg, _, err := config.Load()
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}
	token, err := getaccesstoken.GetToken(cfg)
	if err != nil {
		return fmt.Errorf("getting access token: %w", err)
	}

	resp, err := material.Show(token, mediaID)
	if err != nil {
		return fmt.Errorf("showing material: %w", err)
	}

	// Determine if this is a news/video response (has JSON content) or binary
	if len(resp.NewsItem) > 0 {
		// News material
		for i, art := range resp.NewsItem {
			if i > 0 {
				fmt.Fprintln(cmd.OutOrStdout(), strings.Repeat("─", 60))
			}
			fmt.Fprintf(cmd.OutOrStdout(), "%s %d\n", color.CyanString(i18n.G("Article")), i+1)
			fmt.Fprintf(cmd.OutOrStdout(), "  %s  %s\n", color.YellowString(i18n.G("Title:")), art.Title)
			if art.Author != "" {
				fmt.Fprintf(cmd.OutOrStdout(), "  %s  %s\n", color.YellowString(i18n.G("Author:")), art.Author)
			}
			if art.Digest != "" {
				fmt.Fprintf(cmd.OutOrStdout(), "  %s  %s\n", color.YellowString(i18n.G("Digest:")), art.Digest)
			}
			if art.URL != "" {
				fmt.Fprintf(cmd.OutOrStdout(), "  %s  %s\n", color.YellowString(i18n.G("URL:")), art.URL)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "  %s  %s\n", color.YellowString(i18n.G("Cover media ID:")), art.ThumbMediaID)
			fmt.Fprintf(cmd.OutOrStdout(), "  %s  %t\n", color.YellowString(i18n.G("Show cover:")), art.ShowCoverPic != 0)
			fmt.Fprintf(cmd.OutOrStdout(), "  %s  %t\n", color.YellowString(i18n.G("Open comments:")), art.NeedOpenComment != 0)
		}
		return nil
	}

	if resp.VideoTitle != "" {
		// Video material
		fmt.Fprintf(cmd.OutOrStdout(), "  %s  %s\n", color.YellowString(i18n.G("Title:")), resp.VideoTitle)
		if resp.VideoDescription != "" {
			fmt.Fprintf(cmd.OutOrStdout(), "  %s  %s\n", color.YellowString(i18n.G("Description:")), resp.VideoDescription)
		}
		if resp.DownURL != "" {
			fmt.Fprintf(cmd.OutOrStdout(), "  %s  %s\n", color.YellowString(i18n.G("Download URL:")), resp.DownURL)
		}
		return nil
	}

	// Binary content (image/voice)
	if c.output != "" {
		if err := os.WriteFile(c.output, resp.RawBody, 0644); err != nil {
			return fmt.Errorf("writing output %q: %w", c.output, err)
		}
		fmt.Fprintf(cmd.OutOrStdout(), i18n.G("Saved to %s (%d bytes)\n"),
			color.GreenString(c.output), len(resp.RawBody))
	} else {
		fmt.Fprintf(cmd.OutOrStdout(), i18n.G("Binary content, %d bytes. Use --output to save to file.\n"),
			len(resp.RawBody))
	}
	return nil
}

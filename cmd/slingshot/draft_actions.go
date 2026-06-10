package main

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/fatih/color"
	"github.com/spf13/cobra"

	"github.com/nanjj/slingshot/internal/config"
	"github.com/nanjj/slingshot/internal/draft"
	"github.com/nanjj/slingshot/internal/getaccesstoken"
	"github.com/nanjj/slingshot/internal/i18n"
	u "github.com/nanjj/slingshot/internal/usage"
)

// --- Actions ---

func (c *cmdDraft) doList(parsed []*u.Parsed, cmd *cobra.Command) error {
	_ = parsed

	cfg, _, err := config.Load()
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}
	token, err := getaccesstoken.GetToken(cfg)
	if err != nil {
		return fmt.Errorf("getting access token: %w", err)
	}

	resp, err := draft.List(token, 0, 20)
	if err != nil {
		return fmt.Errorf("listing drafts: %w", err)
	}

	if resp.TotalCount == 0 {
		fmt.Fprintln(cmd.OutOrStdout(), i18n.G("No drafts found."))
		return nil
	}

	// Print table header
	fmt.Fprintf(cmd.OutOrStdout(), "\n%s\n",
		color.CyanString(i18n.G("Drafts (%d total):"), resp.TotalCount))
	fmt.Fprintln(cmd.OutOrStdout(), strings.Repeat("─", 80))

	for i, item := range resp.Items {
		title := i18n.G("(no title)")
		if len(item.Content.Articles) > 0 {
			title = item.Content.Articles[0].Title
		}
		t := time.Unix(item.UpdateTime, 0).Format("2006-01-02 15:04")
		num := fmt.Sprintf("%d.", i+1)
		fmt.Fprintf(cmd.OutOrStdout(), "  %s  %s\n", color.YellowString(num), color.GreenString(title))
		fmt.Fprintf(cmd.OutOrStdout(), "      %s  %s\n",
			color.CyanString(i18n.G("ID:")), item.MediaID)
		fmt.Fprintf(cmd.OutOrStdout(), "      %s  %s\n",
			color.CyanString(i18n.G("Updated:")), t)
		if i < len(resp.Items)-1 {
			fmt.Fprintln(cmd.OutOrStdout())
		}
	}
	fmt.Fprintln(cmd.OutOrStdout(), strings.Repeat("─", 80))
	return nil
}

func (c *cmdDraft) doRemove(parsed []*u.Parsed, cmd *cobra.Command) error {
	if len(parsed) < 1 || parsed[0].Skipped {
		return errors.New(i18n.G("expected an id argument"))
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

	if err := draft.Remove(token, mediaID); err != nil {
		return fmt.Errorf("removing draft: %w", err)
	}

	fmt.Fprintf(cmd.OutOrStdout(), i18n.G("Draft removed: %s\n"), color.GreenString(mediaID))
	return nil
}

func (c *cmdDraft) doShow(parsed []*u.Parsed, cmd *cobra.Command) error {
	if len(parsed) < 1 || parsed[0].Skipped {
		return errors.New(i18n.G("expected an id argument"))
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

	resp, err := draft.Show(token, mediaID)
	if err != nil {
		return fmt.Errorf("showing draft: %w", err)
	}

	if len(resp.Articles) == 0 {
		fmt.Fprintln(cmd.OutOrStdout(), i18n.G("Draft has no articles."))
		return nil
	}

	for i, art := range resp.Articles {
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
		if art.ContentSourceURL != "" {
			fmt.Fprintf(cmd.OutOrStdout(), "  %s  %s\n", color.YellowString(i18n.G("Source URL:")), art.ContentSourceURL)
		}
		fmt.Fprintf(cmd.OutOrStdout(), "  %s  %s\n", color.YellowString(i18n.G("Cover media ID:")), art.ThumbMediaID)
		fmt.Fprintf(cmd.OutOrStdout(), "  %s  %t\n", color.YellowString(i18n.G("Show cover:")), art.ShowCoverPic != 0)
		fmt.Fprintf(cmd.OutOrStdout(), "  %s  %t\n", color.YellowString(i18n.G("Open comments:")), art.NeedOpenComment != 0)
		fmt.Fprintf(cmd.OutOrStdout(), "  %s  %t\n", color.YellowString(i18n.G("Only fans comment:")), art.OnlyFansCanComment != 0)
		if art.Content != "" {
			fmt.Fprintf(cmd.OutOrStdout(), "  %s  %s\n", color.YellowString(i18n.G("Content preview:")),
				truncate(stripTags(art.Content), 120))
		}
	}
	return nil
}

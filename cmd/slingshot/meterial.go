package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/fatih/color"
	"github.com/spf13/cobra"

	cli "github.com/nanjj/slingshot/internal/cmd"
	"github.com/nanjj/slingshot/internal/config"
	"github.com/nanjj/slingshot/internal/getaccesstoken"
	"github.com/nanjj/slingshot/internal/i18n"
	"github.com/nanjj/slingshot/internal/material"
	u "github.com/nanjj/slingshot/internal/usage"
)

// cmdMeterial 是 meterial 的父命令。
type cmdMeterial struct {
	global *cmdGlobal
}

func (c *cmdMeterial) command() *cobra.Command {
	cmd := &cobra.Command{}
	cmd.Use = "meterial"
	cmd.Short = i18n.G("Manage WeChat permanent materials")
	cmd.Long = cli.FormatSection(
		color.CyanString("Description:"),
		i18n.G(`Manage WeChat public account permanent materials.

Subcommands:
  add    <file>     Upload a permanent material
  list              List permanent materials
  remove <id>       Remove a permanent material
  show   <id>       Show a permanent material's details
`),
	)
	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		return cmd.Help()
	}

	cmd.AddCommand(
		c.cmdAdd().command(),
		c.cmdList().command(),
		c.cmdRemove().command(),
		c.cmdShow().command(),
	)

	return cmd
}

func (c *cmdMeterial) cmdAdd() *cmdMeterialAdd {
	return &cmdMeterialAdd{
		global: c.global,
	}
}

func (c *cmdMeterial) cmdList() *cmdMeterialList {
	return &cmdMeterialList{
		global: c.global,
	}
}

func (c *cmdMeterial) cmdRemove() *cmdMeterialRemove {
	return &cmdMeterialRemove{
		global: c.global,
	}
}

func (c *cmdMeterial) cmdShow() *cmdMeterialShow {
	return &cmdMeterialShow{
		global: c.global,
	}
}

// --- cmdMeterialAdd ---

// cmdMeterialAdd implements "slingshot meterial add <file>".

type cmdMeterialAdd struct {
	global       *cmdGlobal
	mtype        string
	title        string
	introduction string
}

func (c *cmdMeterialAdd) command() *cobra.Command {
	cmd := &cobra.Command{}
	cmd.Use = "add " + u.File.Render()
	cmd.Short = i18n.G("Upload a permanent material")
	cmd.Long = cli.FormatSection(
		color.CyanString("Description:"),
		i18n.G(`Upload a file as a permanent material to WeChat material management.

The --type flag specifies the material type (default: image). For video
type, --title is required and --introduction is optional.

Examples:
  slingshot meterial add image.png
  slingshot meterial add video.mp4 --type video --title "My Video"
  slingshot meterial add voice.amr --type voice
`),
	)
	cmd.Flags().StringVarP(&c.mtype, "type", "t", "image",
		i18n.G("Material type: image, video, voice"))
	cmd.Flags().StringVarP(&c.title, "title", "", "",
		i18n.G("Title (required for video material)"))
	cmd.Flags().StringVarP(&c.introduction, "introduction", "", "",
		i18n.G("Introduction (optional, for video material)"))
	cmd.RunE = c.run
	cmd.Args = cobra.ArbitraryArgs
	return cmd
}

func (c *cmdMeterialAdd) run(cmd *cobra.Command, args []string) error {
	parsed, err := c.global.Parse(u.Usage{u.File}, cmd, args)
	if err != nil {
		return err
	}
	if len(parsed) < 1 || parsed[0].Skipped {
		return errors.New(i18n.G("expected a file argument"))
	}
	filePath := parsed[0].String

	// Validate file exists
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		return fmt.Errorf(i18n.G("file not found: %s"), filePath)
	}

	// Validate material type
	mtype := material.MaterialType(c.mtype)
	switch mtype {
	case material.TypeImage, material.TypeVideo, material.TypeVoice:
		// valid
	default:
		return fmt.Errorf(i18n.G("invalid material type %q: must be one of: image, video, voice"), c.mtype)
	}

	// Load config and get token
	cfg, _, err := config.Load()
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}
	token, err := getaccesstoken.GetToken(cfg)
	if err != nil {
		return fmt.Errorf("getting access token: %w", err)
	}

	fmt.Fprintf(cmd.OutOrStdout(), i18n.G("Uploading %s as %s...\n"), filepath.Base(filePath), mtype)

	resp, err := material.Add(token, filePath, mtype, c.title, c.introduction)
	if err != nil {
		return fmt.Errorf("uploading material: %w", err)
	}

	fmt.Fprintf(cmd.OutOrStdout(), i18n.G("Uploaded %s -> media_id: %s\n"),
		color.GreenString(filepath.Base(filePath)), color.CyanString(resp.MediaID))
	if resp.URL != "" {
		fmt.Fprintf(cmd.OutOrStdout(), "  %s: %s\n", i18n.G("URL"), resp.URL)
	}
	return nil
}

// --- cmdMeterialList ---

// cmdMeterialList implements "slingshot meterial list".
type cmdMeterialList struct {
	global *cmdGlobal
	mtype  string
	offset int
	count  int
}

func (c *cmdMeterialList) command() *cobra.Command {
	cmd := &cobra.Command{}
	cmd.Use = "list"
	cmd.Short = i18n.G("List permanent materials")
	cmd.Long = cli.FormatSection(
		color.CyanString("Description:"),
		i18n.G(`List permanent materials from WeChat material management.

By default lists image materials. Use --type to list video, voice, or news materials.

Examples:
  slingshot meterial list
  slingshot meterial list --type image
  slingshot meterial list --type video --offset 10 --count 5
`),
	)
	cmd.Flags().StringVarP(&c.mtype, "type", "t", "image",
		i18n.G("Material type: image, video, voice, news"))
	cmd.Flags().IntVarP(&c.offset, "offset", "o", 0,
		i18n.G("Offset for pagination"))
	cmd.Flags().IntVarP(&c.count, "count", "c", 20,
		i18n.G("Number of items to list (max 20)"))
	cmd.RunE = c.run
	cmd.Args = cobra.NoArgs
	return cmd
}

func (c *cmdMeterialList) run(cmd *cobra.Command, args []string) error {
	// Validate material type
	mtype := material.MaterialType(c.mtype)
	switch mtype {
	case material.TypeImage, material.TypeVideo, material.TypeVoice, material.TypeNews:
		// valid
	default:
		return fmt.Errorf(i18n.G("invalid material type %q: must be one of: image, video, voice, news"), c.mtype)
	}

	// Clamp count
	if c.count < 1 {
		c.count = 1
	}
	if c.count > 20 {
		c.count = 20
	}
	if c.offset < 0 {
		c.offset = 0
	}

	// Load config and get token
	cfg, _, err := config.Load()
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}
	token, err := getaccesstoken.GetToken(cfg)
	if err != nil {
		return fmt.Errorf("getting access token: %w", err)
	}

	// List materials
	resp, err := material.List(token, mtype, c.offset, c.count)
	if err != nil {
		return fmt.Errorf("listing materials: %w", err)
	}

	if resp.TotalCount == 0 {
		fmt.Fprintln(cmd.OutOrStdout(), i18n.G("No materials found."))
		return nil
	}

	// Print summary
	fmt.Fprintf(cmd.OutOrStdout(), "%s\n",
		color.CyanString(i18n.G("Materials (%s, %d total, showing %d):"), c.mtype, resp.TotalCount, resp.ItemCount))

	for i, item := range resp.Items {
		t := time.Unix(item.UpdateTime, 0).Format("2006-01-02 15:04")
		num := fmt.Sprintf("%d.", c.offset+i+1)
		fmt.Fprintf(cmd.OutOrStdout(), "  %s  %s\n", color.YellowString(num), color.GreenString(item.Name))
		fmt.Fprintf(cmd.OutOrStdout(), "      %s  %s\n",
			color.CyanString(i18n.G("ID:")), item.MediaID)
		fmt.Fprintf(cmd.OutOrStdout(), "      %s  %s\n",
			color.CyanString(i18n.G("Updated:")), t)
		if i < len(resp.Items)-1 {
			fmt.Fprintln(cmd.OutOrStdout())
		}
	}
	return nil
}

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
	cmd.Args = cobra.ArbitraryArgs
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

	cfg, _, err := config.Load()
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}
	token, err := getaccesstoken.GetToken(cfg)
	if err != nil {
		return fmt.Errorf("getting access token: %w", err)
	}

	if err := material.Remove(token, mediaID); err != nil {
		return fmt.Errorf("removing material: %w", err)
	}

	fmt.Fprintf(cmd.OutOrStdout(), i18n.G("Material removed: %s\n"), color.GreenString(mediaID))
	return nil
}

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

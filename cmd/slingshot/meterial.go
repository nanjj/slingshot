package main

import (
	"fmt"
	"strings"
	"time"

	"github.com/fatih/color"
	"github.com/spf13/cobra"

	cli "github.com/nanjj/slingshot/internal/cmd"
	"github.com/nanjj/slingshot/internal/config"
	"github.com/nanjj/slingshot/internal/getaccesstoken"
	"github.com/nanjj/slingshot/internal/i18n"
	"github.com/nanjj/slingshot/internal/material"
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
  list              List permanent materials
`),
	)
	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		return cmd.Help()
	}

	cmd.AddCommand(
		c.cmdList().command(),
	)

	return cmd
}

func (c *cmdMeterial) cmdList() *cmdMeterialList {
	return &cmdMeterialList{
		global: c.global,
	}
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
	fmt.Fprintf(cmd.OutOrStdout(), "\n%s\n",
		color.CyanString(i18n.G("Materials (%s, %d total, showing %d):"), c.mtype, resp.TotalCount, resp.ItemCount))
	fmt.Fprintln(cmd.OutOrStdout(), strings.Repeat("─", 80))

	for i, item := range resp.Items {
		t := time.Unix(item.UpdateTime, 0).Format("2006-01-02 15:04")
		num := fmt.Sprintf("%d.", i+1)
		fmt.Fprintf(cmd.OutOrStdout(), "  %s  %s\n", color.YellowString(num), color.GreenString(item.Name))
		fmt.Fprintf(cmd.OutOrStdout(), "      %s  %s\n",
			color.CyanString(i18n.G("ID:")), item.MediaID)
		if item.URL != "" {
			fmt.Fprintf(cmd.OutOrStdout(), "      %s  %s\n",
				color.CyanString(i18n.G("URL:")), item.URL)
		}
		fmt.Fprintf(cmd.OutOrStdout(), "      %s  %s\n",
			color.CyanString(i18n.G("Updated:")), t)
		if i < len(resp.Items)-1 {
			fmt.Fprintln(cmd.OutOrStdout())
		}
	}
	fmt.Fprintln(cmd.OutOrStdout(), strings.Repeat("─", 80))
	return nil
}

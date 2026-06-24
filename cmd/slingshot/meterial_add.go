package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/fatih/color"
	cli "github.com/nanjj/slingshot/internal/cmd"
	"github.com/nanjj/slingshot/internal/i18n"
	"github.com/nanjj/slingshot/internal/material"
	u "github.com/nanjj/slingshot/internal/usage"
	"github.com/nanjj/clog"
	"github.com/spf13/cobra"
)

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
	cmd.Args = cobra.MinimumNArgs(1)
	return cmd
}

func (c *cmdMeterialAdd) run(cmd *cobra.Command, args []string) (err error) {
	span, ctx := clog.StartSpanFromContext(cmd.Context(), "material_add")
	defer func() {
		if err != nil {
			clog.Error(ctx, "error", "error", err.Error())
		}
		span.Finish()
	}()

	parsed, err := c.global.Parse(u.Usage{u.File}, cmd, args)
	if err != nil {
		return err
	}
	if len(parsed) < 1 || parsed[0].Skipped {
		return errors.New(i18n.G("expected a file argument"))
	}
	filePath := parsed[0].String
	clog.Info(ctx, "material_add", "file", filePath, "type", c.mtype)

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

	// Validate title for video material
	if mtype == material.TypeVideo && c.title == "" {
		return errors.New(i18n.G("--title is required for video material"))
	}

	// Load config and get token
	token, err := loadToken()
	if err != nil {
		return err
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
	clog.Info(ctx, "material_add_result", "mediaID", resp.MediaID)
	return nil
}

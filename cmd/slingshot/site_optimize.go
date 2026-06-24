package main

import (
	"errors"
	"fmt"

	"github.com/fatih/color"
	"github.com/nanjj/clog"
	"github.com/spf13/cobra"

	cli "github.com/nanjj/slingshot/internal/cmd"
	"github.com/nanjj/slingshot/internal/i18n"
	"github.com/nanjj/slingshot/internal/site"
	"github.com/nanjj/slingshot/internal/config"
	u "github.com/nanjj/slingshot/internal/usage"
)

var siteOptimizeUsage = u.Usage{
	u.Name,
}

type cmdSiteOptimize struct {
	global *cmdGlobal
	flagForce bool
}

func (c *cmdSiteOptimize) command() *cobra.Command {
	cmd := &cobra.Command{}
	cmd.Use = "optimize " + u.Name.Render()
	cmd.Short = i18n.G("Optimize site CSS for responsive display")
	cmd.Long = cli.FormatSection(
		color.CyanString("Description:"),
		i18n.G(`Upgrade the site's style.css to the latest responsive version.

Checks for the responsive sentinel marker in the existing CSS. If the CSS
is already optimized, no changes are made unless --force is specified.

This command can be run at any time — before rsync, after adding pages,
or as a standalone optimization step.`),
	)
	cmd.Flags().BoolVar(&c.flagForce, "force", false, i18n.G("Force upgrade even if CSS is already optimized"))
	cmd.RunE = c.run
	cmd.Args = cobra.ArbitraryArgs
	return cmd
}

func (c *cmdSiteOptimize) run(cmd *cobra.Command, args []string) (err error) {
	span, ctx := clog.StartSpanFromContext(cmd.Context(), "site_optimize")
	defer func() {
		if err != nil {
			clog.Error(ctx, "error", "error", err.Error())
		}
		span.Finish()
	}()

	parsed, err := c.global.Parse(siteOptimizeUsage, cmd, args)
	if err != nil {
		return err
	}
	if len(parsed) < 1 || parsed[0].Skipped {
		return errors.New(i18n.G("expected a site name argument"))
	}
	name := parsed[0].String
	clog.Info(ctx, "site_optimize", "name", name, "force", c.flagForce)

	cfg, _, err := config.Load()
	if err != nil {
		return fmt.Errorf("%s: %w", i18n.G("loading config"), err)
	}

	siteConfig, ok := config.GetSite(cfg, name)
	if !ok {
		return fmt.Errorf(i18n.G("site %q not found"), name)
	}
	if siteConfig.Dir == "" {
		return fmt.Errorf(i18n.G("site %q has no directory configured"), name)
	}

	upgraded, err := site.UpgradeCSS(siteConfig.Dir, c.flagForce)
	if err != nil {
		return fmt.Errorf("optimizing site CSS: %w", err)
	}

	if upgraded {
		fmt.Fprintf(color.Output, "%s %s\n", color.GreenString("✓"), i18n.G("CSS upgraded to responsive version."))
	} else {
		fmt.Fprintf(color.Output, "%s %s\n", color.CyanString("i"), i18n.G("CSS is already optimized."))
	}

	clog.Info(ctx, "site_optimize_result", "upgraded", upgraded)
	return nil
}

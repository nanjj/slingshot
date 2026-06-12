package main

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"sort"

	"github.com/fatih/color"
	"github.com/spf13/cobra"

	cli "github.com/nanjj/slingshot/internal/cmd"
	"github.com/nanjj/slingshot/internal/config"
	"github.com/nanjj/slingshot/internal/i18n"
	u "github.com/nanjj/slingshot/internal/usage"
)

// site 子命令语法
var siteListUsage = u.Usage{}

var siteAddUsage = u.Usage{
	u.Name,
}

var siteRemoveUsage = u.Usage{
	u.Name,
}

var siteRsyncUsage = u.Usage{
	u.Name,
}

// cmdSite 是 site 的父命令。
type cmdSite struct {
	global *cmdGlobal
}

func (c *cmdSite) command() *cobra.Command {
	cmd := &cobra.Command{}
	cmd.Use = "site"
	cmd.Short = i18n.G("Manage deployment sites")
	cmd.Long = cli.FormatSection(
		color.CyanString("Description:"),
		i18n.G(`Manage static site deployment targets.

Each site has a local directory and an optional rsync command for deployment.

Subcommands:
  list                  List all configured sites
  add    <name>         Add a new site (use --dir and --rsync)
  remove <name>         Remove a site
  rsync  <name>         Deploy site via rsync
`),
	)
	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		return cmd.Help()
	}

	cmd.AddCommand(
		c.cmdList().command(),
		c.cmdAdd().command(),
		c.cmdRemove().command(),
		c.cmdRsync().command(),
	)

	return cmd
}

func (c *cmdSite) cmdList() *cmdSiteSub {
	return &cmdSiteSub{
		global:  c.global,
		name:    "list",
		usage:   siteListUsage,
		short:   i18n.G("List all sites"),
		long:    i18n.G("List all configured deployment sites."),
		minArgs: 0,
		action:  c.doList,
	}
}

func (c *cmdSite) cmdAdd() *cmdSiteAdd {
	return &cmdSiteAdd{
		global: c.global,
	}
}

func (c *cmdSite) cmdRemove() *cmdSiteSub {
	return &cmdSiteSub{
		global:  c.global,
		name:    "remove",
		usage:   siteRemoveUsage,
		short:   i18n.G("Remove a site"),
		long:    i18n.G("Remove a deployment site from configuration."),
		minArgs: 1,
		action:  c.doRemove,
	}
}

func (c *cmdSite) cmdRsync() *cmdSiteRsync {
	return &cmdSiteRsync{
		global: c.global,
	}
}

// Actions

func (c *cmdSite) doList(cfg *config.Config, parsed []*u.Parsed) error {
	sites := config.GetSites(cfg)
	if len(sites) == 0 {
		fmt.Println(i18n.G("No sites configured."))
		return nil
	}

	sorted := make([]string, 0, len(sites))
	for name := range sites {
		sorted = append(sorted, name)
	}
	sort.Strings(sorted)

	fmt.Fprintf(color.Output, "%s\n\n",
		color.CyanString(i18n.G("Sites (%d total):"), len(sites)))

	for _, name := range sorted {
		site := sites[name]
		fmt.Fprintf(color.Output, "  %s\n", color.GreenString(name))
		if site.Dir != "" {
			fmt.Fprintf(color.Output, "    %s %s\n", color.CyanString(i18n.G("Dir:")), site.Dir)
		}
		if site.Rsync != "" {
			fmt.Fprintf(color.Output, "    %s %s\n", color.CyanString(i18n.G("Rsync:")), site.Rsync)
		}
	}
	return nil
}

func (c *cmdSite) doAdd(cfg *config.Config, parsed []*u.Parsed, dir, rsync string) error {
	if len(parsed) < 1 || parsed[0].Skipped {
		return errors.New(i18n.G("expected a site name argument"))
	}
	name := parsed[0].String

	if dir == "" {
		return errors.New(i18n.G("--dir is required"))
	}

	// Create directory if it doesn't exist
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("creating site directory: %w", err)
	}

	site := config.Site{Dir: dir, Rsync: rsync}
	if err := config.AddSite(cfg, name, site); err != nil {
		return fmt.Errorf("adding site: %w", err)
	}

	path := config.Path()
	if err := config.Save(cfg, path); err != nil {
		return fmt.Errorf("%s: %w", i18n.G("saving config"), err)
	}

	fmt.Fprintf(color.Output, "%s %s\n", i18n.G("Site added:"), color.GreenString(name))
	return nil
}

func (c *cmdSite) doRemove(cfg *config.Config, parsed []*u.Parsed) error {
	if len(parsed) < 1 || parsed[0].Skipped {
		return errors.New(i18n.G("expected a site name argument"))
	}
	name := parsed[0].String

	if _, ok := config.GetSite(cfg, name); !ok {
		return fmt.Errorf(i18n.G("site %q not found"), name)
	}

	if err := config.RemoveSite(cfg, name); err != nil {
		return fmt.Errorf("removing site: %w", err)
	}

	path := config.Path()
	if err := config.Save(cfg, path); err != nil {
		return fmt.Errorf("%s: %w", i18n.G("saving config"), err)
	}

	fmt.Fprintf(color.Output, "%s %s\n", i18n.G("Site removed:"), color.GreenString(name))
	return nil
}

// --- cmdSiteSub ---

// cmdSiteSub 是 site 的通用子命令模板（用于 list/remove 等无额外标志的子命令）。
type cmdSiteSub struct {
	global  *cmdGlobal
	name    string
	usage   u.Usage
	short   string
	long    string
	minArgs int
	action  func(cfg *config.Config, parsed []*u.Parsed) error
}

func (s *cmdSiteSub) command() *cobra.Command {
	cmd := &cobra.Command{}
	cmd.Use = s.name
	cmd.Short = s.short
	cmd.Long = cli.FormatSection(
		color.CyanString("Description:"),
		s.long,
	)
	cmd.RunE = s.run
	cmd.Args = cobra.ArbitraryArgs
	return cmd
}

func (s *cmdSiteSub) run(cmd *cobra.Command, args []string) error {
	cfg, _, err := config.Load()
	if err != nil {
		return fmt.Errorf("%s: %w", i18n.G("loading config"), err)
	}

	if len(args) < s.minArgs && !s.global.flagExplain {
		return errors.New(i18n.G("not enough arguments"))
	}

	parsed, err := s.global.Parse(s.usage, cmd, args)
	if err != nil {
		return err
	}

	return s.action(cfg, parsed)
}

// --- cmdSiteAdd ---

// cmdSiteAdd implements "slingshot site add <name>".
type cmdSiteAdd struct {
	global *cmdGlobal
	dir    string
	rsync  string
}

func (c *cmdSiteAdd) command() *cobra.Command {
	cmd := &cobra.Command{}
	cmd.Use = "add " + u.Name.Render()
	cmd.Short = i18n.G("Add a new site")
	cmd.Long = cli.FormatSection(
		color.CyanString("Description:"),
		i18n.G(`Add a new deployment site.

The --dir flag specifies the local site directory (required).
The --rsync flag specifies the rsync deployment command (optional).`),
	)
	cmd.Flags().StringVarP(&c.dir, "dir", "d", "",
		i18n.G("Local site directory (required)"))
	cmd.Flags().StringVarP(&c.rsync, "rsync", "r", "",
		i18n.G("Rsync deployment command"))
	cmd.RunE = c.run
	cmd.Args = cobra.ArbitraryArgs
	return cmd
}

func (c *cmdSiteAdd) run(cmd *cobra.Command, args []string) error {
	cfg, _, err := config.Load()
	if err != nil {
		return fmt.Errorf("%s: %w", i18n.G("loading config"), err)
	}

	parsed, err := c.global.Parse(siteAddUsage, cmd, args)
	if err != nil {
		return err
	}

	// We need a cmdSite to call doAdd. Let's create a temporary one.
	siteCmd := &cmdSite{global: c.global}
	return siteCmd.doAdd(cfg, parsed, c.dir, c.rsync)
}

// --- cmdSiteRsync ---

// cmdSiteRsync implements "slingshot site rsync <name>".
type cmdSiteRsync struct {
	global *cmdGlobal
}

func (c *cmdSiteRsync) command() *cobra.Command {
	cmd := &cobra.Command{}
	cmd.Use = "rsync " + u.Name.Render()
	cmd.Short = i18n.G("Deploy site via rsync")
	cmd.Long = cli.FormatSection(
		color.CyanString("Description:"),
		i18n.G(`Run the configured rsync command to deploy site content to remote.

The command is executed in the site's local directory.`),
	)
	cmd.RunE = c.run
	cmd.Args = cobra.ArbitraryArgs
	return cmd
}

func (c *cmdSiteRsync) run(cmd *cobra.Command, args []string) error {
	parsed, err := c.global.Parse(siteRsyncUsage, cmd, args)
	if err != nil {
		return err
	}
	if len(parsed) < 1 || parsed[0].Skipped {
		return errors.New(i18n.G("expected a site name argument"))
	}
	name := parsed[0].String

	cfg, _, err := config.Load()
	if err != nil {
		return fmt.Errorf("%s: %w", i18n.G("loading config"), err)
	}

	site, ok := config.GetSite(cfg, name)
	if !ok {
		return fmt.Errorf(i18n.G("site %q not found"), name)
	}

	if site.Rsync == "" {
		return fmt.Errorf(i18n.G("site %q has no rsync command configured"), name)
	}

	if site.Dir == "" {
		return fmt.Errorf(i18n.G("site %q has no directory configured"), name)
	}

	// Change to site directory
	origDir, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("getting current directory: %w", err)
	}
	if err := os.Chdir(site.Dir); err != nil {
		return fmt.Errorf("changing to site directory: %w", err)
	}
	defer os.Chdir(origDir)

	fmt.Fprintf(color.Output, "%s %s\n", i18n.G("Running rsync for site:"), color.GreenString(name))
	fmt.Fprintf(color.Output, "  %s %s\n", color.CyanString(i18n.G("Dir:")), site.Dir)
	fmt.Fprintf(color.Output, "  %s %s\n", color.CyanString(i18n.G("Command:")), site.Rsync)

	// Execute the rsync command via shell
	rsyncCmd := exec.Command("sh", "-c", site.Rsync)
	rsyncCmd.Stdout = os.Stdout
	rsyncCmd.Stderr = os.Stderr
	if err := rsyncCmd.Run(); err != nil {
		return fmt.Errorf("rsync failed: %w", err)
	}

	fmt.Fprintf(color.Output, "%s %s\n", color.GreenString("✓"), i18n.G("Rsync completed successfully."))
	return nil
}

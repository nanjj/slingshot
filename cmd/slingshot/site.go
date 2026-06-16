package main

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"github.com/fatih/color"
	"github.com/spf13/cobra"

	cli "github.com/nanjj/slingshot/internal/cmd"
	"github.com/nanjj/slingshot/internal/config"
	"github.com/nanjj/slingshot/internal/i18n"
	"github.com/nanjj/slingshot/internal/site"
	u "github.com/nanjj/slingshot/internal/usage"
)

// site 子命令语法

var siteListUsage = u.Usage{}

var siteAddUsage = u.Usage{
	u.Name,
	u.Sequence(u.Key, u.Value).List(0),
}

var siteRemoveUsage = u.Usage{
	u.Name,
}

var siteRsyncUsage = u.Usage{
	u.Name,
}

var siteUpdateUsage = u.Usage{
	u.Name,
	u.Key,
	u.Value,
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
		color.CyanString(i18n.G("Description:")),
		i18n.G(`Manage static site deployment targets with type-specific workflows.

Each site has a local directory and optional configuration keys (dir, rsync, etc.).
Site types:
  page (default)  — Ongoing page additions, rsync from site dir directly.
  zine            — Zine-generated site, auto-builds before rsync, rsync from public/.

Subcommands:
  list                  List all configured sites
  add    <name> ...     Add a new site with key=value pairs
  update <name> <k> <v> Update a site's configuration key
  remove <name>         Remove a site
  optimize <name>       Optimize site CSS for responsive display
  rsync  <name>         Deploy site via rsync (auto-builds zine, optimizes CSS)
`),
	)
	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		return cmd.Help()
	}

	cmd.AddCommand(
		c.cmdList().command(),
		c.cmdAdd().command(),
		c.cmdUpdate().command(),
		c.cmdRemove().command(),
		c.cmdOptimize().command(),
		c.cmdRsync().command(),
	)

	return cmd
}

func (c *cmdSite) cmdOptimize() *cmdSiteOptimize {
	return &cmdSiteOptimize{
		global: c.global,
	}
}

// --- cmdSiteSub (通用模板: list, remove) ---

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
		color.CyanString(i18n.G("Description:")),
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

func (c *cmdSite) cmdAdd() *cmdSiteAdd {
	return &cmdSiteAdd{
		global: c.global,
	}
}

func (c *cmdSite) cmdUpdate() *cmdSiteUpdate {
	return &cmdSiteUpdate{
		global: c.global,
	}
}

func (c *cmdSite) cmdRsync() *cmdSiteRsync {
	return &cmdSiteRsync{
		global: c.global,
	}
}



// --- cmdSiteAdd ---

type cmdSiteAdd struct {
	global *cmdGlobal
}

func (c *cmdSiteAdd) command() *cobra.Command {
	cmd := &cobra.Command{}
	cmd.Use = "add " + u.Name.Render() + " [" + u.Key.Render() + " " + u.Value.Render() + "...]"
	cmd.Short = i18n.G("Add a new site")
	cmd.Long = cli.FormatSection(
		color.CyanString(i18n.G("Description:")),
		i18n.G(`Add a new deployment site with key-value configuration pairs.

The first positional argument is the site name. Subsequent arguments are
key-value pairs for site configuration.

Required keys:
  dir   Local site directory

Optional keys:
  rsync Rsync deployment command
  type  Site type: page (default) or zine (auto-build before rsync, public/ output)

Example:
  slingshot site add mysite dir ~/mysite rsync 'rsync -avz --delete ./ user@host:/path' type zine`),
	)
	cmd.RunE = c.run
	cmd.Args = cobra.ArbitraryArgs
	return cmd
}

func (c *cmdSiteAdd) run(cmd *cobra.Command, args []string) error {
	parsed, err := c.global.Parse(siteAddUsage, cmd, args)
	if err != nil {
		return err
	}

	name := parsed[0].String

	// Extract key-value pairs from the list atom
	kvs := make(map[string]string)
	if !parsed[1].Skipped {
		for _, kv := range parsed[1].List {
			key := kv.List[0].String
			value := kv.List[1].String
			kvs[key] = value
		}
	}

	// Validate required keys
	dir, ok := kvs["dir"]
	if !ok {
		return errors.New(i18n.G("'dir' is required"))
	}

	// Create directory if it doesn't exist
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("creating site directory: %w", err)
	}

	// Write optimized shared style.css
	if _, err := site.UpgradeCSS(dir, false); err != nil {
		return fmt.Errorf("writing style.css: %w", err)
	}

	// Load config
	cfg, _, err := config.Load()
	if err != nil {
		return fmt.Errorf("%s: %w", i18n.G("loading config"), err)
	}

	// Set all key-value pairs
	escaped := config.EscapeName(name)
	for key, value := range kvs {
		if err := config.Set(cfg, "sites."+escaped+"."+key, value); err != nil {
			return fmt.Errorf("setting site %s: %w", key, err)
		}
	}

	// Save config
	path := config.Path()
	if err := config.Save(cfg, path); err != nil {
		return fmt.Errorf("%s: %w", i18n.G("saving config"), err)
	}

	fmt.Fprintf(color.Output, "%s %s\n", i18n.G("Site added:"), color.GreenString(name))
	return nil
}

// --- cmdSiteUpdate ---

type cmdSiteUpdate struct {
	global *cmdGlobal
}

func (c *cmdSiteUpdate) command() *cobra.Command {
	cmd := &cobra.Command{}
	cmd.Use = "update " + u.Name.Render() + " " + u.Key.Render() + " " + u.Value.Render()
	cmd.Short = i18n.G("Update a site setting")
	cmd.Long = cli.FormatSection(
		color.CyanString(i18n.G("Description:")),
		i18n.G(`Update a single configuration field on an existing site.

Arguments:
  <name>  Site name
  <key>   Configuration key (e.g. dir, rsync)
  <value> New value

Example:
  slingshot site update mysite rsync 'rsync -avz --delete ./ user@host:/path'`),
	)
	cmd.RunE = c.run
	cmd.Args = cobra.ArbitraryArgs
	return cmd
}

func (c *cmdSiteUpdate) run(cmd *cobra.Command, args []string) error {
	parsed, err := c.global.Parse(siteUpdateUsage, cmd, args)
	if err != nil {
		return err
	}

	name := parsed[0].String
	key := parsed[1].String
	value := parsed[2].String

	// Load config
	cfg, _, err := config.Load()
	if err != nil {
		return fmt.Errorf("%s: %w", i18n.G("loading config"), err)
	}

	// Validate site exists
	if _, ok := config.GetSite(cfg, name); !ok {
		return fmt.Errorf(i18n.G("site %q not found"), name)
	}

	// For dir: create directory and convert $HOME to ~ for portability
	if key == "dir" {
		if err := os.MkdirAll(value, 0755); err != nil {
			return fmt.Errorf("creating site directory: %w", err)
		}
		if home, err := os.UserHomeDir(); err == nil && strings.HasPrefix(value, home) {
			value = "~" + value[len(home):]
		}
	}

	// Set the field
	escaped := config.EscapeName(name)
	if err := config.Set(cfg, "sites."+escaped+"."+key, value); err != nil {
		return fmt.Errorf("setting site %s: %w", key, err)
	}

	// Save config
	path := config.Path()
	if err := config.Save(cfg, path); err != nil {
		return fmt.Errorf("%s: %w", i18n.G("saving config"), err)
	}

	fmt.Fprintf(color.Output, "%s %s %s=%s\n", i18n.G("Site updated:"), color.GreenString(name), key, value)
	return nil
}

// --- doRsync (shared helper) ---

// doRsync executes the rsync command in the given directory.
// It changes to the directory, runs the command via shell, and restores
// the original working directory.
func doRsync(dir, rsyncCmd string) error {
	if rsyncCmd == "" {
		return errors.New(i18n.G("no rsync command configured"))
	}
	if dir == "" {
		return errors.New(i18n.G("no directory configured"))
	}

	origDir, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("getting current directory: %w", err)
	}
	if err := os.Chdir(dir); err != nil {
		return fmt.Errorf("changing to site directory: %w", err)
	}
	defer os.Chdir(origDir)

	cmd := exec.Command("sh", "-c", rsyncCmd)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("rsync failed: %w", err)
	}
	return nil
}

// doBuild runs 'zine build' in the given directory.
func doBuild(dir string) error {
	origDir, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("getting current directory: %w", err)
	}
	if err := os.Chdir(dir); err != nil {
		return fmt.Errorf("changing to zine site directory: %w", err)
	}
	defer os.Chdir(origDir)

	// zine release is the production build command; default output is public/
	// --force allows overwriting non-empty output directory (normal on re-deploy)
	cmd := exec.Command("zine", "release", "--force")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	fmt.Printf("  %s %s\n", color.CyanString(i18n.G("Running:")), "zine release --force")
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("zine release failed: %w", err)
	}
	return nil
}

// siteTypeLabel returns a human-readable label for a site type.
func siteTypeLabel(typ string) string {
	switch typ {
	case "zine":
		return "zine"
	default:
		return "page"
	}
}

type cmdSiteRsync struct {
	global *cmdGlobal
}

func (c *cmdSiteRsync) command() *cobra.Command {
	cmd := &cobra.Command{}
	cmd.Use = "rsync " + u.Name.Render()
	cmd.Short = i18n.G("Deploy site via rsync")
	cmd.Long = cli.FormatSection(
		color.CyanString(i18n.G("Description:")),
		i18n.G(`Run the configured rsync command to deploy site content to remote.

For zine-type sites, automatically runs 'zine build' before rsync
and executes rsync from the 'public/' output directory.
For page-type sites, rsync runs directly from the site directory.`),
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

	siteConfig, ok := config.GetSite(cfg, name)
	if !ok {
		return fmt.Errorf(i18n.G("site %q not found"), name)
	}

	if siteConfig.Rsync == "" {
		return fmt.Errorf(i18n.G("site %q has no rsync command configured"), name)
	}

	fmt.Fprintf(color.Output, "%s %s\n", i18n.G("Running rsync for site:"), color.GreenString(name))
	fmt.Fprintf(color.Output, "  %s %s\n", color.CyanString(i18n.G("Dir:")), siteConfig.Dir)
	fmt.Fprintf(color.Output, "  %s %s\n", color.CyanString(i18n.G("Type:")), siteTypeLabel(siteConfig.Type))
	fmt.Fprintf(color.Output, "  %s %s\n", color.CyanString(i18n.G("Command:")), siteConfig.Rsync)

	// Determine working directory and whether to build
	isZine := siteConfig.Type == "zine"
	rsyncDir := siteConfig.Dir

	if isZine {
		// Zine: build first, then rsync from public/
		publicDir := filepath.Join(siteConfig.Dir, "public")
		fmt.Fprintf(color.Output, "  %s %s -> %s\n", color.CyanString(i18n.G("Build:")), i18n.G("zine build"), publicDir)

		if err := doBuild(siteConfig.Dir); err != nil {
			return err
		}
		rsyncDir = publicDir
	} else {
		// Page: auto-optimize CSS before deploying
		upgraded, err := site.UpgradeCSS(siteConfig.Dir, false)
		if err != nil {
			return fmt.Errorf("optimizing site CSS: %w", err)
		}
		if upgraded {
			fmt.Fprintf(color.Output, "%s %s\n", color.GreenString("✓"), i18n.G("CSS upgraded to responsive version."))
		}
	}

	if err := doRsync(rsyncDir, siteConfig.Rsync); err != nil {
		return err
	}

	fmt.Fprintf(color.Output, "%s %s\n", color.GreenString("✓"), i18n.G("Rsync completed successfully."))
	return nil
}

// --- Actions ---

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
		fmt.Fprintf(color.Output, "    %s %s\n", color.CyanString(i18n.G("Type:")), siteTypeLabel(site.Type))
		if site.Rsync != "" {
			fmt.Fprintf(color.Output, "    %s %s\n", color.CyanString(i18n.G("Rsync:")), site.Rsync)
		}
	}
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

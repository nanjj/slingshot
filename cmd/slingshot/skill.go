package main

import (
	"embed"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"

	"github.com/fatih/color"
	"github.com/spf13/cobra"

	cli "github.com/nanjj/slingshot/internal/cmd"
	"github.com/nanjj/slingshot/internal/i18n"
	u "github.com/nanjj/slingshot/internal/usage"
)

//go:embed embedded_skills
var skillsFS embed.FS

// cmdSkill 是 skill 的父命令。
type cmdSkill struct {
	global *cmdGlobal
}

func (c *cmdSkill) command() *cobra.Command {
	cmd := &cobra.Command{}
	cmd.Use = "skill"
	cmd.Short = i18n.G("Manage built-in skills")
	cmd.Long = cli.FormatSection(
		color.CyanString("Description:"),
		i18n.G(`Manage built-in skills for AI agents.

Subcommands:
  list              List all built-in skills
  install   <name>  Install a built-in skill`),
	)
	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		return cmd.Help()
	}

	cmd.AddCommand(
		c.cmdList().command(),
		c.cmdInstall().command(),
	)

	return cmd
}

func (c *cmdSkill) cmdList() *cmdSkillSub {
	return &cmdSkillSub{
		global: c.global,
		name:   "list",
		usage:  u.Usage{},
		short:  i18n.G("List all built-in skills"),
		long:   i18n.G(`List all skills that are built into the slingshot binary.`),
		action: c.doList,
	}
}

func (c *cmdSkill) cmdInstall() *cmdSkillInstall {
	return &cmdSkillInstall{
		global: c.global,
	}
}

func (c *cmdSkill) doList(parsed []*u.Parsed, cmd *cobra.Command) error {
	_ = parsed

	entries, err := skillsFS.ReadDir("embedded_skills")
	if err != nil {
		return fmt.Errorf("reading embedded skills: %w", err)
	}

	var skills []string
	for _, e := range entries {
		if e.IsDir() {
			skills = append(skills, e.Name())
		}
	}
	sort.Strings(skills)

	if len(skills) == 0 {
		fmt.Fprintln(cmd.OutOrStdout(), i18n.G("No built-in skills found."))
		return nil
	}

	fmt.Fprintln(cmd.OutOrStdout(), color.CyanString(i18n.G("Built-in skills:")))
	for _, s := range skills {
		fmt.Fprintf(cmd.OutOrStdout(), "  %s\n", color.GreenString(s))
	}
	return nil
}

// --- cmdSkillInstall ---

// cmdSkillInstall 实现 "slingshot skill install <name>"。
type cmdSkillInstall struct {
	global *cmdGlobal
	path   string
}

func (c *cmdSkillInstall) command() *cobra.Command {
	cmd := &cobra.Command{}
	cmd.Use = "install " + u.Name.Render()
	cmd.Short = i18n.G("Install a built-in skill to the local skills directory")
	cmd.Long = cli.FormatSection(
		color.CyanString("Description:"),
		i18n.G(`Install a built-in skill to the local skills directory.

The skill's SKILL.md is extracted from the binary and written to the
destination directory. By default the skill is installed under
$PWD/.dscli/skills/<name>/. Use --path to specify a custom directory.

Examples:
  slingshot skill install weixin
  slingshot skill install weixin --path /home/user/.dscli/skills`),
	)
	cmd.Flags().StringVarP(&c.path, "path", "p", "",
		i18n.G("Installation path (default: $PWD/.dscli/skills)"))
	cmd.RunE = c.run
	cmd.Args = cobra.ArbitraryArgs
	return cmd
}
func (c *cmdSkillInstall) run(cmd *cobra.Command, args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("%s", i18n.G("expected a skill name argument"))
	}
	skillName := args[0]

	// Verify the skill exists in embedded FS
	skillDir := filepath.Join("embedded_skills", skillName)
	info, err := fs.Stat(skillsFS, skillDir)
	if err != nil {
		return fmt.Errorf(i18n.G("skill %q not found"), skillName)
	}
	if !info.IsDir() {
		return fmt.Errorf(i18n.G("skill %q not found"), skillName)
	}

	// Read SKILL.md
	skillContent, err := skillsFS.ReadFile(filepath.Join(skillDir, "SKILL.md"))
	if err != nil {
		return fmt.Errorf(i18n.G("skill %q has no SKILL.md"), skillName)
	}

	// Determine install path
	installPath := c.path
	if installPath == "" {
		cwd, err := os.Getwd()
		if err != nil {
			return fmt.Errorf("getting current directory: %w", err)
		}
		installPath = filepath.Join(cwd, ".dscli", "skills")
	}

	// Create destination directory
	destDir := filepath.Join(installPath, skillName)
	if err := os.MkdirAll(destDir, 0755); err != nil {
		return fmt.Errorf("creating directory %q: %w", destDir, err)
	}

	// Write SKILL.md
	destFile := filepath.Join(destDir, "SKILL.md")
	if err := os.WriteFile(destFile, skillContent, 0644); err != nil {
		return fmt.Errorf("writing %q: %w", destFile, err)
	}

	fmt.Fprintf(cmd.OutOrStdout(), i18n.G("Installed skill %s to %s\n"),
		color.GreenString(skillName), color.CyanString(destFile))
	return nil
}

// --- cmdSkillSub ---

// cmdSkillSub 是 skill 子命令模板（用于 list 等无额外标志的子命令）。
type cmdSkillSub struct {
	global *cmdGlobal
	name   string
	usage  u.Usage
	short  string
	long   string
	action func(parsed []*u.Parsed, cmd *cobra.Command) error
}

func (s *cmdSkillSub) command() *cobra.Command {
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

func (s *cmdSkillSub) run(cmd *cobra.Command, args []string) error {
	parsed, err := s.global.Parse(s.usage, cmd, args)
	if err != nil {
		return err
	}
	return s.action(parsed, cmd)
}

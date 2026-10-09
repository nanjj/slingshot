package main

import (
	"embed"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"

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

The skill's whole directory is extracted from the binary: SKILL.md and every
companion file (references/, scripts/, ...) are written to the destination
directory. Regular files are installed with mode 0644; files under scripts/
are installed executable (0755). By default the skill is installed under
<project root>/.dscli/skills/<name>/, where <project root> is the nearest
ancestor of $PWD that contains a .git or .dscli directory (falling back
to $PWD if none is found). Use --path to specify a custom directory.

Examples:
  slingshot skill install weixin
  slingshot skill install weixin --path /home/user/.dscli/skills`),
	)
	cmd.Flags().StringVarP(&c.path, "path", "p", "",
		i18n.G("Installation path (default: <project root>/.dscli/skills)"))
	cmd.Args = cobra.ArbitraryArgs
	cmd.RunE = c.run
	return cmd
}
func (c *cmdSkillInstall) run(cmd *cobra.Command, args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("%s", i18n.G("expected a skill name argument"))
	}
	skillName := args[0]

	// The name is used both as an embedded-FS path segment and as a
	// destination directory name. Reject anything that is not a single path
	// element before touching the filesystem: "..", "." and "a/b" would
	// otherwise make installSkillFS RemoveAll a directory outside the skills
	// install path (e.g. "<project>/.dscli" or the install path itself).
	if !isSinglePathElement(skillName) {
		return fmt.Errorf(i18n.G("invalid skill name %q: must be a single path element"), skillName)
	}

	// Verify the skill exists in embedded FS
	skillDir := path.Join("embedded_skills", skillName)
	info, err := fs.Stat(skillsFS, skillDir)
	if err != nil || !info.IsDir() {
		return fmt.Errorf(i18n.G("skill %q not found"), skillName)
	}

	// Determine install path: use --path, or derive from project root
	installPath := c.path
	if installPath == "" {
		base, err := findProjectRoot()
		if err != nil {
			// Fall back to CWD if not in a git repo
			base, err = os.Getwd()
			if err != nil {
				return fmt.Errorf("getting current directory: %w", err)
			}
		}
		installPath = filepath.Join(base, ".dscli", "skills")
	}

	destDir := filepath.Join(installPath, skillName)
	files, err := installSkillFS(skillsFS, skillDir, destDir)
	if err != nil {
		return err
	}

	fmt.Fprintf(cmd.OutOrStdout(), i18n.G("Installed skill %s to %s (%d file(s))\n"),
		color.GreenString(skillName), color.CyanString(destDir), len(files))
	return nil
}

// isSinglePathElement reports whether name is safe to use as a single path
// element: a non-empty name that is not "." or "..", contains no path
// separator, and equals its own path.Base.
//
// The check is deliberately stricter than the filesystem requires. A name
// such as ".." or "a/b" would either escape the install path or silently
// resolve to a parent directory, and installSkillFS starts by RemoveAll-ing
// the destination.
func isSinglePathElement(name string) bool {
	if name == "" || name == "." || name == ".." {
		return false
	}
	// Defense in depth: the separator check below already rejects "a/b", and
	// path.Base also catches a trailing slash ("weixin/") whose base is the
	// clean name. Keep it even though it looks redundant — removing it would
	// silently re-open trailing-separator names.
	if name != path.Base(name) {
		return false
	}
	return !strings.ContainsAny(name, `/:\`)
}

// installSkillFS copies the whole embedded skill tree rooted at srcDir (an
// fs.FS path such as "embedded_skills/weixin") into destDir on the local
// filesystem and returns the installed file paths, sorted.
//
// Permissions follow one rule: directories are 0755, regular files are 0644,
// and every file under a "scripts/" directory is 0755 so installed helper
// scripts are directly executable.
//
// The destination is wiped first, so a previous installation that contained
// extra files (an older layout) cannot leave stale files behind.
func installSkillFS(fsys fs.FS, srcDir, destDir string) ([]string, error) {
	if err := os.RemoveAll(destDir); err != nil {
		return nil, fmt.Errorf("cleaning previous installation %q: %w", destDir, err)
	}
	if err := os.MkdirAll(destDir, 0755); err != nil {
		return nil, fmt.Errorf("creating directory %q: %w", destDir, err)
	}

	var installed []string
	err := fs.WalkDir(fsys, srcDir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(filepath.FromSlash(srcDir), filepath.FromSlash(p))
		if err != nil {
			return err
		}
		if rel == "." {
			return nil
		}
		dest := filepath.Join(destDir, rel)

		if d.IsDir() {
			if err := os.MkdirAll(dest, 0755); err != nil {
				return fmt.Errorf("creating directory %q: %w", dest, err)
			}
			return nil
		}

		// Stream the file rather than buffering it: skills may ship assets.
		src, err := fsys.Open(p)
		if err != nil {
			return fmt.Errorf("opening %q: %w", p, err)
		}
		defer src.Close()

		mode := fs.FileMode(0644)
		if isScriptPath(rel) {
			mode = 0755
		}
		out, err := os.OpenFile(dest, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, mode)
		if err != nil {
			return fmt.Errorf("creating %q: %w", dest, err)
		}
		if _, err := io.Copy(out, src); err != nil {
			out.Close()
			return fmt.Errorf("writing %q: %w", dest, err)
		}
		if err := out.Close(); err != nil {
			return fmt.Errorf("writing %q: %w", dest, err)
		}
		// OpenFile honours umask on create; chmod restores the intended mode.
		if err := os.Chmod(dest, mode); err != nil {
			return fmt.Errorf("setting mode on %q: %w", dest, err)
		}

		installed = append(installed, dest)
		return nil
	})
	if err != nil {
		return nil, err
	}

	sort.Strings(installed)
	return installed, nil
}

// isScriptPath reports whether a skill-relative path lives under a scripts/
// directory and must therefore be installed executable.
func isScriptPath(rel string) bool {
	for _, part := range strings.Split(filepath.ToSlash(rel), "/") {
		if part == "scripts" {
			return true
		}
	}
	return false
}

// findProjectRoot walks up from the current directory to find the
// project root: the nearest ancestor that contains a .git directory
// or a .dscli directory. Returns an error if neither is found.
func findProjectRoot() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}
	dir, err = filepath.Abs(dir)
	if err != nil {
		return "", err
	}
	for {
		if info, err := os.Stat(filepath.Join(dir, ".git")); err == nil && info.IsDir() {
			return dir, nil
		}
		if info, err := os.Stat(filepath.Join(dir, ".dscli")); err == nil && info.IsDir() {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			// Reached filesystem root without finding .git or .dscli
			return "", fmt.Errorf("no .git or .dscli directory found in any parent of current directory")
		}
		dir = parent
	}
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

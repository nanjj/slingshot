package main

import (
	"errors"
	"fmt"
	"os"
	"sort"

	"github.com/spf13/cobra"

	"github.com/nanjj/slingshot/internal/config"
	cli "github.com/nanjj/slingshot/internal/cmd"
	"github.com/nanjj/slingshot/internal/i18n"
	u "github.com/nanjj/slingshot/internal/usage"
)

// config 子命令语法
// 注意: cobra 已处理子命令名, 这里只定义参数。

var configGetUsage = u.Usage{
	u.Key,
}

var configSetUsage = u.Usage{
	u.Key,
	u.Value,
}

var configUnsetUsage = u.Usage{
	u.Key,
}

// cmdConfig 实现 "slingshot config" 子命令。
type cmdConfig struct {
	global *cmdGlobal
}

func (c *cmdConfig) command() *cobra.Command {
	cmd := &cobra.Command{}
	cmd.Use = "config"
	cmd.Short = i18n.G("Manage configuration")
	cmd.Long = cli.FormatSection(
		fmt.Sprintf("\033[36mDescription:\033[0m"),
		i18n.G(`Manage slingshot configuration file (~/.config/slingshot/config.yml).

Subcommands:
  list                  List all config keys and values
  show                  Show full configuration
  get    <key>          Get a config value
  set    <key> <value>  Set a config value
  unset <key>           Unset (delete) a config key
`),
	)
	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		return cmd.Help()
	}

	cmd.AddCommand(
		(&cmdConfigSub{
			global: c.global,
			name:   "list",
			usage:  u.Usage{},
			short:  i18n.G("List all config keys"),
			long:   i18n.G("List all configuration keys and their current values."),
			minArgs: 0,
			action: func(cfg *config.Config, parsed []*u.Parsed) error {
				return c.doList(cfg)
			},
		}).command(),

		(&cmdConfigSub{
			global: c.global,
			name:   "show",
			usage:  u.Usage{},
			short:  i18n.G("Show full configuration"),
			long:   i18n.G("Display the complete configuration in YAML format."),
			minArgs: 0,
			action: func(cfg *config.Config, parsed []*u.Parsed) error {
				return c.doShow(cfg)
			},
		}).command(),

		(&cmdConfigSub{
			global: c.global,
			name:   "get",
			usage:  configGetUsage,
			short:  i18n.G("Get a config value"),
			long:   i18n.G("Get the value of a specific configuration key."),
			minArgs: 1,
			action: func(cfg *config.Config, parsed []*u.Parsed) error {
				return c.doGet(cfg, parsed)
			},
		}).command(),

		(&cmdConfigSub{
			global: c.global,
			name:   "set",
			usage:  configSetUsage,
			short:  i18n.G("Set a config value"),
			long:   i18n.G("Set a configuration key to a new value and save."),
			minArgs: 2,
			action: func(cfg *config.Config, parsed []*u.Parsed) error {
				return c.doSet(cfg, parsed)
			},
		}).command(),

		(&cmdConfigSub{
			global: c.global,
			name:   "unset",
			usage:  configUnsetUsage,
			short:  i18n.G("Unset (delete) a config key"),
			long:   i18n.G("Unset (delete) a configuration key and save."),
			minArgs: 1,
			action: func(cfg *config.Config, parsed []*u.Parsed) error {
				return c.doUnset(cfg, parsed)
			},
		}).command(),
	)

	return cmd
}

// Actions

func (c *cmdConfig) doList(cfg *config.Config) error {
	keys := config.AllKeys(cfg)
	sorted := make([]string, 0, len(keys))
	for k := range keys {
		sorted = append(sorted, k)
	}
	sort.Strings(sorted)

	for _, k := range sorted {
		fmt.Printf("%s = %v\n", k, keys[k])
	}
	return nil
}

func (c *cmdConfig) doShow(cfg *config.Config) error {
	path := config.Path()
	// Reload from file to show raw YAML content
	loaded, _, err := config.Load()
	if err != nil {
		return err
	}
	// Save to string to display
	if err := config.Save(loaded, path); err != nil {
		return err
	}
	// Read back the file
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("%s: %w", i18n.G("reading config"), err)
	}
	fmt.Print(string(data))
	return nil
}

func (c *cmdConfig) doGet(cfg *config.Config, parsed []*u.Parsed) error {
	if len(parsed) < 1 || parsed[0].Skipped {
		return errors.New(i18n.G("expected a key argument"))
	}
	key := parsed[0].String

	val, err := config.Get(cfg, key)
	if err != nil {
		return err
	}

	fmt.Println(val)
	return nil
}

func (c *cmdConfig) doSet(cfg *config.Config, parsed []*u.Parsed) error {
	if len(parsed) < 2 || parsed[1].Skipped {
		return errors.New(i18n.G("expected key and value arguments"))
	}
	key := parsed[0].String
	val := parsed[1].String

	if err := config.Set(cfg, key, val); err != nil {
		return err
	}

	path := config.Path()
	if err := config.Save(cfg, path); err != nil {
		return fmt.Errorf("%s: %w", i18n.G("saving config"), err)
	}

	fmt.Printf(i18n.G("set %s = %s\n"), key, val)
	return nil
}

func (c *cmdConfig) doUnset(cfg *config.Config, parsed []*u.Parsed) error {
	if len(parsed) < 1 || parsed[0].Skipped {
		return errors.New(i18n.G("expected a key argument"))
	}
	key := parsed[0].String

	if err := config.Del(cfg, key); err != nil {
		return err
	}

	path := config.Path()
	if err := config.Save(cfg, path); err != nil {
		return fmt.Errorf("%s: %w", i18n.G("saving config"), err)
	}

	fmt.Printf(i18n.G("unset %s\n"), key)
	return nil
}

// cmdConfigSub 是 config 的子命令模板。
type cmdConfigSub struct {
	global  *cmdGlobal
	name    string
	usage   u.Usage
	short   string
	long    string
	minArgs int
	action  func(cfg *config.Config, parsed []*u.Parsed) error
}

func (s *cmdConfigSub) command() *cobra.Command {
	cmd := &cobra.Command{}
	cmd.Use = s.name
	cmd.Short = s.short
	cmd.Long = cli.FormatSection(
		fmt.Sprintf("\033[36mDescription:\033[0m"),
		s.long,
	)
	cmd.RunE = s.run
	cmd.Args = cobra.ArbitraryArgs
	return cmd
}

func (s *cmdConfigSub) run(cmd *cobra.Command, args []string) error {
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

	if err := s.action(cfg, parsed); err != nil {
		return err
	}

	return nil
}

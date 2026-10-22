package main

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/nanjj/slingshot/internal/config"
	"github.com/nanjj/slingshot/internal/deepseek"
	"github.com/nanjj/slingshot/internal/i18n"
	u "github.com/nanjj/slingshot/internal/usage"
)

// deepseekFetchUsage 是 fetch 的位置参数语法: 单个 URL。
var deepseekFetchUsage = u.Usage{
	u.Placeholder("url"),
}

// cmdDeepseekFetch 是 deepseek fetch 子命令。
type cmdDeepseekFetch struct {
	global *cmdGlobal
}

func (c *cmdDeepseekFetch) command() *cobra.Command {
	var local bool

	cmd := &cobra.Command{}
	cmd.Use = "fetch <url>"
	cmd.Short = i18n.G("Fetch a page as Markdown via the DeepSeek fetch API")
	cmd.Args = cobra.ArbitraryArgs
	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		parsed, err := c.global.Parse(deepseekFetchUsage, cmd, args)
		if err != nil {
			return err
		}

		url := parsed[0].String
		out := cmd.OutOrStdout()

		if local {
			text, err := deepseek.FetchLocal(cmd.Context(), url)
			if err != nil {
				return err
			}
			fmt.Fprint(out, text)
			if text != "" && text[len(text)-1] != '\n' {
				fmt.Fprintln(out)
			}
			return nil
		}

		cfg, _, err := config.Load()
		if err != nil {
			return fmt.Errorf("loading config: %w", err)
		}

		key, err := deepseek.ResolveKey(cfg)
		if err != nil {
			return err
		}

		client := deepseek.NewClient(key)
		resp, err := client.Fetch(cmd.Context(), url)
		if err != nil {
			return err
		}

		if resp.Title != "" {
			fmt.Fprintf(out, "# %s\n\n", resp.Title)
		}
		fmt.Fprint(out, resp.Content)
		if resp.Content != "" && resp.Content[len(resp.Content)-1] != '\n' {
			fmt.Fprintln(out)
		}
		return nil
	}

	cmd.Flags().BoolVar(&local, "local", true,
		i18n.G("Fetch with the local lightpanda binary (default). Use --local=false for the API"))

	return cmd
}

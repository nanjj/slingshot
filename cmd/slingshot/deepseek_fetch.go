package main

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/nanjj/slingshot/internal/config"
	"github.com/nanjj/slingshot/internal/deepseek"
	"github.com/nanjj/slingshot/internal/i18n"
)

// cmdDeepseekFetch 是 deepseek fetch 子命令。
type cmdDeepseekFetch struct {
	global *cmdGlobal
}

func (c *cmdDeepseekFetch) command() *cobra.Command {
	cmd := &cobra.Command{}
	cmd.Use = "fetch <url>"
	cmd.Short = i18n.G("Fetch a page as Markdown via the DeepSeek fetch API")
	cmd.Args = cobra.ExactArgs(1)
	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		cfg, _, err := config.Load()
		if err != nil {
			return fmt.Errorf("loading config: %w", err)
		}

		key, err := deepseek.ResolveKey(cfg)
		if err != nil {
			return err
		}

		client := deepseek.NewClient(key)
		resp, err := client.Fetch(cmd.Context(), args[0])
		if err != nil {
			return err
		}

		out := cmd.OutOrStdout()
		if resp.Title != "" {
			fmt.Fprintf(out, "# %s\n\n", resp.Title)
		}
		fmt.Fprint(out, resp.Content)
		if resp.Content != "" && resp.Content[len(resp.Content)-1] != '\n' {
			fmt.Fprintln(out)
		}
		return nil
	}

	return cmd
}

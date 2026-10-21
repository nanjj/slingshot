package main

import (
	"errors"
	"fmt"
	"strings"

	"github.com/fatih/color"
	"github.com/spf13/cobra"

	"github.com/nanjj/slingshot/internal/config"
	"github.com/nanjj/slingshot/internal/deepseek"
	"github.com/nanjj/slingshot/internal/i18n"
)

// cmdDeepseekSearch 是 deepseek search 子命令。
type cmdDeepseekSearch struct {
	global *cmdGlobal
}

func (c *cmdDeepseekSearch) command() *cobra.Command {
	var (
		count       int
		needContent bool
		showSummary bool
		local       bool
	)

	cmd := &cobra.Command{}
	cmd.Use = "search <word>..."
	cmd.Short = i18n.G("Search the web via the DeepSeek search API")
	cmd.Args = cobra.MinimumNArgs(1)
	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		cfg, _, err := config.Load()
		if err != nil {
			return fmt.Errorf("loading config: %w", err)
		}

		query := strings.Join(args, " ")
		req := deepseek.SearchRequest{
			Query:       query,
			Count:       count,
			NeedContent: needContent,
		}

		var resp *deepseek.SearchResponse
		if local {
			resp, err = deepseek.SearchLocal(cmd.Context(), cfg, req)
		} else {
			var key string
			key, err = deepseek.ResolveKey(cfg)
			if err != nil {
				return err
			}
			resp, err = deepseek.NewClient(key).Search(cmd.Context(), req)
		}
		if err != nil {
			return err
		}

		if len(resp.Results) == 0 {
			return errors.New(i18n.G("no results"))
		}

		out := cmd.OutOrStdout()
		for i, r := range resp.Results {
			fmt.Fprintf(out, "%d. %s\n", i+1, color.New(color.Bold).Sprint(r.Title))
			fmt.Fprintf(out, "   %s\n", r.URL)
			if r.Snippet != "" {
				fmt.Fprintf(out, "   %s\n", r.Snippet)
			}
			if showSummary && r.Summary != "" {
				fmt.Fprintf(out, "   ---\n   %s\n", r.Summary)
			}
			if needContent && r.Content != "" {
				fmt.Fprintf(out, "   ---\n   %s\n", r.Content)
			}
			fmt.Fprintln(out)
		}
		return nil
	}

	cmd.Flags().IntVarP(&count, "count", "n", 0,
		i18n.G("Number of results (1-50, default 10)"))
	cmd.Flags().BoolVar(&showSummary, "summary", false,
		i18n.G("Include the long summary in the output"))
	cmd.Flags().BoolVar(&needContent, "content", false,
		i18n.G("Include the page content in the output (implies need_content)"))
	cmd.Flags().BoolVar(&local, "local", false,
		i18n.G("Search Google through the remote lightpanda CDP instead of the API"))
	return cmd
}

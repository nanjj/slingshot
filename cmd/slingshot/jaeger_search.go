package main

import (
	"fmt"
	"net/url"

	"github.com/spf13/cobra"

	cli "github.com/nanjj/slingshot/internal/cmd"
	"github.com/nanjj/slingshot/internal/i18n"
	u "github.com/nanjj/slingshot/internal/usage"
)

var jaegerSearchUsage = u.Usage{
	u.Placeholder("service"),
}

// cmdJaegerSearch 实现 "slingshot jaeger search <service>"。
type cmdJaegerSearch struct {
	global *cmdGlobal
	jaeger *cmdJaeger
	limit  int
	tags   string
}

func (c *cmdJaegerSearch) command() *cobra.Command {
	cmd := &cobra.Command{}
	cmd.Use = "search <service>"
	cmd.Short = i18n.G("Search traces for a service")
	cmd.Long = i18n.G(`Search recent traces for a service, with optional limit and tag filtering.

Examples:
  slingshot jaeger search Dscli
  slingshot jaeger search Dscli --limit 5
  slingshot jaeger search Dscli --tags '{"error":"true"}'`)
	cli.AddIntFlag(cmd.Flags(), &c.limit, "limit|n", 10,
		i18n.G("Max trace count"))
	cli.AddStringFlag(cmd.Flags(), &c.tags, "tags|t", "",
		i18n.G("Filter tags as JSON, e.g. '{\"error\":\"true\"}'"))
	cmd.Args = cobra.ArbitraryArgs
	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		if _, err := c.global.Parse(jaegerSearchUsage, cmd, args); err != nil {
			return err
		}
		return c.run(cmd, args)
	}
	return cmd
}

func (c *cmdJaegerSearch) run(cmd *cobra.Command, args []string) error {
	service := url.QueryEscape(args[0])
	path := fmt.Sprintf("/api/traces?service=%s&limit=%d", service, c.limit)
	if c.tags != "" {
		path += "&tags=" + url.QueryEscape(c.tags)
	}
	data, err := c.jaeger.jaegerGet(path)
	if err != nil {
		return err
	}
	printJSON(data)
	return nil
}

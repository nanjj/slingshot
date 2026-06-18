package main

import (
	"fmt"
	"time"

	"github.com/spf13/cobra"

	cli "github.com/nanjj/slingshot/internal/cmd"
	"github.com/nanjj/slingshot/internal/i18n"
)

// cmdJaegerDeps 实现 "slingshot jaeger deps"。
type cmdJaegerDeps struct {
	global   *cmdGlobal
	jaeger   *cmdJaeger
	lookback string
}

func (c *cmdJaegerDeps) command() *cobra.Command {
	cmd := &cobra.Command{}
	cmd.Use = "deps"
	cmd.Short = i18n.G("Get service dependency graph")
	cmd.Long = i18n.G(`Get the service dependency graph from Jaeger.

Examples:
  slingshot jaeger deps
  slingshot jaeger deps --lookback 24h`)
	cli.AddStringFlag(cmd.Flags(), &c.lookback, "lookback|l", "1h",
		i18n.G("Lookback duration (e.g. 1h, 24h, 3600s)"))
	cmd.Args = cobra.NoArgs
	cmd.RunE = c.run
	return cmd
}

func (c *cmdJaegerDeps) run(cmd *cobra.Command, args []string) error {
	// Parse the lookback duration string
	d, err := time.ParseDuration(c.lookback)
	if err != nil {
		return fmt.Errorf(i18n.G("invalid lookback duration %q: %v"), c.lookback, err)
	}

	now := time.Now()
	endEpoch := now.UnixMicro()
	lookbackUs := d.Microseconds()

	path := fmt.Sprintf("/api/dependencies?endEpoch=%d&lookback=%d", endEpoch, lookbackUs)
	data, err := c.jaeger.jaegerGet(path)
	if err != nil {
		return err
	}
	printJSON(data)
	return nil
}

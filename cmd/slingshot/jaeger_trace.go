package main

import (
	"github.com/spf13/cobra"

	"github.com/nanjj/slingshot/internal/i18n"
)

// cmdJaegerTrace 实现 "slingshot jaeger trace <traceID>"。
type cmdJaegerTrace struct {
	global *cmdGlobal
	jaeger *cmdJaeger
}

func (c *cmdJaegerTrace) command() *cobra.Command {
	cmd := &cobra.Command{}
	cmd.Use = "trace <traceID>"
	cmd.Short = i18n.G("Get full trace details")
	cmd.Long = i18n.G("Retrieve the full trace details including all spans, process info, tags, and logs.")
	cmd.Args = cobra.ExactArgs(1)
	cmd.RunE = c.run
	return cmd
}

func (c *cmdJaegerTrace) run(cmd *cobra.Command, args []string) error {
	data, err := c.jaeger.jaegerGet("/api/traces/" + args[0])
	if err != nil {
		return err
	}
	printJSON(data)
	return nil
}

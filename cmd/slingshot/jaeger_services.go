package main

import (
	"github.com/spf13/cobra"

	"github.com/nanjj/slingshot/internal/i18n"
)

// cmdJaegerServices 实现 "slingshot jaeger services"。
type cmdJaegerServices struct {
	global *cmdGlobal
	jaeger *cmdJaeger
}

func (c *cmdJaegerServices) command() *cobra.Command {
	cmd := &cobra.Command{}
	cmd.Use = "services"
	cmd.Short = i18n.G("List all registered services")
	cmd.Long = i18n.G("List all services registered with Jaeger.")
	cmd.Args = cobra.NoArgs
	cmd.RunE = c.run
	return cmd
}

func (c *cmdJaegerServices) run(cmd *cobra.Command, args []string) error {
	data, err := c.jaeger.jaegerGet("/api/services")
	if err != nil {
		return err
	}
	printJSON(data)
	return nil
}

package main

import (
	"fmt"
	"net/url"

	"github.com/spf13/cobra"

	"github.com/nanjj/slingshot/internal/i18n"
)

// cmdJaegerOperations 实现 "slingshot jaeger operations <service>"。
type cmdJaegerOperations struct {
	global *cmdGlobal
	jaeger *cmdJaeger
}

func (c *cmdJaegerOperations) command() *cobra.Command {
	cmd := &cobra.Command{}
	cmd.Use = "operations <service>"
	cmd.Short = i18n.G("List operations for a service")
	cmd.Long = i18n.G("List all operation names registered by a given service.")
	cmd.Args = cobra.ExactArgs(1)
	cmd.RunE = c.run
	return cmd
}

func (c *cmdJaegerOperations) run(cmd *cobra.Command, args []string) error {
	service := url.PathEscape(args[0])
	data, err := c.jaeger.jaegerGet(fmt.Sprintf("/api/services/%s/operations", service))
	if err != nil {
		return err
	}
	printJSON(data)
	return nil
}

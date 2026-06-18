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
	cmd.Long = i18n.G(`Retrieve the full trace details including all spans, process info, tags, and logs.

Subcommands provide progressive-disclosure views:
  topology      Show the service-dependency graph extracted from the trace
  critical-path Identify the span chain most responsible for latency`)
	cmd.Args = cobra.ExactArgs(1)
	cmd.RunE = c.run
	cmd.AddCommand(
		c.cmdTopology().command(),
		c.cmdCriticalPath().command(),
	)
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

// cmdTopology 返回 topology 子命令构建器。
func (c *cmdJaegerTrace) cmdTopology() *cmdJaegerTraceTopology {
	return &cmdJaegerTraceTopology{
		global: c.global,
		jaeger: c.jaeger,
		trace:  c,
	}
}

// cmdCriticalPath 返回 critical-path 子命令构建器。
func (c *cmdJaegerTrace) cmdCriticalPath() *cmdJaegerTraceCriticalPath {
	return &cmdJaegerTraceCriticalPath{
		global: c.global,
		jaeger: c.jaeger,
		trace:  c,
	}
}

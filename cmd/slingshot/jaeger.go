package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	"github.com/fatih/color"
	"github.com/spf13/cobra"

	cli "github.com/nanjj/slingshot/internal/cmd"
	"github.com/nanjj/slingshot/internal/i18n"
)

// cmdJaeger 是 jaeger 的父命令。
type cmdJaeger struct {
	global *cmdGlobal
	host   string
}

func (c *cmdJaeger) command() *cobra.Command {
	cmd := &cobra.Command{}
	cmd.Use = "jaeger"
	cmd.Short = i18n.G("Query Jaeger tracing data via HTTP API")
	cmd.Long = cli.FormatSection(
		color.CyanString("Description:"),
		i18n.G(`Query Jaeger tracing data via the Jaeger Query HTTP API.

Replaces the unreliable Jaeger MCP Server with direct HTTP calls —
no truncation, no timeouts, no extra daemon.

Subcommands:
  services              List all registered services
  operations  <service> List operations for a service
  search      <service> Search traces for a service
  trace       <traceID> Get full trace details
  trace       topology      <traceID> Show trace service dependency graph
  trace       critical-path <traceID> Show trace critical (slowest) path
  deps                  Get service dependency graph

Environment:
  JAEGER_HOST  Jaeger Query URL (default: http://localhost:16686)`),
	)
	cmd.PersistentFlags().StringVarP(&c.host, "host", "H", "",
		i18n.G("Jaeger Query API host (default: http://localhost:16686, or $JAEGER_HOST)"))
	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		return cmd.Help()
	}
	cmd.SilenceUsage = true

	cmd.AddCommand(
		c.cmdServices().command(),
		c.cmdOperations().command(),
		c.cmdSearch().command(),
		c.cmdTrace().command(),
		c.cmdDeps().command(),
	)

	return cmd
}

// resolveHost 返回 Jaeger 地址，优先级: --host > JAEGER_HOST > 默认值
func (c *cmdJaeger) resolveHost() string {
	if c.host != "" {
		return c.host
	}
	if env := os.Getenv("JAEGER_HOST"); env != "" {
		return env
	}
	return "http://localhost:16686"
}

// jaegerGet 向 Jaeger API 发送 GET 请求并返回响应体。
func (c *cmdJaeger) jaegerGet(path string) ([]byte, error) {
	host := c.resolveHost()
	url := host + path

	client := &http.Client{Timeout: 60 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		return nil, fmt.Errorf(i18n.G("request to Jaeger failed: %w"), err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf(i18n.G("reading response body: %w"), err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf(i18n.G("Jaeger API returned %d: %s"), resp.StatusCode, string(body))
	}

	return body, nil
}

// printJSON 格式化输出 JSON。
func printJSON(data []byte) {
	var v interface{}
	if err := json.Unmarshal(data, &v); err != nil {
		// 不是 JSON 则直接输出
		fmt.Println(string(data))
		return
	}
	pretty, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		fmt.Println(string(data))
		return
	}
	fmt.Println(string(pretty))
}

// --- Factory methods ---

// cmdServices 返回 services 子命令构建器。
func (c *cmdJaeger) cmdServices() *cmdJaegerServices {
	return &cmdJaegerServices{
		global: c.global,
		jaeger: c,
	}
}

// cmdOperations 返回 operations 子命令构建器。
func (c *cmdJaeger) cmdOperations() *cmdJaegerOperations {
	return &cmdJaegerOperations{
		global: c.global,
		jaeger: c,
	}
}

// cmdSearch 返回 search 子命令构建器。
func (c *cmdJaeger) cmdSearch() *cmdJaegerSearch {
	return &cmdJaegerSearch{
		global: c.global,
		jaeger: c,
	}
}

// cmdTrace 返回 trace 子命令构建器。
func (c *cmdJaeger) cmdTrace() *cmdJaegerTrace {
	return &cmdJaegerTrace{
		global: c.global,
		jaeger: c,
	}
}

// cmdDeps 返回 deps 子命令构建器。
func (c *cmdJaeger) cmdDeps() *cmdJaegerDeps {
	return &cmdJaegerDeps{
		global: c.global,
		jaeger: c,
	}
}

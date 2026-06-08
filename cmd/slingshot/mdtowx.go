package main

import (
	"fmt"

	"github.com/spf13/cobra"

	cli "github.com/nanjj/slingshot/internal/cmd"
	"github.com/nanjj/slingshot/internal/i18n"
	u "github.com/nanjj/slingshot/internal/usage"
)

// 定义 mdtowx 命令的语法:
//   mdtowx [<remote>:] <file>
// 顶层 atom 序列: cobra 处理了 "mdtowx", 这里只定义参数。
var mdtowxUsage = u.Usage{
	u.RemoteColonOpt, // [<remote>:]
	u.File,           // <file>
}

// cmdMdtowx 实现 "slingshot mdtowx" 子命令。
type cmdMdtowx struct {
	global *cmdGlobal
}

func (c *cmdMdtowx) command() *cobra.Command {
	cmd := &cobra.Command{}
	cmd.Use = "mdtowx " + u.RemoteColonOpt.Render() + " " + u.File.Render()
	cmd.Short = i18n.G("Convert Markdown to WeChat public account HTML")
	cmd.Long = cli.FormatSection(
		fmt.Sprintf("\033[36mDescription:\033[0m"),
		i18n.G(`Convert a Markdown file to HTML format suitable for WeChat public accounts.

The conversion process:
  1. Parse the Markdown file and generate styled HTML
  2. Extract image references from the HTML
  3. Optionally upload images to WeChat (planned)
  4. Save the result as <filename>.html
`),
	)
	cmd.RunE = c.run
	cmd.Args = cobra.ArbitraryArgs

	return cmd
}

func (c *cmdMdtowx) run(cmd *cobra.Command, args []string) error {
	parsed, err := mdtowxUsage.Parse(args)
	if err != nil {
		return err
	}

	// parsed[0] = RemoteColonOpt (Skipped if no remote)
	// parsed[1] = File
	remote := parsed[0] // may be Skipped
	file := parsed[1]

	_ = remote // remote name, if specified

	fmt.Fprintf(cmd.OutOrStdout(), i18n.G("Converting %s to WeChat HTML...\n"), quote(file.String))

	// TODO: 实际的 mdtowx 转换逻辑
	_ = file.String

	fmt.Fprintln(cmd.OutOrStdout(), i18n.G("Done."))
	return nil
}

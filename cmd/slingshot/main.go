package main

import (
	"context"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/nanjj/clog"
	"github.com/nanjj/slingshot/internal/i18n"
	u "github.com/nanjj/slingshot/internal/usage"
)

// cmdGlobal 是全局共享状态, 类似 incus 的 cmdGlobal。
type cmdGlobal struct {
	// 全局标志
	flagHelp    bool
	flagVersion bool
	flagQuiet   bool
	flagExplain bool
}

// Parse 包装 usage.Parse, 注入全局配置 (如 ExplainOnly)。
func (c *cmdGlobal) Parse(usage u.Usage, cmd *cobra.Command, args []string) ([]*u.Parsed, error) {
	return usage.Parse(args, u.Config{ExplainOnly: c.flagExplain})
}

func main() {
	// Initialize Jaeger tracer.
	// dscli pre-seeds CLOG_TRACEPARENT in the environment, so
	// StartSpanFromContext will automatically pick it up as a parent.
	tracer, err := clog.NewTracer("slingshot")
	if err != nil {
		fmt.Fprintf(os.Stderr, "warning: tracing disabled: %v\n", err)
	} else {
		clog.SetGlobalTracer(tracer)
		defer clog.CloseTracer(tracer)
	}

	rootCmd := &cobra.Command{
		Use:           "slingshot",
		Short:         i18n.G("A slingshot for AI agents"),
		Long:          i18n.G(`Slingshot is a multi-purpose CLI for AI agents.`),
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}

	// PersistentPreRunE / PersistentPostRunE: trace every command invocation.
	// StartSpanFromContext automatically extracts the parent trace from
	// CLOG_TRACEPARENT (pre-seeded by dscli) and creates a child span.
	// The span is injected into the returned ctx; we discard the span value
	// and retrieve it later in PersistentPostRunE via SpanFromContext.
	rootCmd.PersistentPreRunE = func(cmd *cobra.Command, args []string) error {
		_, ctx := clog.StartSpanFromContext(cmd.Context(), cmd.CommandPath())
		cmd.SetContext(ctx) // propagate so PersistentPostRunE can find the span
		return nil
	}

	rootCmd.PersistentPostRunE = func(cmd *cobra.Command, args []string) error {
		if span := clog.SpanFromContext(cmd.Context()); span != nil {
			span.Finish()
		}
		return nil
	}
	// 隐藏 completion 和 help 子命令（运行时仍可用）
	rootCmd.CompletionOptions = cobra.CompletionOptions{
		HiddenDefaultCmd: true,
	}
	rootCmd.SetHelpCommand(&cobra.Command{
		Use:    "help [command]",
		Hidden: true,
		Run: func(c *cobra.Command, args []string) {
			cmd, _, e := c.Root().Find(args)
			if cmd == nil || e != nil {
				c.Printf("Unknown help topic %#q\n", args)
				c.Root().Usage()
			} else {
				cmd.Help()
			}
		},
	})
	// 覆盖默认 usage 模板：去掉硬编码的 (eq .Name "help")，让 help 真正隐藏
	// slingshot 不使用命令组(Group)，因此模板简化了组相关逻辑
	rootCmd.SetUsageTemplate(`Usage:{{if .Runnable}}
  {{.UseLine}}{{end}}{{if .HasAvailableSubCommands}}
  {{.CommandPath}} [command]{{end}}{{if gt (len .Aliases) 0}}

Aliases:
  {{.NameAndAliases}}{{end}}{{if .HasExample}}

Examples:
{{.Example}}{{end}}{{if .HasAvailableSubCommands}}

Available Commands:{{range .Commands}}{{if .IsAvailableCommand}}
  {{rpad .Name .NamePadding }} {{.Short}}{{end}}{{end}}{{end}}{{if .HasAvailableLocalFlags}}

Flags:
{{.LocalFlags.FlagUsages | trimTrailingWhitespaces}}{{end}}{{if .HasAvailableInheritedFlags}}

Global Flags:
{{.InheritedFlags.FlagUsages | trimTrailingWhitespaces}}{{end}}{{if .HasHelpSubCommands}}

Additional help topics:{{range .Commands}}{{if .IsAdditionalHelpTopicCommand}}
  {{rpad .CommandPath .CommandPathPadding}} {{.Short}}{{end}}{{end}}{{end}}{{if .HasAvailableSubCommands}}

Use "{{.CommandPath}} [command] --help" for more information about a command.{{end}}
`)

	global := &cmdGlobal{}

	// 全局标志
	rootCmd.PersistentFlags().BoolVarP(&global.flagHelp, "help", "h", false, i18n.G("Show help"))
	rootCmd.PersistentFlags().BoolVarP(&global.flagVersion, "version", "v", false, i18n.G("Show version"))
	rootCmd.PersistentFlags().BoolVarP(&global.flagQuiet, "quiet", "q", false, i18n.G("Quiet mode"))
	rootCmd.PersistentFlags().BoolVar(&global.flagExplain, "explain", false, i18n.G("If the command is valid, explain its parsed arguments instead of running it"))

	// 注册子命令
	rootCmd.AddCommand(
		(&cmdDraft{global: global}).command(),
		(&cmdConfig{global: global}).command(),
		(&cmdMeterial{global: global}).command(),
		(&cmdSkill{global: global}).command(),
		(&cmdSite{global: global}).command(),
		(&cmdPage{global: global}).command(),
		(&cmdJaeger{global: global}).command(),
		(&cmdI18n{global: global}).command(),
		(&cmdCode{global: global}).command(),
	)



	// 处理 version 标志
	rootCmd.SetVersionTemplate("slingshot v0.1.1\n")
	rootCmd.Version = "0.1.1"

	// 执行
	err = rootCmd.ExecuteContext(context.Background())
	if err == u.ErrExplainOnly {
		// --explain 成功完成, 不显示错误信息
		os.Exit(0)
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, i18n.G("Error: %v\n"), err)
		os.Exit(1)
	}
}

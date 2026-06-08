package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/nanjj/slingshot/internal/i18n"
)

// cmdGlobal 是全局共享状态, 类似 incus 的 cmdGlobal。
type cmdGlobal struct {
	// 全局标志
	flagHelp    bool
	flagVersion bool
	flagQuiet   bool
	// 预留: 可添加配置、日志等
}

func main() {
	rootCmd := &cobra.Command{
		Use:           "slingshot",
		Short:         i18n.G("Slingshot — AI agent tools for WeChat public accounts"),
		Long:          i18n.G(`Slingshot is a CLI tool for AI agents to manage WeChat public account content.`),
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}

	global := &cmdGlobal{}

	// 全局标志
	rootCmd.PersistentFlags().BoolVarP(&global.flagHelp, "help", "h", false, i18n.G("Show help"))
	rootCmd.PersistentFlags().BoolVarP(&global.flagVersion, "version", "v", false, i18n.G("Show version"))
	rootCmd.PersistentFlags().BoolVarP(&global.flagQuiet, "quiet", "q", false, i18n.G("Quiet mode"))

	// 注册子命令
	rootCmd.AddCommand(
		(&cmdMdtowx{global: global}).command(),
		(&cmdWxdraft{global: global}).command(),
		(&cmdConfig{global: global}).command(),
	)


	// 处理 version 标志
	rootCmd.SetVersionTemplate("slingshot v0.1.0\n")
	rootCmd.Version = "0.1.0"

	// 执行
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, i18n.G("Error: %v\n"), err)
		os.Exit(1)
	}
}

// quote 用单引号包裹字符串, 用于输出。
func quote(s string) string {
	return "'" + s + "'"
}

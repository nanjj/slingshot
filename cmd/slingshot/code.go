package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/fatih/color"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/spf13/cobra"

	cli "github.com/nanjj/slingshot/internal/cmd"
	"github.com/nanjj/slingshot/internal/code/base"
	"github.com/nanjj/slingshot/internal/code/lsp"
	codemcp "github.com/nanjj/slingshot/internal/code/mcp"
)

// cmdCode implements the "slingshot code" parent command.
// The code command group exposes code intelligence tools via MCP stdio.
type cmdCode struct {
	global *cmdGlobal
}

func (c *cmdCode) command() *cobra.Command {
	cmd := &cobra.Command{}
	cmd.Use = "code"
	cmd.Short = "Code intelligence tools with MCP stdio server"
	cmd.Long = cli.FormatSection(
		color.CyanString("Description:"),
		`Start an MCP stdio server for code intelligence.

The code command provides code search, graph analysis, AST navigation,
and project management through the Model Context Protocol (MCP).

Subcommands:
  serve    Start MCP stdio server for code intelligence`,
	)
	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		return cmd.Help()
	}
	cmd.SilenceUsage = true

	cmd.AddCommand(
		c.cmdServe().command(),
	)

	return cmd
}

// --- Factory methods ---

func (c *cmdCode) cmdServe() *cmdCodeServe {
	return &cmdCodeServe{
		global: c.global,
		code:   c,
	}
}

// ─── Serve command ─────────────────────────────────────────────────────────────

type codeServeOptions struct {
	projectRoot string
	dbPath      string
	logLevel    string
}

type cmdCodeServe struct {
	global *cmdGlobal
	code   *cmdCode
	opts   *codeServeOptions
}

func (c *cmdCodeServe) command() *cobra.Command {
	opts := &codeServeOptions{}
	c.opts = opts

	cmd := &cobra.Command{}
	cmd.Use = "serve"
	cmd.Short = "Start MCP stdio server for code intelligence"
	cmd.Long = cli.FormatSection(
		color.CyanString("Description:"),
		`Start an MCP stdio server that exposes code intelligence tools
over the Model Context Protocol (stdio transport).

The server reads JSON-RPC requests from stdin and writes responses to stdout.
All diagnostic logs are written to stderr.

The server includes:
  - Code search & navigation (BM25, pattern, name-based)
  - Code graph analysis (architecture, schema, trace paths)
  - AST analysis (structure, node, definitions, validation)
  - Change detection & impact analysis
  - Project indexing & management
  - ADR & memo persistence

Environment:
  SLINGSHOT_PROJECT_ROOT  Project root directory (default: current directory)
  SLINGSHOT_CODE_DB       Code graph database path (default: ~/.config/slingshot/code.db)
Flags:
  --project-root <path>   Project root directory
  --db-path <path>        Code graph database path
  --log-level <level>     Log level: debug, info, warn, error (default: warn)`,
	)
	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		// Resolve project root: flag > env > CWD
		if opts.projectRoot == "" {
			opts.projectRoot = os.Getenv("SLINGSHOT_PROJECT_ROOT")
		}
		if opts.projectRoot == "" {
			cwd, err := os.Getwd()
			if err != nil {
				return fmt.Errorf("get current directory: %w", err)
			}
			opts.projectRoot = cwd
		}

		// Resolve DB path: flag > env > default
		if opts.dbPath == "" {
			opts.dbPath = os.Getenv("SLINGSHOT_CODE_DB")
		}
		if opts.dbPath == "" {
			home, err := os.UserHomeDir()
			if err != nil {
				return fmt.Errorf("get home directory: %w", err)
			}
			opts.dbPath = filepath.Join(home, ".config", "slingshot", "code.db")
		}

		return c.run(cmd.Context())
	}
	cmd.SilenceUsage = true

	cmd.Flags().StringVar(&opts.projectRoot, "project-root", "", "Project root directory")
	cmd.Flags().StringVar(&opts.dbPath, "db-path", "", "Code graph database path")
	cmd.Flags().StringVar(&opts.logLevel, "log-level", "warn", "Log level: debug, info, warn, error")

	return cmd
}

func (c *cmdCodeServe) run(ctx context.Context) error {
	opts := c.opts

	// 1. Logger
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
		Level: parseLogLevel(opts.logLevel),
	}))
	slog.SetDefault(logger)

	// 2. Ensure DB directory exists
	dbDir := filepath.Dir(opts.dbPath)
	if err := os.MkdirAll(dbDir, 0755); err != nil {
		return fmt.Errorf("create db directory: %w", err)
	}

	// 3. Open store
	store, err := base.OpenStore(opts.dbPath)
	if err != nil {
		return fmt.Errorf("open store: %w", err)
	}
	defer store.Close()

	// 4. Create analyzer
	analyzer := lsp.NewAnalyzer()

	// 5. Create MCP server
	mcpOpts := &codemcp.Options{
		ProjectRoot: opts.projectRoot,
		DBPath:      opts.dbPath,
	}
	codeServer := codemcp.NewServer(store, analyzer, mcpOpts)

	srv := mcp.NewServer(&mcp.Implementation{
		Name:    "slingshot-code",
		Version: "0.1.0",
	}, &mcp.ServerOptions{
		Logger: logger,
	})

	// 6. Register all tools
	codeServer.RegisterAll(srv)

	// 7. Connect stdio transport
	logger.Info("code MCP server starting",
		"projectRoot", opts.projectRoot,
		"dbPath", opts.dbPath,
		"logLevel", opts.logLevel,
	)
	session, err := srv.Connect(ctx, &mcp.StdioTransport{}, nil)
	if err != nil {
		return fmt.Errorf("connect: %w", err)
	}

	// 8. Wait for shutdown
	logger.Info("code MCP server started")
	return session.Wait()
}

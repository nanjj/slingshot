package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/fatih/color"
	"github.com/spf13/cobra"

	cli "github.com/nanjj/slingshot/internal/cmd"
	"github.com/nanjj/slingshot/internal/config"
	"github.com/nanjj/slingshot/internal/draft"
	"github.com/nanjj/slingshot/internal/getaccesstoken"
	"github.com/nanjj/slingshot/internal/i18n"
	u "github.com/nanjj/slingshot/internal/usage"
)

// --- cmdDraftExtract ---

// cmdDraftExtract implements "slingshot draft extract <id>".
type cmdDraftExtract struct {
	global *cmdGlobal
	output string // --output/-o: write to a file instead of stdout
}

func (c *cmdDraftExtract) command() *cobra.Command {
	cmd := &cobra.Command{}
	cmd.Use = "extract " + u.ID.Render()
	cmd.Short = i18n.G("Print a draft's full content")
	cmd.Long = cli.FormatSection(
		color.CyanString("Description:"),
		i18n.G(`Print the full content of a draft's article(s).

The draft is resolved the same way as "show" and "remove": a sidecar YAML
file with a media_id, a 1-based index from "list", or a raw media_id.

The article content is the raw WeChat HTML and is written to stdout
verbatim, so it can be redirected or piped without colour codes getting
in the way. Warnings go to stderr.

When a draft holds more than one article, each article is preceded by a
"<!-- article N: <title> -->" comment line; a single article is printed
as-is.

Use --output to write the content to a file instead of stdout.`),
	)
	cmd.Flags().StringVarP(&c.output, "output", "o", "",
		i18n.G("Write content to a file instead of stdout"))
	cmd.RunE = c.run
	cmd.Args = cobra.ArbitraryArgs
	return cmd
}

func (c *cmdDraftExtract) run(cmd *cobra.Command, args []string) error {
	parsed, err := c.global.Parse(draftExtractUsage, cmd, args)
	if err != nil {
		return err
	}
	if len(parsed) < 1 || parsed[0].Skipped {
		return errors.New(i18n.G("expected an id argument"))
	}
	idStr := parsed[0].String

	cfg, _, err := config.Load()
	if err != nil {
		return fmt.Errorf("%s: %w", i18n.G("loading config"), err)
	}
	token, err := getaccesstoken.GetToken(cfg)
	if err != nil {
		return fmt.Errorf("%s: %w", i18n.G("getting access token"), err)
	}

	mediaID, err := resolveID(token, idStr)
	if err != nil {
		return fmt.Errorf("%s: %w", i18n.G("resolving draft ID"), err)
	}

	resp, err := draft.Show(token, mediaID)
	if err != nil {
		return fmt.Errorf("%s: %w", i18n.G("showing draft"), err)
	}

	if len(resp.Articles) == 0 {
		fmt.Fprintln(cmd.ErrOrStderr(), i18n.G("Draft has no articles."))
		return nil
	}

	content := formatDraftContents(resp.Articles)

	if c.output != "" {
		if dir := filepath.Dir(c.output); dir != "" && dir != "." {
			if err := os.MkdirAll(dir, 0755); err != nil {
				return fmt.Errorf(i18n.G("creating directory for %q: %w"), c.output, err)
			}
		}
		if err := os.WriteFile(c.output, []byte(content), 0644); err != nil {
			return fmt.Errorf(i18n.G("writing %q: %w"), c.output, err)
		}
		fmt.Fprintf(cmd.ErrOrStderr(), i18n.G("Wrote %d bytes to %s\n"), len(content), c.output)
		return nil
	}

	fmt.Fprint(cmd.OutOrStdout(), content)
	return nil
}

// formatDraftContents renders a draft's articles into the bytes "draft extract"
// emits. It is a pure function so the multi-article layout can be unit tested
// without the WeChat API.
//
// A single article is emitted verbatim (no added trailing newline), which keeps
// redirections byte-identical to the stored content. With more than one
// article, each is preceded by a "<!-- article N: <title> -->" comment and the
// articles are separated by exactly one newline, so the pieces never run
// together on one line.
func formatDraftContents(articles []draft.ArticleInfo) string {
	var sb strings.Builder
	multi := len(articles) > 1
	// prevEndsWithNewline tracks whether the previously written chunk ended
	// with a newline, so the next header can start on its own line without
	// emitting a blank line when it is not needed. The zero value is fine:
	// the first iteration writes a header (which always ends with a newline)
	// before the flag is ever read.
	var prevEndsWithNewline bool

	for i, art := range articles {
		if multi {
			if i > 0 && !prevEndsWithNewline {
				sb.WriteByte('\n')
			}
			fmt.Fprintf(&sb, "<!-- article %d: %s -->\n", i+1, sanitizeCommentText(art.Title))
			prevEndsWithNewline = true
		}
		sb.WriteString(art.Content)
		prevEndsWithNewline = strings.HasSuffix(art.Content, "\n")
	}

	return sb.String()
}

// sanitizeCommentText makes an article title safe to embed in an HTML comment.
// Titles come from the WeChat API and are normally single-line, but an
// embedded newline would break the one-header-per-article layout and a literal
// "-->" would close the comment early (the remainder would then be parsed as
// content). CR/LF collapse to a space; "-->" is HTML-escaped to "--&gt;".
func sanitizeCommentText(s string) string {
	s = strings.ReplaceAll(s, "-->", "--&gt;")
	s = strings.NewReplacer("\r\n", " ", "\n", " ", "\r", " ").Replace(s)
	return s
}

package main

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/fatih/color"
	"github.com/spf13/cobra"

	"github.com/nanjj/slingshot/internal/i18n"
	u "github.com/nanjj/slingshot/internal/usage"
)

var i18nPotUsage = u.Usage{}

// cmdI18nPot implements "slingshot i18n pot".
type cmdI18nPot struct {
	global       *cmdGlobal
	i18nCmd      *cmdI18n
	output       string
	sources      []string
	packageName  string
	bugsAddress  string
	creationDate string
}

func (c *cmdI18nPot) command() *cobra.Command {
	cmd := &cobra.Command{}
	cmd.Use = "pot"
	cmd.Short = i18n.G("Generate the .pot template from source code")
	cmd.Long = i18n.G(`Generate the .pot translation template from i18n.G() and i18n.NG()
calls in the source tree.

The template records each msgid once, with every "#: file:line" location
where it is used, a "#, c-format" flag when the string looks like a printf
format, and msgid_plural when a plural call supplied one. This is what the
gettext toolchain — and xgettext before it — produces, so the file can be
handed to translators or merged into .po files without post-processing.

Unlike xgettext it never aborts on a non-constant argument: such a call is
reported on stderr and skipped.`)
	cmd.Flags().StringVarP(&c.output, "output", "o", "",
		i18n.G("Output path (default: <locales dir>/slingshot.pot)"))
	cmd.Flags().StringArrayVar(&c.sources, "source", nil,
		i18n.G("Restrict the scan to DIR, FILE or GLOB (repeatable; default: whole module)"))
	cmd.Flags().StringVar(&c.packageName, "package-name", "slingshot",
		i18n.G("Project-Id-Version in the template header"))
	cmd.Flags().StringVar(&c.bugsAddress, "msgid-bugs-address", "nanjunjie@gmail.com",
		i18n.G("Report-Msgid-Bugs-To in the template header"))
	cmd.Flags().StringVar(&c.creationDate, "creation-date", "",
		i18n.G("Override POT-Creation-Date (default: now, for reproducible builds)"))
	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		if _, err := c.global.Parse(i18nPotUsage, cmd, args); err != nil {
			return err
		}
		return c.run(cmd, args)
	}
	return cmd
}

func (c *cmdI18nPot) run(cmd *cobra.Command, args []string) error {
	root := c.i18nCmd.resolveRoot()

	color.New(color.Faint).Fprintln(os.Stderr, i18n.G("Scanning source code for i18n calls..."))
	infos, err := extractMsgidsFrom(root, c.sources)
	if err != nil {
		return fmt.Errorf("extracting msgids from source: %w", err)
	}
	if len(infos) == 0 {
		return fmt.Errorf("no i18n calls found in source code")
	}

	date := c.creationDate
	if date == "" {
		date = time.Now().Format("2006-01-02 15:04-0700")
	}

	out := c.output
	if out == "" {
		out = filepath.Join(c.i18nCmd.resolveDir(), "slingshot.pot")
	}

	if err := os.WriteFile(out, []byte(writePot(infos, potHeader(c.packageName, c.bugsAddress, date))), 0644); err != nil {
		return fmt.Errorf("writing %s: %w", out, err)
	}

	withLocations, withFormat, withPlural := 0, 0, 0
	for _, info := range infos {
		if len(info.Locations) > 0 {
			withLocations++
		}
		if hasFlag(info.Flags, "c-format") {
			withFormat++
		}
		if info.Plural != "" {
			withPlural++
		}
	}

	fmt.Fprintf(color.Output, "%s %s (%d %s, %d %s, %d %s, %d %s)\n",
		color.GreenString("\u2713"),
		out,
		len(infos), i18n.G("msgids"),
		withLocations, i18n.G("with locations"),
		withFormat, i18n.G("c-format"),
		withPlural, i18n.G("plural"))

	return nil
}

// potHeader renders the template preamble.
//
// Wording, field order and column alignment follow xgettext, so a project can
// switch between the two generators without the header churning in review.
func potHeader(pkg, bugs, date string) string {
	return "# SOME DESCRIPTIVE TITLE.\n" +
		"# Copyright (C) YEAR THE PACKAGE'S COPYRIGHT HOLDER\n" +
		"# This file is distributed under the same license as the PACKAGE package.\n" +
		"# FIRST AUTHOR <EMAIL@ADDRESS>, YEAR.\n" +
		"#\n" +
		"#, fuzzy\n" +
		"msgid   \"\"\n" +
		"msgstr  \"Project-Id-Version: " + pkg + "\\n\"\n" +
		"        \"Report-Msgid-Bugs-To: " + bugs + "\\n\"\n" +
		"        \"POT-Creation-Date: " + date + "\\n\"\n" +
		"        \"PO-Revision-Date: YEAR-MO-DA HO:MI+ZONE\\n\"\n" +
		"        \"Last-Translator: FULL NAME <EMAIL@ADDRESS>\\n\"\n" +
		"        \"Language-Team: LANGUAGE <LL@li.org>\\n\"\n" +
		"        \"Language: \\n\"\n" +
		"        \"MIME-Version: 1.0\\n\"\n" +
		"        \"Content-Type: text/plain; charset=UTF-8\\n\"\n" +
		"        \"Content-Transfer-Encoding: 8bit\\n\"" +
		"\n"
}

// msgInfo accumulates everything the .pot template needs to say about one
// msgid: where it is used, whether it is a printf-style format string, and
// its plural form if the call site supplied one.
type msgInfo struct {
	Locations []string // "file:line", sorted and de-duplicated
	Flags     []string // gettext flags, currently only "c-format"
	Plural    string   // msgid_plural, empty for singular calls
}

// addLocation records a source location, de-duplicated but otherwise in
// discovery order.
//
// xgettext-go does not sort the location list: it appends each occurrence as
// the AST walk finds it, and files are visited in the order they were passed
// on the command line. Preserving that order is what makes the generated .pot
// byte-identical to xgettext-go's, which is the whole point of the exercise.
func (m *msgInfo) addLocation(loc string) {
	if loc == "" {
		return
	}
	for _, existing := range m.Locations {
		if existing == loc {
			return
		}
	}
	m.Locations = append(m.Locations, loc)
}

// needsCFormat reports whether msgid looks like a printf-style format string.
//
// "%%" is a literal percent sign and must not count, otherwise every msgid
// that merely prints a percent would be flagged.
func needsCFormat(msgid string) bool {
	for i := 0; i < len(msgid); i++ {
		if msgid[i] != '%' {
			continue
		}
		if i+1 < len(msgid) && msgid[i+1] == '%' {
			i++ // skip the escaped pair
			continue
		}
		return true
	}
	return false
}

// potSegments splits value after each newline, keeping the newline at the end
// of its segment: "a\nb" yields ["a\n", "b"], "a\n" yields ["a\n"].
func potSegments(value string) []string {
	if !strings.Contains(value, "\n") {
		return []string{value}
	}

	var segs []string
	for len(value) > 0 {
		i := strings.IndexByte(value, '\n')
		if i < 0 {
			segs = append(segs, value)
			break
		}
		segs = append(segs, value[:i+1])
		value = value[i+1:]
	}
	return segs
}

// writePotValueGNU writes one keyword/value pair the way xgettext does:
// the keyword padded to column 8, continuation lines indented by 8 spaces.
func writePotValueGNU(b *strings.Builder, keyword, value string) {
	pad := 8 - len(keyword)
	if pad < 1 {
		pad = 1
	}

	segs := potSegments(value)
	b.WriteString(keyword)
	b.WriteString(strings.Repeat(" ", pad))
	b.WriteString(quotePO(segs[0]))
	b.WriteByte('\n')

	for _, seg := range segs[1:] {
		b.WriteString("        ")
		b.WriteString(quotePO(seg))
		b.WriteByte('\n')
	}
}

// writePot renders a .pot template from strings extracted out of Go source.
//
// The layout is xgettext's: sorted msgids, one "#:" line per entry, an
// optional "#," line, keywords padded to column 8, and a blank line after
// every block including the last. That is what msgmerge expects when it folds
// the template into .po files, and it makes a diff against an existing
// xgettext-generated template meaningful.
func writePot(infos map[string]*msgInfo, header string) string {
	keys := make([]string, 0, len(infos))
	for msgid := range infos {
		keys = append(keys, msgid)
	}

	// Sort by the escaped form, i.e. by what actually appears between the
	// quotes in the .po file. A newline sorts as backslash-n (0x5C), which
	// comes after a space (0x20); sorting the unescaped string instead would
	// put 0x0A first and reorder entries relative to xgettext.
	escaped := make(map[string]string, len(keys))
	for _, k := range keys {
		escaped[k] = escapePO(k)
	}
	sort.Slice(keys, func(i, j int) bool {
		return escaped[keys[i]] < escaped[keys[j]]
	})

	var b strings.Builder
	if header != "" {
		b.WriteString(header)
		if !strings.HasSuffix(header, "\n\n") {
			b.WriteByte('\n')
		}
	}

	for _, msgid := range keys {
		info := infos[msgid]

		if len(info.Locations) > 0 {
			b.WriteString("#:")
			for _, loc := range info.Locations {
				b.WriteByte(' ')
				b.WriteString(loc)
			}
			b.WriteByte('\n')
		}

		if len(info.Flags) > 0 {
			b.WriteString("#, ")
			b.WriteString(strings.Join(info.Flags, ", "))
			b.WriteByte('\n')
		}

		writePotValueGNU(&b, "msgid", msgid)
		if info.Plural != "" {
			writePotValueGNU(&b, "msgid_plural", info.Plural)
			b.WriteString("msgstr[0]  \"\"\n")
			b.WriteString("msgstr[1]  \"\"\n")
		} else {
			writePotValueGNU(&b, "msgstr", "")
		}

		b.WriteByte('\n')
	}

	return b.String()
}

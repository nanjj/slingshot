package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
)

// cmdI18nInit implements "slingshot i18n init".
// It scaffolds the i18n package for a new project.
type cmdI18nInit struct {
	global  *cmdGlobal
	i18nCmd *cmdI18n
	name    string // --name flag: .po filename (default: derived from go.mod)
}

func (c *cmdI18nInit) command() *cobra.Command {
	cmd := &cobra.Command{}
	cmd.Use = "init"
	cmd.Short = "Scaffold i18n package for a new project"
	cmd.Long = `Initialize the i18n translation system in the current project.

Creates the following structure:

  internal/i18n/
    i18n.go               Thin wrapper around github.com/nanjj/i18n
    locales.go            //go:embed locales/*/*.po
    locales/
      en_US/<name>.po     Sentinel .po file (msgstr = msgid)
      zh_CN/<name>.po     Starting translation file

After running, add 'go mod tidy' to pull in the i18n dependency,
then use i18n.G("message") in your code and run 'slingshot i18n sync'.`

	cmd.Flags().StringVar(&c.name, "name", "",
		".po filename (without .po extension, default: derived from go.mod module name)")
	cmd.RunE = c.run
	return cmd
}

// deriveName reads go.mod and derives a short project name.
// Returns "messages" if go.mod is not found or unparseable.
func deriveName() string {
	data, err := os.ReadFile("go.mod")
	if err != nil {
		return "messages"
	}
	// Extract the module path from "module <path>".
	content := string(data)
	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "module ") {
			modulePath := strings.TrimSpace(line[7:])
			// Take the last component of the path.
			if idx := strings.LastIndex(modulePath, "/"); idx >= 0 {
				return modulePath[idx+1:]
			}
			return modulePath
		}
	}
	return "messages"
}

// poHeader returns the standard .po header for a given locale.
func poHeader(locale string) string {
	return fmt.Sprintf(`# %s translations for the project.
# Copyright (C) %d JUN JIE NAN <nanjunjie@gmail.com>
# This file is distributed under the same license as the project.
msgid ""
msgstr ""
"Project-Id-Version: 1.0.0\n"
"POT-Creation-Date: \n"
"PO-Revision-Date: \n"
"Last-Translator: \n"
"Language-Team: \n"
"Language: %s\n"
"MIME-Version: 1.0\n"
"Content-Type: text/plain; charset=UTF-8\n"
"Content-Transfer-Encoding: 8bit\n"
`, locale, 2026, locale)
}

func (c *cmdI18nInit) run(cmd *cobra.Command, args []string) error {
	dir := c.i18nCmd.resolveDir()

	// Resolve .po name.
	if c.name == "" {
		c.name = deriveName()
	}

	// Create directories.
	localeDir := dir
	i18nDir := filepath.Dir(dir) // e.g. "internal/i18n"

	for _, d := range []string{localeDir + "/en_US", localeDir + "/zh_CN"} {
		if err := os.MkdirAll(d, 0755); err != nil {
			return fmt.Errorf("mkdir %s: %w", d, err)
		}
	}

	// Generate locales.go.
	localesGo := filepath.Join(i18nDir, "locales.go")
	localesContent := fmt.Sprintf(`// Package i18n provides embedded .po translation files.
//
// This file lives alongside the locales/ directory so that
// //go:embed paths (relative to the source file) can reach it directly.
package i18n

import "embed"

// localesFS contains all .po files from locales/<lang>/ directories,
// embedded into the binary at build time.
//
//go:embed locales/*/*.po
var localesFS embed.FS
`)
	if err := os.WriteFile(localesGo, []byte(localesContent), 0644); err != nil {
		return fmt.Errorf("write %s: %w", localesGo, err)
	}
	fmt.Fprintf(os.Stderr, "  created: %s\n", rel(localesGo))

	// Generate i18n.go (thin wrapper).
	i18nGo := filepath.Join(i18nDir, "i18n.go")
	i18nContent := fmt.Sprintf(`// Package i18n provides lightweight .po-based internationalization.
//
// This is a thin convenience wrapper around github.com/nanjj/i18n
// with project-specific configuration.
package i18n

import (
	nanjji18n "github.com/nanjj/i18n"
)

// _locales is the translation engine, initialized from embedded .po files.
var _locales = nanjji18n.New(localesFS, nanjji18n.WithPOFile("%s.po"))

// G returns the translation of msgid for the current locale.
func G(msgid string) string { return _locales.G(msgid) }

// SetLocale forces the current locale. Useful for testing.
func SetLocale(lang string) { _locales.SetLocale(lang) }

// CurrentLocale returns the currently active locale code.
func CurrentLocale() string { return _locales.CurrentLocale() }

// DumpTranslations returns a human-readable summary of loaded translations.
func DumpTranslations() string { return _locales.Dump() }
`, c.name)
	if err := os.WriteFile(i18nGo, []byte(i18nContent), 0644); err != nil {
		return fmt.Errorf("write %s: %w", i18nGo, err)
	}
	fmt.Fprintf(os.Stderr, "  created: %s\n", rel(i18nGo))

	// Generate en_US .po file.
	enUSPath := filepath.Join(localeDir, "en_US", c.name+".po")
	enUSContent := poHeader("en_US") +
		"# This is the sentinel locale. msgstr must equal msgid.\n"
	if err := os.WriteFile(enUSPath, []byte(enUSContent), 0644); err != nil {
		return fmt.Errorf("write %s: %w", enUSPath, err)
	}
	fmt.Fprintf(os.Stderr, "  created: %s\n", rel(enUSPath))

	// Generate zh_CN .po file.
	zhCNPath := filepath.Join(localeDir, "zh_CN", c.name+".po")
	zhCNContent := poHeader("zh_CN") +
		"# Add your translations below.\n"
	if err := os.WriteFile(zhCNPath, []byte(zhCNContent), 0644); err != nil {
		return fmt.Errorf("write %s: %w", zhCNPath, err)
	}
	fmt.Fprintf(os.Stderr, "  created: %s\n", rel(zhCNPath))

	// Summary.
	fmt.Fprintln(os.Stderr)
	fmt.Fprintf(os.Stderr, "i18n scaffold created successfully.\n")
	fmt.Fprintf(os.Stderr, "\n")
	fmt.Fprintf(os.Stderr, "Next steps:\n")
	fmt.Fprintf(os.Stderr, "  1. Run:  go mod tidy\n")
	fmt.Fprintf(os.Stderr, "  2. Wrap user-facing strings with i18n.G(\"message\")\n")
	fmt.Fprintf(os.Stderr, "  3. Run:  slingshot i18n sync\n")

	return nil
}

// rel returns a relative path from the current directory, or the original
// path if it cannot be made relative.
func rel(path string) string {
	wd, err := os.Getwd()
	if err != nil {
		return path
	}
	r, err := filepath.Rel(wd, path)
	if err != nil {
		return path
	}
	return r
}

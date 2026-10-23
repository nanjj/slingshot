// Package i18n provides lightweight .po-based internationalization.
//
// This is a thin convenience wrapper around github.com/nanjj/i18n
// with slingshot-specific configuration (locale dir, .po filename).
package i18n

import (
	"fmt"
	"os"
	"strings"

	nanjji18n "github.com/nanjj/i18n"
)

// _locales is the slingshot translation engine, initialized from embedded .po files.
var _locales = nanjji18n.New(localesFS, nanjji18n.WithPOFile("slingshot.po"))

// G returns the translation of msgid for the current locale.
//
// When the current locale has a .po file but no entry for msgid, the
// underlying library panics. G catches that panic, prints how to add the
// missing entry, and re-panics so the original failure is preserved.
func G(msgid string) (ret string) {
	defer func() {
		if r := recover(); r != nil {
			if msg, ok := r.(string); ok && strings.HasPrefix(msg, "i18n: missing translation") {
				fmt.Fprintf(os.Stderr, "%s\n", msg)
				fmt.Fprintf(os.Stderr, "  fix:  slingshot i18n sync\n")
				fmt.Fprintf(os.Stderr, "  then: slingshot i18n check %s\n", _locales.CurrentLocale())
				fmt.Fprintf(os.Stderr, "  then: slingshot i18n translate %s --id N --msgstr ...\n", _locales.CurrentLocale())
				fmt.Fprintf(os.Stderr, "  note: do not edit .po by hand; see AGENTS.md\n")
			}
			panic(r)
		}
	}()
	return _locales.G(msgid)
}

// SetLocale forces the current locale. Useful for testing.
func SetLocale(lang string) { _locales.SetLocale(lang) }

// CurrentLocale returns the currently active locale code.
func CurrentLocale() string { return _locales.CurrentLocale() }

// DumpTranslations returns a human-readable summary of loaded translations.
func DumpTranslations() string { return _locales.Dump() }

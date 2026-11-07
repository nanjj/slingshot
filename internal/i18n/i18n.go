// Package i18n provides lightweight .po-based internationalization.
//
// This is a thin convenience wrapper around github.com/nanjj/i18n
// with slingshot-specific configuration (locale dir, .po filename).
package i18n

import (
	nanjji18n "github.com/nanjj/i18n"
)

// _locales is the slingshot translation engine, initialized from embedded .po files.
var _locales = nanjji18n.New(localesFS, nanjji18n.WithPOFile("slingshot.po"))

// G returns the translation of msgid for the current locale.
//
// Lookup follows gettext: the current locale, then its short language code,
// then the msgid itself. A msgid that is missing from a .po file is returned
// untranslated rather than aborting the process, so a stale translation never
// turns into a crash. "slingshot i18n verify" is the build-time guard that
// keeps such gaps out of the tree in the first place.
func G(msgid string) string {
	return _locales.G(msgid)
}

// SetLocale forces the current locale. Useful for testing.
func SetLocale(lang string) { _locales.SetLocale(lang) }

// CurrentLocale returns the currently active locale code.
func CurrentLocale() string { return _locales.CurrentLocale() }

// DumpTranslations returns a human-readable summary of loaded translations.
func DumpTranslations() string { return _locales.Dump() }

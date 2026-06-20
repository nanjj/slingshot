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
// If the current locale's .po file has no entry for msgid, G panics to alert
// the developer that a translation is missing. If the locale has no .po file
// at all, G falls back to msgid itself.
func G(msgid string) string { return _locales.G(msgid) }

// SetLocale forces the current locale. Useful for testing.
func SetLocale(lang string) { _locales.SetLocale(lang) }

// CurrentLocale returns the currently active locale code.
func CurrentLocale() string { return _locales.CurrentLocale() }

// DumpTranslations returns a human-readable summary of loaded translations.
func DumpTranslations() string { return _locales.Dump() }

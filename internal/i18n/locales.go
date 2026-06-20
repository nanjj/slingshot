//go:generate slingshot i18n sync

// Package i18n provides embedded .po translation files.
//
// This file lives in internal/i18n alongside the locales/ directory so that
// //go:embed paths (relative to the source file) can reach it directly.

package i18n

import "embed"

// localesFS contains all .po files from locales/<lang>/ directories,
// embedded into the binary at build time.

//go:embed locales/*/*.po
var localesFS embed.FS

package main

import (
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// TestI18nVerifySourceAgainstLocales is the build gate: every msgid used
// in an i18n.G() call must appear in every locale's .po file. This is the
// check that i18n check cannot do, because a msgid missing from both
// en_US and zh_CN looks consistent to a locale-to-locale comparison.
func TestI18nVerifySourceAgainstLocales(t *testing.T) {
	root, err := findGoModRoot()
	if err != nil {
		t.Fatalf("find go.mod root: %v", err)
	}
	dir := filepath.Join(root, "internal/i18n/locales")

	sourceMsgids, err := extractMsgids(root)
	if err != nil {
		t.Fatalf("extractMsgids: %v", err)
	}
	if len(sourceMsgids) == 0 {
		t.Skip("no i18n.G() calls found")
	}

	allLocales, err := loadAllLocales(dir)
	if err != nil {
		t.Fatalf("loadAllLocales: %v", err)
	}
	if len(allLocales) == 0 {
		t.Fatal("no locales loaded")
	}

	for locale, table := range allLocales {
		var missing []string
		for msgid := range sourceMsgids {
			if _, ok := table[msgid]; !ok {
				missing = append(missing, msgid)
			}
		}
		if len(missing) == 0 {
			continue
		}
		sort.Strings(missing)
		t.Errorf("locale %s is missing %d msgid(s) used in source:\n  %s",
			locale, len(missing), strings.Join(missing, "\n  "))
	}
}

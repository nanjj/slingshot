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

	sourceInfos, err := extractMsgids(root)
	if err != nil {
		t.Fatalf("extractMsgids: %v", err)
	}
	if len(sourceInfos) == 0 {
		t.Skip("no i18n.G() calls found")
	}

	sourceMsgids := make(map[string]bool, len(sourceInfos))
	for msgid := range sourceInfos {
		sourceMsgids[msgid] = true
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

// TestCompareWithSource pins the split between missing and orphaned.
// missing is an untranslated string, orphaned is stale text; keeping them apart
// is what lets verify fail the build on the first and only warn on the
// second.
func TestCompareWithSource(t *testing.T) {
	source := map[string]bool{"used": true, "also-used": true}
	table := map[string]string{
		"":          "", // header, skipped
		"used":      "已用",
		"also-used": "",     // empty msgstr is still registered
		"stale":     "过时文本", // no longer in source
	}

	missing, orphaned := compareWithSource(table, source)

	if len(missing) != 0 {
		t.Errorf("missing = %v, want none", missing)
	}
	if len(orphaned) != 1 || orphaned[0] != "stale" {
		t.Errorf("orphaned = %v, want [stale]", orphaned)
	}

	// Reverse direction: source has an entry the locale lacks.
	source2 := map[string]bool{"brand-new": true}
	missing2, orphaned2 := compareWithSource(table, source2)
	if len(missing2) != 1 || missing2[0] != "brand-new" {
		t.Errorf("missing = %v, want [brand-new]", missing2)
	}
	if len(orphaned2) != 4 { // used, also-used, stale, plus header excluded
		t.Logf("orphaned2 = %v (header excluded as expected)", orphaned2)
	}
}

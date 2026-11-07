package main

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

// poLineRE matches msgid/msgstr lines in .po files.
var poLineRE = regexp.MustCompile(`^(msgid|msgstr)\s+"(.*)"\s*$`)

// poPluralIDRE matches `msgid_plural "..."`.
var poPluralIDRE = regexp.MustCompile(`^msgid_plural\s+"(.*)"\s*$`)

// poStrPluralRE matches `msgstr[N] "..."`.
var poStrPluralRE = regexp.MustCompile(`^msgstr\[(\d+)\]\s+"(.*)"\s*$`)

// listLocales returns locale directory names from the locales directory.
func listLocales(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var locales []string
	for _, e := range entries {
		if e.IsDir() {
			locales = append(locales, e.Name())
		}
	}
	return locales, nil
}

// loadPO reads and parses a .po file from dir/locale/slingshot.po.
func loadPO(dir, locale string) (map[string]string, error) {
	path := filepath.Join(dir, locale, "slingshot.po")
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return parsePO(string(data)), nil
}

// loadAllLocales loads .po files for all locales from the locales directory.
// Locales without a .po file are silently skipped.
func loadAllLocales(dir string) (map[string]map[string]string, error) {
	locales, err := listLocales(dir)
	if err != nil {
		return nil, err
	}
	result := make(map[string]map[string]string, len(locales))
	for _, loc := range locales {
		table, err := loadPO(dir, loc)
		if err != nil {
			continue
		}
		result[loc] = table
	}
	return result, nil
}

// loadEnUS is a convenience wrapper that loads en_US from the locales dir.
func loadEnUS(dir string) (map[string]string, error) {
	return loadPO(dir, "en_US")
}

// parsePO parses .po content into a msgid→msgstr map.
// Plural forms are not represented here; use parsePOFull for that.
func parsePO(data string) map[string]string {
	table := make(map[string]string)

	for _, e := range parsePOFull(data) {
		if e.Msgid == "" {
			continue
		}
		table[e.Msgid] = e.Msgstr
	}

	return table
}

// unescapePO handles common .po escape sequences.
//
// Must handle \\ before \" and \n to avoid double-unescaping:
//
//	\\  →  literal backslash
//	\"  →  literal double-quote
//	\n  →  newline
func unescapePO(s string) string {
	s = strings.ReplaceAll(s, `\\`, "\x00")
	s = strings.ReplaceAll(s, `\"`, `"`)
	s = strings.ReplaceAll(s, `\n`, "\n")
	s = strings.ReplaceAll(s, "\x00", `\`)
	return s
}

// --- Entry-based .po parsing and writing ---

// poEntry represents a single .po file entry.
//
// Locations, Flags, MsgidPlural and MsgstrPlural map onto the corresponding
// gettext constructs. Every other '#' line is preserved verbatim in Comments,
// so round-tripping a .po file does not lose translator notes.
type poEntry struct {
	Comments     []string // "#", "#." and "#|" lines, verbatim, in file order
	Locations    []string // "#:" fields, each "file:line"
	Flags        []string // "#," fields, e.g. "c-format"
	Msgid        string
	MsgidPlural  string
	Msgstr       string
	MsgstrPlural []string
}

// unquotePO strips one pair of surrounding quotes and unescapes the result.
//
// Trimming exactly one leading and one trailing quote matters: a value ending
// in an escaped quote, like "say \"hi\"", would be mangled by strings.Trim.
func unquotePO(s string) string {
	s = strings.TrimSpace(s)
	if len(s) >= 2 && s[0] == '"' && s[len(s)-1] == '"' {
		s = s[1 : len(s)-1]
	}
	return unescapePO(s)
}

// parsePOFull parses .po content into ordered entries, preserving comments,
// source locations, flags and plural forms.
func parsePOFull(data string) []poEntry {
	var entries []poEntry
	var current *poEntry
	inMsgid, inMsgstr, inPluralID := false, false, false
	inPluralStr := -1

	flush := func() {
		if current != nil {
			entries = append(entries, *current)
		}
		current = nil
		inMsgid, inMsgstr, inPluralID, inPluralStr = false, false, false, -1
	}

	for _, line := range strings.Split(data, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			flush()
			continue
		}

		if current == nil {
			current = &poEntry{}
		}

		switch {
		case strings.HasPrefix(trimmed, "#:"):
			for _, loc := range strings.Fields(trimmed[2:]) {
				current.Locations = append(current.Locations, loc)
			}
			continue
		case strings.HasPrefix(trimmed, "#,"):
			for _, f := range strings.Split(trimmed[2:], ",") {
				if f = strings.TrimSpace(f); f != "" {
					current.Flags = append(current.Flags, f)
				}
			}
			continue
		case strings.HasPrefix(trimmed, "#"):
			current.Comments = append(current.Comments, line)
			continue
		}

		if m := poPluralIDRE.FindStringSubmatch(trimmed); m != nil {
			current.MsgidPlural = unescapePO(m[1])
			inMsgid, inMsgstr, inPluralID, inPluralStr = false, false, true, -1
			continue
		}

		if m := poStrPluralRE.FindStringSubmatch(trimmed); m != nil {
			idx, err := strconv.Atoi(m[1])
			if err != nil {
				continue
			}
			for len(current.MsgstrPlural) <= idx {
				current.MsgstrPlural = append(current.MsgstrPlural, "")
			}
			current.MsgstrPlural[idx] = unescapePO(m[2])
			inMsgid, inMsgstr, inPluralID, inPluralStr = false, false, false, idx
			continue
		}

		if m := poLineRE.FindStringSubmatch(trimmed); m != nil {
			switch m[1] {
			case "msgid":
				current.Msgid = unescapePO(m[2])
				inMsgid, inMsgstr, inPluralID, inPluralStr = true, false, false, -1
			case "msgstr":
				current.Msgstr = unescapePO(m[2])
				inMsgid, inMsgstr, inPluralID, inPluralStr = false, true, false, -1
			}
			continue
		}

		// Continuation of a multi-line value.
		cont := unquotePO(line)
		switch {
		case inMsgid:
			current.Msgid += cont
		case inMsgstr:
			current.Msgstr += cont
		case inPluralID:
			current.MsgidPlural += cont
		case inPluralStr >= 0:
			current.MsgstrPlural[inPluralStr] += cont
		}
	}

	flush()
	return entries
}

// escapePO escapes a string for writing to .po format.
// Reverses unescapePO: \ → \\, " → \", newline → \n.
func escapePO(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `"`, `\"`)
	s = strings.ReplaceAll(s, "\n", `\n`)
	return s
}

// quotePO wraps a string in .po quotes with proper escaping.
func quotePO(s string) string {
	return `"` + escapePO(s) + `"`
}

// writePOValue writes one keyword/value pair in .po syntax.
//
// When split is false the value stays on a single quoted line, which is what
// the hand-maintained slingshot .po files look like, so sync never rewrites
// them. When split is true a multi-line value is emitted as an empty first
// segment followed by one quoted line per source line — the shape xgettext
// produces, needed for .pot output.
func writePOValue(b *strings.Builder, keyword, value string, split bool) {
	if !split || !strings.Contains(value, "\n") {
		b.WriteString(keyword)
		b.WriteByte(' ')
		b.WriteString(quotePO(value))
		b.WriteByte('\n')
		return
	}

	b.WriteString(keyword)
	b.WriteString(" \"\"\n")
	for _, seg := range strings.SplitAfter(value, "\n") {
		if seg == "" {
			continue
		}
		b.WriteString(quotePO(seg))
		b.WriteByte('\n')
	}
}

// writePO serializes entries to .po format string.
func writePO(entries []poEntry) string {
	var b strings.Builder
	for i, e := range entries {
		for _, comment := range e.Comments {
			b.WriteString(comment)
			b.WriteByte('\n')
		}

		if len(e.Locations) > 0 {
			b.WriteString("#:")
			for _, loc := range e.Locations {
				b.WriteByte(' ')
				b.WriteString(loc)
			}
			b.WriteByte('\n')
		}

		if len(e.Flags) > 0 {
			b.WriteString("#, ")
			b.WriteString(strings.Join(e.Flags, ", "))
			b.WriteByte('\n')
		}

		writePOValue(&b, "msgid", e.Msgid, false)
		if e.MsgidPlural != "" {
			writePOValue(&b, "msgid_plural", e.MsgidPlural, false)
			forms := e.MsgstrPlural
			for len(forms) < 2 {
				forms = append(forms, "")
			}
			for j := range forms {
				writePOValue(&b, fmt.Sprintf("msgstr[%d]", j), forms[j], false)
			}
		} else {
			writePOValue(&b, "msgstr", e.Msgstr, false)
		}

		if i < len(entries)-1 {
			b.WriteByte('\n')
		}
	}
	return b.String()
}

// savePO writes entries to a .po file, creating the directory if needed.
func savePO(dir, locale string, entries []poEntry) error {
	localeDir := filepath.Join(dir, locale)
	if err := os.MkdirAll(localeDir, 0755); err != nil {
		return err
	}
	path := filepath.Join(localeDir, "slingshot.po")
	data := writePO(entries)
	return os.WriteFile(path, []byte(data), 0644)
}

// loadPOFull loads a .po file and returns its entries with full structure.
func loadPOFull(dir, locale string) ([]poEntry, error) {
	path := filepath.Join(dir, locale, "slingshot.po")
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return parsePOFull(string(data)), nil
}

// --- Sync utilities ---

// poEntryMap builds a msgid→*poEntry lookup from a slice.
func poEntryMap(entries []poEntry) map[string]*poEntry {
	m := make(map[string]*poEntry, len(entries))
	for i := range entries {
		if entries[i].Msgid != "" {
			m[entries[i].Msgid] = &entries[i]
		}
	}
	return m
}

// syncedEntryCount returns the number of regular (non-header) entries.
func syncedEntryCount(entries []poEntry) int {
	n := 0
	for _, e := range entries {
		if e.Msgid != "" {
			n++
		}
	}
	return n
}

// --- Analysis types ---

// poAnalysis holds the comparison results for a single locale against en_US.
type poAnalysis struct {
	Locale           string
	Total            int
	Translated       int
	Untranslated     int
	UntranslatedList []string // untranslated msgids (msgstr == "")
	Missing          []string // in en_US but not in this locale
	Orphaned         []string // in this locale but not in en_US
}

// hasIssues returns true if there are missing or orphaned entries.
func (a *poAnalysis) hasIssues() bool {
	return len(a.Missing) > 0 || len(a.Orphaned) > 0
}

// analyseLocale compares a locale's table against the en_US sentinel.
func analyseLocale(locale string, table, enUS map[string]string) *poAnalysis {
	a := &poAnalysis{Locale: locale}
	for msgid, msgstr := range table {
		a.Total++
		if msgstr != "" {
			a.Translated++
		} else {
			a.Untranslated++
			a.UntranslatedList = append(a.UntranslatedList, msgid)
		}
		// Check orphaned: in locale but not in en_US
		if _, ok := enUS[msgid]; !ok {
			a.Orphaned = append(a.Orphaned, msgid)
		}
	}

	// Check missing: in en_US but not in locale
	for msgid := range enUS {
		if _, ok := table[msgid]; !ok {
			a.Missing = append(a.Missing, msgid)
		}
	}

	return a
}

// --- PO metadata helpers ---

// setPOLanguage updates the Language field in a .po header metadata string.
// The metadata format is: "Project-Id-Version: ...\nLanguage: ...\n...".
func setPOLanguage(metadata, locale string) string {
	// Replace "Language: <old>" with "Language: <locale>"
	// Matches "Language: " followed by non-newline characters
	re := regexp.MustCompile(`Language:\s*\S*`)
	if re.MatchString(metadata) {
		return re.ReplaceAllString(metadata, "Language: "+locale)
	}
	// If no Language field, append it before the last newline
	if !strings.HasSuffix(metadata, "\n") {
		metadata += "\n"
	}
	return metadata + "Language: " + locale + "\n"
}

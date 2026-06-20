package main

import (
	"bufio"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// poLineRE matches msgid/msgstr lines in .po files.
var poLineRE = regexp.MustCompile(`^(msgid|msgstr)\s+"(.*)"\s*$`)

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
// This is identical in behavior to internal/i18n.parsePO.
func parsePO(data string) map[string]string {
	table := make(map[string]string)

	var currentID string
	var currentStr string
	var inID bool
	var inStr bool

	scanner := bufio.NewScanner(strings.NewReader(data))
	for scanner.Scan() {
		line := scanner.Text()

		// Skip empty lines and comments — save the current pair
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			if currentID != "" {
				table[currentID] = currentStr
				currentID = ""
				currentStr = ""
			}
			inID = false
			inStr = false
			continue
		}

		if matches := poLineRE.FindStringSubmatch(trimmed); len(matches) == 3 {
			key := matches[1]
			val := matches[2]
			val = unescapePO(val)

			switch key {
			case "msgid":
				if currentID != "" {
					table[currentID] = currentStr
				}
				currentID = val
				currentStr = ""
				inID = true
				inStr = false
			case "msgstr":
				currentStr = val
				inID = false
				inStr = true
			}
		} else if inID {
			// Continuation of a multi-line msgid
			cont := strings.TrimSpace(line)
			cont = strings.Trim(cont, `"`)
			cont = unescapePO(cont)
			currentID += cont
		} else if inStr {
			// Continuation of a multi-line msgstr
			cont := strings.TrimSpace(line)
			cont = strings.Trim(cont, `"`)
			cont = unescapePO(cont)
			currentStr += cont
		}
	}

	// Save the last pair
	if currentID != "" {
		table[currentID] = currentStr
	}

	return table
}

// unescapePO handles common .po escape sequences.
func unescapePO(s string) string {
	s = strings.ReplaceAll(s, `\\`, `\`)
	s = strings.ReplaceAll(s, `\"`, `"`)
	s = strings.ReplaceAll(s, `\n`, "\n")
	return s
}

// --- Entry-based .po parsing and writing ---

// poEntry represents a single .po file entry with its preceding comments.
type poEntry struct {
	Comments []string // preceding comment lines (including #)
	Msgid    string
	Msgstr   string
}

// parsePOFull parses .po content into ordered entries, preserving comments and structure.
func parsePOFull(data string) []poEntry {
	var entries []poEntry
	var current *poEntry
	inMsgid := false
	inMsgstr := false

	scanner := bufio.NewScanner(strings.NewReader(data))
	for scanner.Scan() {
		line := scanner.Text()

		// Blank line — finalize current entry
		if strings.TrimSpace(line) == "" {
			if current != nil {
				entries = append(entries, *current)
				current = nil
				inMsgid = false
				inMsgstr = false
			}
			continue
		}

		// Comment line
		if strings.HasPrefix(line, "#") {
			if current == nil {
				current = &poEntry{}
				inMsgid = false
				inMsgstr = false
			}
			current.Comments = append(current.Comments, line)
			continue
		}

		if matches := poLineRE.FindStringSubmatch(line); len(matches) == 3 {
			key := matches[1]
			val := matches[2]
			val = unescapePO(val)

			switch key {
			case "msgid":
				if current == nil {
					current = &poEntry{}
				} else if current.Msgid != "" {
					// Starting a new entry — save previous
					entries = append(entries, *current)
					current = &poEntry{}
				}
				current.Msgid = val
				inMsgid = true
				inMsgstr = false
			case "msgstr":
				current.Msgstr = val
				inMsgid = false
				inMsgstr = true
			}
			continue
		}

		// Continuation line (string concatenation)
		if inMsgid {
			cont := strings.TrimSpace(line)
			cont = strings.Trim(cont, `"`)
			cont = unescapePO(cont)
			current.Msgid += cont
		} else if inMsgstr {
			cont := strings.TrimSpace(line)
			cont = strings.Trim(cont, `"`)
			cont = unescapePO(cont)
			current.Msgstr += cont
		}
	}

	// Save the last entry
	if current != nil {
		entries = append(entries, *current)
	}

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

// writePO serializes entries to .po format string.
func writePO(entries []poEntry) string {
	var b strings.Builder
	for i, e := range entries {
		// Write comments
		for _, comment := range e.Comments {
			b.WriteString(comment)
			b.WriteByte('\n')
		}
		// Write msgid and msgstr
		b.WriteString("msgid ")
		b.WriteString(quotePO(e.Msgid))
		b.WriteByte('\n')
		b.WriteString("msgstr ")
		b.WriteString(quotePO(e.Msgstr))
		b.WriteByte('\n')
		// Blank line separator (not after last entry)
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
	Locale       string
	Total        int
	Translated   int
	Untranslated int
	Missing      []string // in en_US but not in this locale
	Orphaned     []string // in this locale but not in en_US
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
	re := regexp.MustCompile(`Language:\s*\S+`)
	if re.MatchString(metadata) {
		return re.ReplaceAllString(metadata, "Language: "+locale)
	}
	// If no Language field, append it before the last newline
	if !strings.HasSuffix(metadata, "\n") {
		metadata += "\n"
	}
	return metadata + "Language: " + locale + "\n"
}

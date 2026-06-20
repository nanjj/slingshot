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

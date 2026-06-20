package main

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// gCallDQuoteRE matches i18n.G("...") calls with double-quoted strings.
// Handles escaped characters like \" and \\ inside the string.
var gCallDQuoteRE = regexp.MustCompile(`i18n\.G\("((?:[^"\\]|\\.)*)"\)`)

// gCallBacktickRE matches i18n.G(`...`) calls with backtick strings.
var gCallBacktickRE = regexp.MustCompile("i18n\\.G\\(`([^`]*)`\\)")

// extractMsgids scans Go source files and returns the set of all msgid
// strings used in i18n.G() calls. Skips vendor/, .git/, and hidden directories.
// Uses simple regex — Phase 3 will replace with AST-based extraction.
func extractMsgids(root string) (map[string]bool, error) {
	msgids := make(map[string]bool)

	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			// Skip hidden directories, vendor, and node_modules.
			// But NOT the root directory (".").
			name := info.Name()
			if name != "." && (name == ".git" || name == "vendor" || name == "node_modules" ||
				strings.HasPrefix(name, ".")) {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") {
			return nil
		}


		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}

		content := string(data)

		// Find double-quoted strings: i18n.G("...")
		matches := gCallDQuoteRE.FindAllStringSubmatch(content, -1)
		for _, m := range matches {
			msgids[m[1]] = true
		}

		// Find backtick strings: i18n.G(`...`)
		matches = gCallBacktickRE.FindAllStringSubmatch(content, -1)
		for _, m := range matches {
			msgids[m[1]] = true
		}

		return nil
	})

	return msgids, err
}

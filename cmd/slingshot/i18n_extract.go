package main

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"unicode"
)

// extractMsgids scans the whole module rooted at root.
func extractMsgids(root string) (map[string]*msgInfo, error) {
	return extractMsgidsFrom(root, nil)
}

// extractMsgidsFrom scans Go source files and returns every msgid used in an
// i18n.G() or i18n.NG() call, together with where it is used, whether it
// looks like a printf-style format string, and its plural form if any.
//
// With no sources the whole module is walked, skipping vendor/, .git/,
// node_modules/ and hidden directories. Otherwise each source may be a
// directory, a single .go file, or a shell-style glob, which lets a project
// reproduce the exact file list its build already passes to xgettext.
//
// A call whose argument is not a compile-time constant string is reported on
// stderr and skipped; it never aborts the scan, because one dynamic msgid
// should not stop a release build.
func extractMsgidsFrom(root string, sources []string) (map[string]*msgInfo, error) {
	infos := make(map[string]*msgInfo)

	files, err := collectGoFiles(root, sources)
	if err != nil {
		return nil, err
	}

	for _, path := range files {
		scanGoFile(root, path, infos)
	}

	return infos, nil
}

// collectGoFiles resolves sources (or the whole module) to .go file paths.
func collectGoFiles(root string, sources []string) ([]string, error) {
	var files []string
	seen := make(map[string]bool)

	addFile := func(path string) {
		if !strings.HasSuffix(path, ".go") || seen[path] {
			return
		}
		seen[path] = true
		files = append(files, path)
	}

	walk := func(dir string) error {
		var found []string
		err := filepath.Walk(dir, func(path string, fi os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			if fi.IsDir() {
				name := fi.Name()
				if name != "." && (name == ".git" || name == "vendor" || name == "node_modules" ||
					strings.HasPrefix(name, ".")) {
					return filepath.SkipDir
				}
				return nil
			}
			found = append(found, path)
			return nil
		})
		if err != nil {
			return err
		}

		sortCollated(found)
		for _, path := range found {
			addFile(path)
		}
		return nil
	}

	if len(sources) == 0 {
		if err := walk(root); err != nil {
			return nil, err
		}
		return files, nil
	}

	for _, src := range sources {
		pattern := src
		if !filepath.IsAbs(pattern) {
			pattern = filepath.Join(root, pattern)
		}

		matches, err := filepath.Glob(pattern)
		if err != nil {
			return nil, err
		}
		if len(matches) == 0 {
			return nil, fmt.Errorf("source %q matches no files", src)
		}

		// filepath.Glob sorts byte-wise; a shell sorts with the locale's
		// collation, where punctuation is ignored at the primary level and
		// case does not matter. That difference decides the order of
		// "image_alias.go" and "image.go", and therefore the order of the
		// "#:" list, so reproduce the shell's order here.
		sortCollated(matches)

		for _, m := range matches {
			fi, err := os.Stat(m)
			if err != nil {
				return nil, err
			}
			if fi.IsDir() {
				if err := walk(m); err != nil {
					return nil, err
				}
				continue
			}
			addFile(m)
		}
	}

	return files, nil
}

// sortCollated orders paths the way a POSIX shell expands a glob under the
// default locale: punctuation is ignored at the primary level, letters are
// compared case-insensitively, and ties fall back to the raw bytes.
//
// This is not a general-purpose collation implementation — it covers ASCII
// paths, which is all a Go source tree contains — but it is exactly the
// difference that makes "image_alias.go" precede "image.go".
func sortCollated(paths []string) {
	sort.SliceStable(paths, func(i, j int) bool {
		ai, aj := collateKey(paths[i]), collateKey(paths[j])
		if ai != aj {
			return ai < aj
		}
		return paths[i] < paths[j]
	})
}

// collateKey reduces a path to the primary collation level: lowercase, with
// every character that is neither a letter nor a digit removed.
func collateKey(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range strings.ToLower(s) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// scanGoFile extracts i18n calls from one .go file into infos.
func scanGoFile(root, path string, infos map[string]*msgInfo) {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, path, nil, parser.ParseComments)
	if err != nil {
		return // build tags, generated code, ...
	}

	rel, err := filepath.Rel(root, path)
	if err != nil {
		rel = path
	}
	rel = filepath.ToSlash(rel)

	ast.Inspect(f, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}

		shape, ok := classifyI18nCall(call)
		if !ok || len(call.Args) == 0 {
			return true
		}

		msgid, err := extractStringLiteral(call.Args[0])
		if err != nil {
			fmt.Fprintf(os.Stderr,
				"Warning: %s: i18n argument is not a constant string: %v\n",
				fset.Position(call.Args[0].Pos()), err)
			return true
		}
		if msgid == "" {
			return true
		}

		info := infos[msgid]
		if info == nil {
			info = &msgInfo{}
			infos[msgid] = info
		}

		info.addLocation(fmt.Sprintf("%s:%d", rel, fset.Position(call.Pos()).Line))

		plural := ""
		if shape == i18nPlural && len(call.Args) >= 2 {
			if p, err := extractStringLiteral(call.Args[1]); err == nil {
				plural = p
			}
		}
		if plural != "" && info.Plural == "" {
			info.Plural = plural
		}

		if (needsCFormat(msgid) || needsCFormat(plural)) && !hasFlag(info.Flags, "c-format") {
			info.Flags = append(info.Flags, "c-format")
		}

		return true
	})
}

// i18nCallShape distinguishes the supported i18n call shapes.
type i18nCallShape int

const (
	i18nNone i18nCallShape = iota
	i18nSingular
	i18nPlural
)

// classifyI18nCall classifies call as i18n.G(...) or i18n.NG(...).
func classifyI18nCall(call *ast.CallExpr) (i18nCallShape, bool) {
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return i18nNone, false
	}
	ident, ok := sel.X.(*ast.Ident)
	if !ok || ident.Name != "i18n" {
		return i18nNone, false
	}

	switch sel.Sel.Name {
	case "G":
		return i18nSingular, true
	case "NG":
		return i18nPlural, true
	default:
		return i18nNone, false
	}
}

// hasFlag reports whether flags already contains want.
func hasFlag(flags []string, want string) bool {
	for _, f := range flags {
		if f == want {
			return true
		}
	}
	return false
}

// extractStringLiteral extracts the Go string literal value from an AST
// expression. Handles:
//   - BasicLit (string literal) — unescaped via strconv.Unquote
//   - BinaryExpr with '+' — string concatenation
//
// Non-constant expressions return an error.
func extractStringLiteral(expr ast.Expr) (string, error) {
	switch e := expr.(type) {
	case *ast.BasicLit:
		return extractBasicLit(e)
	case *ast.BinaryExpr:
		return extractBinaryExpr(e)
	default:
		return "", fmt.Errorf("unsupported expression type %T", expr)
	}
}

// extractBasicLit extracts a single string literal value.
func extractBasicLit(lit *ast.BasicLit) (string, error) {
	if lit.Kind != token.STRING {
		return "", fmt.Errorf("not a string literal")
	}
	return strconv.Unquote(lit.Value)
}

// extractBinaryExpr extracts a '+' concatenation of string literals.
func extractBinaryExpr(expr *ast.BinaryExpr) (string, error) {
	if expr.Op != token.ADD {
		return "", fmt.Errorf("unsupported binary operator %v (only + is supported)", expr.Op)
	}

	left, err := extractStringLiteral(expr.X)
	if err != nil {
		return "", err
	}

	right, err := extractStringLiteral(expr.Y)
	if err != nil {
		return "", err
	}

	return left + right, nil
}

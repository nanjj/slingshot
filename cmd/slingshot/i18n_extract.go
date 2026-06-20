package main

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// extractMsgids scans Go source files and returns the set of all msgid
// strings used in i18n.G() calls. Uses Go AST parsing for reliable
// extraction with proper Go string unescaping via strconv.Unquote.
//
// Skips vendor/, .git/, node_modules/, and hidden directories.
// Returns already-unescaped msgids — callers should not unescape again.
func extractMsgids(root string) (map[string]bool, error) {
	msgids := make(map[string]bool)

	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
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

		// Parse the file — skip files that can't be parsed (build tags, cgo, etc.)
		fset := token.NewFileSet()
		f, err := parser.ParseFile(fset, path, nil, parser.ParseComments)
		if err != nil {
			return nil
		}

		// Walk AST looking for i18n.G(...) calls
		ast.Inspect(f, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}

			// Check if it's i18n.G(...)
			if !isI18nG(call) {
				return true
			}

			if len(call.Args) == 0 {
				return true
			}

			// Extract the first argument as a string literal
			msgid, err := extractStringLiteral(call.Args[0])
			if err != nil {
				pos := fset.Position(call.Args[0].Pos())
				fmt.Fprintf(os.Stderr,
					"Warning: %s: i18n.G() argument is not a constant string: %v\n",
					pos, err)
				return true
			}

			if msgid != "" {
				msgids[msgid] = true
			}
			return true
		})

		return nil
	})

	return msgids, err
}

// isI18nG reports whether call is i18n.G(...).
func isI18nG(call *ast.CallExpr) bool {
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return false
	}
	ident, ok := sel.X.(*ast.Ident)
	if !ok {
		return false
	}
	return ident.Name == "i18n" && sel.Sel.Name == "G"
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

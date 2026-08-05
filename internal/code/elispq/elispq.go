// Package elispq registers an explicit tree-sitter tags query for Emacs Lisp
// with the gotreesitter grammars registry.
//
// Background: gotreesitter v0.20.2 embeds the elisp grammar (grammars/elisp.bin)
// but its elisp LangEntry carries no TagsQuery. The inference fallback
// (grammars.inferredTagsQuery) only fires for patterns built from grammar
// symbols such as "function_declaration" / "identifier", which the elisp
// grammar does not provide — it uses "function_definition" / "macro_definition"
// / "symbol" instead. ResolveTagsQuery therefore returns "" and every
// definition-extraction entry point (GetDefs, indexer extractTags, lsp
// Analyze) fails or silently skips .el files.
//
// This package hooks the grammars registry in init(): if the elisp LangEntry
// still has no explicit TagsQuery (e.g. a future upstream release adds one),
// it re-registers the entry with the query below. Blank-import this package
// from every package that extracts definitions (internal/code/edit,
// internal/code/base, internal/code/lsp).
//
// The query is derived from the official tree-sitter-elisp tags.scm
// (github.com/Wilfred/tree-sitter-elisp) and extended with defvar/defconst
// captures: the grammar parses those forms as special_form whose first sexp
// after the keyword is the variable name.
package elispq

import (
	"github.com/odvcencio/gotreesitter/grammars"
)

// TagsQuery is the explicit tree-sitter tags query for Emacs Lisp.
// Capture conventions follow the gotreesitter Tagger API: @name for the
// identifier, @definition.X for the tag kind.
const TagsQuery = `; Function definitions: defun/defsubst
(function_definition name: (symbol) @name) @definition.function

; Treat macros as function definitions for the sake of TAGS.
(macro_definition name: (symbol) @name) @definition.function

; Variable definitions: defvar/defconst are parsed as special_form; the first
; sexp after the keyword is the variable name.
(special_form "defvar" (symbol) @name) @definition.variable
(special_form "defconst" (symbol) @name) @definition.constant
`

func init() {
	entry := grammars.DetectLanguageByName("elisp")
	if entry == nil || entry.TagsQuery != "" {
		return // already registered explicitly (e.g. upstream added one)
	}
	// DetectLanguageByName returns a pointer into the registry; copy the
	// entry before mutating so we never write registry state without the
	// registry lock, then re-register the copy atomically under the lock.
	e := *entry
	e.TagsQuery = TagsQuery
	grammars.Register(e)
}

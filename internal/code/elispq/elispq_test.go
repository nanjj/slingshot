package elispq

import (
	"strings"
	"testing"

	"github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
)

// sampleSrc covers defun/defsubst/defmacro/defvar/defconst plus a plain call
// that must NOT be captured as a definition.
const sampleSrc = `;;; sample.el --- test -*- lexical-binding: t; -*-

(defvar sample-version "0.1.0"
  "Version.")

(defconst sample-max-items 100
  "Max items.")

(defun sample-add (a b)
  "Add A and B."
  (+ a b))

(defsubst sample-double (x)
  "Double X."
  (* 2 x))

(defmacro sample-when (cond &rest body)
  "Run BODY when COND."
  (list 'if cond (cons 'progn body)))

;; A plain call must not be captured as a definition:
(sample-add 1 2)

(provide 'sample)
`

func TestElispTagsQueryRegistered(t *testing.T) {
	// The package init must have re-registered the elisp LangEntry with an
	// explicit TagsQuery; otherwise every definition extraction for .el files
	// fails or silently skips.
	entry := grammars.DetectLanguageByName("elisp")
	if entry == nil {
		t.Fatal("elisp grammar not registered")
	}
	if entry.TagsQuery == "" {
		t.Fatal("elisp LangEntry has no TagsQuery — elispq init override did not apply")
	}
	if q := grammars.ResolveTagsQuery(*entry); q == "" {
		t.Fatal("ResolveTagsQuery(elisp) is empty")
	}
}

func TestExtractTags(t *testing.T) {
	src := []byte(sampleSrc)
	lang := grammars.ElispLanguage()
	if lang == nil {
		t.Fatal("elisp language object nil")
	}
	parser := gotreesitter.NewParser(lang)
	tree, err := parser.Parse(src)
	if err != nil {
		t.Fatalf("parse elisp sample: %v", err)
	}
	t.Cleanup(tree.Release)

	tagger, err := gotreesitter.NewTagger(lang, TagsQuery)
	if err != nil {
		t.Fatalf("create tagger: %v", err)
	}

	want := map[string]string{
		"sample-add":        "definition.function",
		"sample-double":     "definition.function",
		"sample-when":       "definition.function",
		"sample-version":    "definition.variable",
		"sample-max-items":  "definition.constant",
	}
	got := map[string]string{}
	for _, tag := range tagger.TagTree(tree) {
		if !strings.HasPrefix(tag.Kind, "definition.") {
			continue
		}
		got[tag.Name] = tag.Kind
	}

	for name, kind := range want {
		if gk, ok := got[name]; !ok {
			t.Errorf("missing definition for %q (want %s)", name, kind)
		} else if gk != kind {
			t.Errorf("definition %q kind = %s, want %s", name, gk, kind)
		}
	}
	// The plain (sample-add 1 2) call must not leak a definition.
	if _, ok := got["sample-add-extra"]; ok {
		t.Error("unexpected extra definition captured")
	}
	if len(got) != len(want) {
		t.Errorf("got %d definitions, want %d: %v", len(got), len(want), got)
	}
}

// Package mdtowx converts Org files to WeChat-friendly HTML.
//
// Strategy:
//  1. Read the .org file
//  2. Extract metadata (#+TITLE:, #+AUTHOR:, etc.) via Go regex
//  3. Convert Org -> Markdown using Emacs batch mode (a derived 'md backend)
//  4. Feed Markdown through the existing goldmark -> WeChat HTML pipeline
//  5. Apply metadata (Org keywords + sidecar YAML override)
//
// Emacs is required for the Org->Markdown conversion. The Markdown is an
// internal intermediate -- the user only sees .org in and .html out.

package mdtowx

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"time"
)

// orgKeywordsRe matches Org #+KEYWORD: value lines.
// Case-insensitive keyword matching; value is trimmed.
var orgKeywordsRe = regexp.MustCompile(`(?mi)^[ \t]*#\+(\w+):[ \t]*(.*?)[ \t]*$`)

// ConvertOrgFile reads an Org file and converts it to WeChat-friendly HTML.
//
// It uses Emacs in batch mode to convert Org to Markdown internally,
// then feeds the result through goldmark for inline-style injection and
// custom renderers. The Markdown is never written to disk.
//
// Metadata is resolved in this priority order (higher wins):
//  1. Extracted from Org #+KEYWORDS (#+TITLE:, #+AUTHOR:, etc.)
//  2. Overridden by sidecar YAML (<filename>.yaml or .yml)
func ConvertOrgFile(filePath string) (*Result, error) {
	orgData, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("reading org file %q: %w", filePath, err)
	}

	// 1. Extract metadata from Org #+KEYWORDS.
	fm := parseOrgKeywords(orgData)
	if fm.Title != "" {
		if err := ValidateTitle(fm.Title); err != nil {
			return nil, err
		}
	}
	author := SanitizeAuthor(fm.Author)

	// 2. Convert Org to Markdown using Emacs batch mode.
	md, err := orgToMarkdown(filePath)
	if err != nil {
		return nil, err
	}

	// 3. Feed through the existing goldmark -> WeChat HTML pipeline.
	result, err := ConvertMarkdown(md)
	if err != nil {
		return nil, err
	}

	// 4. Apply Org metadata (overrides any front matter in the generated Markdown).
	if fm.Title != "" {
		result.Title = fm.Title
	}
	if fm.Author != "" {
		result.Author = author
	}
	if fm.ThumbMediaID != "" {
		result.ThumbMediaID = fm.ThumbMediaID
	}
	if fm.Digest != "" {
		result.Digest = fm.Digest
	}

	// 5. Sidecar YAML override (same logic as ConvertFile).
	meta, ok := readSidecarYAML(filePath)
	if ok {
		if meta.Title != "" {
			if err := ValidateTitle(meta.Title); err != nil {
				return nil, err
			}
			result.Title = meta.Title
		}
		if meta.Author != "" {
			result.Author = SanitizeAuthor(meta.Author)
		}
		if meta.ThumbMediaID != "" {
			result.ThumbMediaID = meta.ThumbMediaID
		}
		if meta.Digest != "" {
			result.Digest = meta.Digest
		}
	}

	return result, nil
}

// orgToMarkdown converts an Org file to Markdown using Emacs in batch mode.
//
// It writes a small Emacs Lisp file to disk and loads it via --load.
// The Lisp defines a derived backend 'my-md from 'md that emits:
//   - Fenced code blocks with language (instead of indented code)
//   - GFM-style tables (instead of raw HTML tables)
func orgToMarkdown(orgPath string) ([]byte, error) {
	// Verify Emacs is available.
	if _, err := exec.LookPath("emacs"); err != nil {
		return nil, fmt.Errorf("emacs not found: %w\n"+
			"Org-to-Markdown conversion requires GNU Emacs (>= 26.1) with Org mode. "+
			"Install it via your package manager or use a .md file instead.", err)
	}

	// Write the Emacs Lisp to a temp file to avoid quoting/escaping issues.
	tmpFile, err := os.CreateTemp("", "org2md-*.el")
	if err != nil {
		return nil, fmt.Errorf("creating temp elisp file: %w", err)
	}
	tmpPath := tmpFile.Name()
	defer os.Remove(tmpPath)

	if _, err := tmpFile.WriteString(orgToMarkdownElisp); err != nil {
		tmpFile.Close()
		return nil, fmt.Errorf("writing temp elisp: %w", err)
	}
	tmpFile.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "emacs",
		"--batch",
		"--visit="+orgPath,
		"--load="+tmpPath,
	)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return nil, fmt.Errorf("emacs org-to-md conversion timed out (30s)")
		}
		// Truncate stderr to a reasonable size for error messages.
		errMsg := stderr.String()
		if len(errMsg) > 1024 {
			errMsg = errMsg[:1024] + "... (truncated)"
		}
		return nil, fmt.Errorf("emacs org-to-md conversion failed: %w\nstderr: %s", err, errMsg)
	}

	return stdout.Bytes(), nil
}

// orgToMarkdownElisp is the Emacs Lisp loaded by orgToMarkdown.
//
// It defines a derived Org export backend 'my-md based on 'md (ox-md) with:
//   - Fenced code blocks: #+begin_src go becomes ```go ... ```
//   - GFM tables: Org tables become | A | B |\n |---|---| format
//   - Image captions: caption quotes escaped so goldmark parses the
//     Markdown title attribute correctly (ox-md emits them unescaped)
//
// The backtick character (ASCII 96) is generated via (string 96) to avoid
// any quoting issues when embedding in Go source.
const orgToMarkdownElisp = `(progn
  ;; Load the base Markdown exporter.
  (require 'ox-md)

  ;; --- Caption support ---
  ;; ox-md drops #+caption: on tables and src blocks (only images keep the
  ;; caption, as a Markdown title attribute). To carry the caption through
  ;; the Markdown intermediate, we emit an internal HTML comment marker
  ;; right before the block:
  ;;
  ;;   <!-- mdtowx-table-caption: TEXT -->
  ;;   <!-- mdtowx-code-caption:  TEXT -->
  ;;
  ;; The Go side (injectBlockCaptions in mdtowx.go) replaces these markers
  ;; with visible caption <span>s ("Table N: ..." / "Listing N: ..."),
  ;; mirroring org's HTML export numbering.
  (defun my-org-md-caption (el info)
    (let* ((caps (org-element-property :caption el))
           (lines (car caps)))  ;; first caption = the display caption
      (when lines
        (let ((text (org-trim (org-export-data lines info))))
          (unless (string-empty-p text)
            text)))))

  ;; --- Fenced code blocks with language info ---
  ;; Override ox-md's default src-block handler (which emits indented code)
  ;; to produce fenced code blocks.
  (defun my-org-md-src-block (src-block contents info)
    (let ((lang (or (org-element-property :language src-block) ""))
          (code (org-export-format-code-default src-block info))
          (bt (string 96))  ;; backtick char
          (cap (my-org-md-caption src-block info)))
      (concat (if cap (concat "<!-- mdtowx-code-caption: " cap " -->\n") "")
              bt bt bt lang "\n" code bt bt bt "\n")))

  ;; --- Image captions: escape title quotes ---
  ;; ox-md emits image captions verbatim inside the Markdown title
  ;; attribute: #+caption: "The" Circle becomes ![img](x.png ""The" Circle").
  ;; goldmark closes the title at the second quote and the whole line
  ;; degrades to literal text.  We escape backslash and double quote so the
  ;; Markdown intermediate stays valid (goldmark round-trips them exactly).
  (defun my-org-md-escape-title (s)
    (mapconcat
     (lambda (c)
       (cond ((= c ?\\) "\\\\")
             ((= c ?\") "\\\"")
             (t (char-to-string c))))
     s ""))

  ;; Override ox-md's link handler only for inline images with captions:
  ;; emit the title attribute with escaped quotes.  All other links (plain
  ;; links, images with a description, http links, ...) are delegated to
  ;; the parent 'md backend via org-export-with-backend.
  (defun my-org-md-link (link desc info)
    (if (org-export-inline-image-p link org-html-inline-image-rules)
        (let* ((type (org-element-property :type link))
               (raw-path (org-element-property :path link))
               (path (cond ((not (string-equal type "file"))
                            (concat type ":" raw-path))
                           ((not (file-name-absolute-p raw-path)) raw-path)
                           (t (expand-file-name raw-path))))
               (caption (org-export-data
                         (org-export-get-caption
                          (org-element-parent-element link))
                         info)))
          (if (org-string-nw-p caption)
              (format "![img](%s \"%s\")" path (my-org-md-escape-title caption))
            (format "![img](%s)" path)))
      (org-export-with-backend 'md link desc info)))

  ;; --- GFM-style tables ---
  ;; Override ox-md's default table handler (which emits raw HTML tables)
  ;; to produce standard GFM pipe tables.
  (defun my-org-md-table (table contents info)
    (let ((rows (org-element-map table 'table-row 'identity info))
          (output '())
          (header-done nil)
          (cap (my-org-md-caption table info)))
      (dolist (row rows)
        (let ((type (org-element-property :type row)))
          (when (eq type 'standard)
            (let ((cells (org-element-map row 'table-cell 'identity info))
                  (cell-strs '()))
              (dolist (cell cells)
                (push (org-trim (or (org-export-data (org-element-contents cell) info) "")) cell-strs))
              (setq cell-strs (nreverse cell-strs))
              (push (concat "| " (mapconcat #'identity cell-strs " | ") " |") output)
              (unless header-done
                (push (concat "| " (mapconcat
                                     (lambda (c) (make-string (max 3 (length c)) ?-))
                                     cell-strs " | ") " |") output)
                (setq header-done t))))))
      (let ((sep "\n"))
        (concat (if cap (concat "<!-- mdtowx-table-caption: " cap " -->\n") "")
                (mapconcat #'identity (nreverse output) sep) sep sep))))

  ;; Register the derived backend.
  (org-export-define-derived-backend 'my-md 'md
    :translate-alist '((src-block . my-org-md-src-block)
                       (table . my-org-md-table)
                       (link . my-org-md-link)))

  ;; Export current buffer (loaded via --visit) as Markdown string.
  ;; TOC is suppressed -- WeChat articles never need a table of contents.
  ;; Sub/superscript is disabled -- _ in filenames like sample_2.d2 must not
  ;; become subscript (<sub>) during export.
  (let ((org-export-with-toc nil)
        (org-export-with-section-numbers nil)
        (org-export-with-sub-superscripts nil))
    (princ (org-export-as 'my-md nil nil t))))
`

// parseOrgKeywords extracts metadata from Org #+KEYWORDS.
//
// Supported keywords (case-insensitive):
//
//	#+TITLE:          article title
//	#+AUTHOR:         author name
//	#+THUMB_MEDIA_ID: WeChat thumbnail media ID or local path
//	#+DIGEST:         article digest/description
//
// Values are trimmed of leading/trailing whitespace. Multi-line keyword
// values are not supported (only the first line is used).
func parseOrgKeywords(data []byte) frontMatter {
	var fm frontMatter
	matches := orgKeywordsRe.FindAllSubmatch(data, -1)
	for _, m := range matches {
		key := strings.ToUpper(string(m[1]))
		value := string(m[2])
		switch key {
		case "TITLE":
			if fm.Title == "" {
				fm.Title = value
			}
		case "AUTHOR":
			if fm.Author == "" {
				fm.Author = value
			}
		case "THUMB_MEDIA_ID":
			if fm.ThumbMediaID == "" {
				fm.ThumbMediaID = value
			}
		case "DIGEST":
			if fm.Digest == "" {
				fm.Digest = value
			}
		}
	}
	return fm
}

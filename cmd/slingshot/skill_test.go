package main

import (
	"fmt"
	"io/fs"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// bt is a single backtick. Test fixtures build fences from it so this file
// never contains a literal triple-backtick sequence.
const bt = "`"

// fenceRE matches a code-fence line: up to 3 leading spaces, at least 3
// backticks, and an optional trailing info string.
var fenceRE = regexp.MustCompile("^[ \t]{0,3}(`{3,})(.*)$")

// headingRE matches an ATX heading at level 2-6 followed by a space. Level 1
// ("# ...") is excluded on purpose: a single "#" opens a shell comment, so
// matching it would flag every commented command inside a shell block. Only a
// space counts as the separator (CommonMark), not a tab.
//
// Known limitation: "## ..." is also a valid shell comment style. The embedded
// skills do not use it, and the convention here is that a "## " line inside a
// code block is a leaked heading.
var headingRE = regexp.MustCompile("^[ \t]{0,3}#{2,6} ")

// casTagRE matches the read_file CAS checksum-tag pollution signature: a short
// base64-ish token, whitespace, then a fence and nothing else on the line. It
// is line-anchored on purpose, so ordinary code lines such as
// echo "abcd <fence>" do not match.
var casTagRE = regexp.MustCompile("^[ \t]{0,3}[A-Za-z0-9+/=]{4,}[ \t]+`{3,}[ \t]*$")

// shellishInfo reports whether a fence info string marks a shell-like or plain
// code block, where a leaked markdown heading is treated as a defect. Only the
// first token is inspected, so "bash linenos" still counts.
//
// Convention: use a "markdown" fence to show markdown examples. "text",
// "plain" and "txt" blocks are treated like shell blocks, so a heading inside
// them is reported.
func shellishInfo(info string) bool {
	fields := strings.Fields(info)
	if len(fields) == 0 {
		return true
	}
	switch strings.ToLower(fields[0]) {
	case "bash", "sh", "shell", "zsh", "console", "terminal", "cmd",
		"powershell", "plain", "text", "txt":
		return true
	}
	return false
}

func TestEmbeddedSkillMarkdownStructure(t *testing.T) {
	entries, err := fs.ReadDir(skillsFS, "embedded_skills")
	if err != nil {
		t.Fatalf("reading embedded_skills: %v", err)
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		name := entry.Name()
		t.Run(name, func(t *testing.T) {
			path := filepath.Join("embedded_skills", name, "SKILL.md")
			content, err := skillsFS.ReadFile(path)
			if err != nil {
				t.Fatalf("reading SKILL.md: %v", err)
			}
			for _, problem := range skillMarkdownProblems(string(content)) {
				t.Errorf("%s: %s", path, problem)
			}
		})
	}
}

// skillMarkdownProblems reports structural defects in one SKILL.md that review
// keeps missing, because a single-line edit can silently overwrite the wrong
// line (see commit 5457087):
//
//  1. an unclosed fence at EOF;
//  2. a fence line with an info string inside an open block: it cannot close
//     the block, so it means the previous block was never closed;
//  3. a markdown heading inside a shell-like or plain code block;
//  4. a read_file CAS checksum tag pasted into the document.
//
// The rules are heuristic on purpose. Documented limitations:
//
//   - a literal fence shown inside a longer fence (4+ backticks) is treated as
//     content, but an equal-length info fence inside a block is reported, so
//     wrap fence examples in a longer fence;
//   - a "## " line inside a shell-like block is always treated as a heading,
//     so do not use "##" as a shell comment separator.
func skillMarkdownProblems(content string) []string {
	var problems []string

	type openBlock struct {
		line   int
		length int
		info   string
	}
	var open *openBlock

	lines := strings.Split(content, "\n")

	for i, line := range lines {
		lineNo := i + 1

		if casTagRE.MatchString(line) {
			problems = append(problems, fmt.Sprintf(
				"line %d: CAS checksum tag pasted into the document: %q", lineNo, line))
		}

		m := fenceRE.FindStringSubmatch(line)
		if m == nil {
			if open != nil && headingRE.MatchString(line) && shellishInfo(open.info) {
				problems = append(problems, fmt.Sprintf(
					"line %d: markdown heading inside code block opened at line %d: %q",
					lineNo, open.line, strings.TrimSpace(line)))
			}
			continue
		}

		fenceLength, info := len(m[1]), strings.TrimSpace(m[2])

		if open == nil {
			open = &openBlock{line: lineNo, length: fenceLength, info: info}
			continue
		}

		// A closing fence carries no info string and is at least as long as
		// the opening fence (CommonMark).
		if info == "" && fenceLength >= open.length {
			open = nil
			continue
		}

		// A fence with an info string cannot close the block. If it is also
		// long enough to be a real fence, it is almost certainly a missing
		// closing fence above it.
		if info != "" && fenceLength >= open.length {
			problems = append(problems, fmt.Sprintf(
				"line %d: fence %q inside code block opened at line %d (missing closing fence?)",
				lineNo, info, open.line))
		}
	}

	if open != nil {
		problems = append(problems, fmt.Sprintf(
			"unclosed code fence opened at line %d (info %q), reaches EOF at line %d",
			open.line, open.info, len(lines)))
	}

	return problems
}

func TestSkillMarkdownProblems(t *testing.T) {
	f := bt + bt + bt
	ff := f + bt

	tests := []struct {
		name        string
		doc         string
		want        int
		wantSubstrs []string
	}{
		{
			name: "clean document",
			doc:  "## Title\n\n" + f + "bash\n# comment\necho hi\n" + f + "\n",
			want: 0,
		},
		{
			name: "clean yaml block",
			doc:  "## Title\n\n" + f + "yaml\n# comment\nkey: value\n" + f + "\n",
			want: 0,
		},
		{
			name: "unclosed fence at EOF",
			doc:  "## Title\n\n" + f + "bash\necho hi\n",
			want: 1,
			wantSubstrs: []string{
				"unclosed code fence opened at line 3",
				"reaches EOF",
			},
		},
		{
			name: "swallowed heading and next block",
			doc:  f + "bash\necho hi\n\n### 3. Section\n\n" + f + "bash\necho again\n" + f + "\n",
			want: 2,
			wantSubstrs: []string{
				"line 4: markdown heading inside code block opened at line 1",
				"line 6: fence \"bash\" inside code block opened at line 1",
			},
		},
		{
			name:        "heading inside shell block",
			doc:         f + "bash\necho hi\n\n## Heading\n" + f + "\n",
			want:        1,
			wantSubstrs: []string{"line 4: markdown heading"},
		},
		{
			name: "literal fence inside longer fence",
			doc:  ff + "markdown\n" + f + "bash\necho hi\n" + f + "\n" + ff + "\n",
			want: 0,
		},
		{
			name:        "same-length info fence is reported by convention",
			doc:         f + "text\n" + f + "js\n" + f + "\n",
			want:        1,
			wantSubstrs: []string{"line 2: fence \"js\""},
		},
		{
			name:        "heading inside text block is reported by convention",
			doc:         f + "text\n## Heading\n" + f + "\n",
			want:        1,
			wantSubstrs: []string{"line 2: markdown heading"},
		},
		{
			name: "tab after hashes is not a heading",
			doc:  f + "bash\n##\tNot a heading\n" + f + "\n",
			want: 0,
		},
		{
			name:        "cas tag pollution",
			doc:         "## Title\n\n" + f + "bash\nPM0a " + f + "\n" + f + "\n",
			want:        1,
			wantSubstrs: []string{"line 4: CAS checksum tag"},
		},
		{
			name:        "indented cas tag pollution",
			doc:         "## Title\n\n" + f + "bash\n  abcd1234 " + f + "\n" + f + "\n",
			want:        1,
			wantSubstrs: []string{"line 4: CAS checksum tag"},
		},
		{
			name: "quoted backticks in shell are not a cas tag",
			doc:  "## Title\n\n" + f + "bash\necho \"abcd " + f + "\"\n" + f + "\n",
			want: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := skillMarkdownProblems(tt.doc)
			if len(got) != tt.want {
				t.Fatalf("got %d problems, want %d: %v", len(got), tt.want, got)
			}
			joined := strings.Join(got, "\n")
			for _, want := range tt.wantSubstrs {
				if !strings.Contains(joined, want) {
					t.Errorf("problems do not contain %q: %v", want, got)
				}
			}
		})
	}
}

// TestWeixinSkillDocumentsDraftAndMaterialCommands guards against silently
// deleted command examples, which structural checks cannot see. Note that the
// subcommand really is spelled "meterial" — see cmd/slingshot/meterial_remove.go.
func TestWeixinSkillDocumentsDraftAndMaterialCommands(t *testing.T) {
	content, err := skillsFS.ReadFile(filepath.Join("embedded_skills", "weixin", "SKILL.md"))
	if err != nil {
		t.Fatalf("reading weixin SKILL.md: %v", err)
	}

	text := string(content)

	for _, want := range []string{
		"slingshot draft add article.html --thumb <media_id>",
		"slingshot meterial remove <media_id>",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("weixin SKILL.md is missing documented command %q", want)
		}
	}
}

package main

import (
	"io/fs"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// fenceRE matches a code-fence line: up to 3 leading spaces, at least 3
// backticks, and an optional trailing info string.
var fenceRE = regexp.MustCompile("^(\\s{0,3})(`{3,})(.*)$")

// headingRE matches an ATX heading at level 2-6.
var headingRE = regexp.MustCompile(`^\s{0,3}#{2,6} `)

// casTagRE matches the read_file CAS checksum-tag pollution signature:
// four base64-ish characters, a space, then a fence of at least 3 backticks.
var casTagRE = regexp.MustCompile("^[A-Za-z0-9+/=]{4} `{3,}\\s*$")

// shellishInfos are fence info strings that mark shell-like (or plain) code
// blocks, where a markdown heading cannot legitimately appear.
var shellishInfos = map[string]bool{
	"":           true,
	"bash":       true,
	"sh":         true,
	"shell":      true,
	"zsh":        true,
	"console":    true,
	"terminal":   true,
	"cmd":        true,
	"powershell": true,
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
		path := filepath.Join("embedded_skills", name, "SKILL.md")

		content, err := skillsFS.ReadFile(path)
		if err != nil {
			t.Errorf("%s: reading SKILL.md: %v", name, err)
			continue
		}

		checkSkillMarkdown(t, name, string(content))
	}
}

func checkSkillMarkdown(t *testing.T, name, content string) {
	t.Helper()

	lines := strings.Split(content, "\n")

	type openBlock struct {
		line   int
		length int
		info   string
	}
	var open *openBlock

	for i, line := range lines {
		lineNo := i + 1

		if m := casTagRE.FindStringSubmatch(line); m != nil {
			t.Errorf("%s:%d CAS-tag pollution: %q", name, lineNo, line)
		}

		m := fenceRE.FindStringSubmatch(line)
		if m == nil {
			// Not a fence line.
			if open != nil && headingRE.MatchString(line) && shellishInfos[open.info] {
				t.Errorf("%s:%d markdown heading inside ```%s block opened at line %d: %q",
					name, lineNo, open.info, open.line, strings.TrimSpace(line))
			}
			continue
		}

		fenceLength := len(m[2])
		info := strings.TrimSpace(m[3])

		if open == nil {
			// No open block: this line opens one.
			open = &openBlock{line: lineNo, length: fenceLength, info: info}
			continue
		}

		// We are inside a block. This line closes it only if its info string
		// is empty and its fence length is >= the opening fence length.
		if info == "" && fenceLength >= open.length {
			open = nil
			continue
		}

		// Otherwise it is an opening fence nested inside the open block.
		t.Errorf("%s:%d opening fence ```%s inside block opened at line %d (missing closing fence?)",
			name, lineNo, info, open.line)
	}

	if open != nil {
		t.Errorf("%s:%d unclosed code fence opened at line %d (info %q)",
			name, len(lines)+1, open.line, open.info)
	}
}

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

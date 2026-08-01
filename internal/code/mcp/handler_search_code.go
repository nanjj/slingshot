// Package mcp — search_code: graph-augmented code search using native Go file scanning.
//
// Replaces the previous ripgrep (rg) subprocess approach with a pure Go
// file walk + line scan. This eliminates an external binary dependency and
// a class of hard-to-debug subprocess/JSON-parsing bugs.

package mcp

import (
	"bufio"
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/nanjj/clog"
)

// ─── Argument struct ───────────────────────────────────────────────────────

type searchCodeArgs struct {
	Pattern          string `json:"pattern,omitempty"`      // required: search pattern
	Regex            string `json:"regex,omitempty"`        // alias for pattern
	Project          string `json:"project,omitempty"`      // project name/path (optional once open_project is bound)
	FilePattern      string `json:"filePattern,omitempty"`  // glob filter (e.g. *.go)
	FilePatternSnake string `json:"file_pattern,omitempty"` // alias for filePattern
	PathFilter       string `json:"pathFilter,omitempty"`   // substring filter on file path
	PathFilterSnake  string `json:"path_filter,omitempty"`  // alias for pathFilter
	File             string `json:"file,omitempty"`         // alias for pathFilter (limit search to a file/subtree)
	Context          int    `json:"context,omitempty"`      // context lines (like grep -C)
	Mode             string `json:"mode,omitempty"`         // compact (default), full, files
	Limit            int    `json:"limit,omitempty"`        // max enriched results
}

// ─── Internal match type (replaces rg JSON types) ──────────────────────────

// lineMatch holds a single pattern match found in a file.
type lineMatch struct {
	File        string // absolute file path
	Line        int    // 1-based line number
	Column      int    // 1-based column (position of match in line)
	Text        string // full line text (trimmed of trailing newline)
	Match       string // the substring that matched
	ContextPre  string // lines before (context > 0), joined by \n
	ContextPost string // lines after (context > 0), joined by \n
}

// ─── Result types ──────────────────────────────────────────────────────────

type searchCodeItem struct {
	FunctionName  string `json:"functionName,omitempty"`
	QualifiedName string `json:"qualifiedName,omitempty"`
	Kind          string `json:"kind,omitempty"`
	File          string `json:"file"`
	Line          int    `json:"line"`
	Column        int    `json:"column,omitempty"`
	MatchLine     string `json:"matchLine,omitempty"`
	ContextPre    string `json:"contextPre,omitempty"`
	ContextPost   string `json:"contextPost,omitempty"`
	Source        string `json:"source,omitempty"` // full mode only
}

type searchCodeResult struct {
	Results      []searchCodeItem `json:"results"`
	TotalMatches int              `json:"totalMatches"`
	TotalResults int              `json:"totalResults"`
	Stats        searchCodeStats  `json:"stats"`
}

type searchCodeStats struct {
	FilesSearched int    `json:"filesSearched"`
	ElapsedSecs   string `json:"elapsedSecs"`
}

// ─── Register ──────────────────────────────────────────────────────────────
func registerSearchCode(srv *mcp.Server, s *Server) {
	mcp.AddTool(srv, &mcp.Tool{
		Name: "search_code",
		Description: "Graph-augmented code search. Finds text patterns via native " +
			"file scanning, then enriches results with the knowledge graph: deduplicates " +
			"matches into containing functions, ranks by structural importance " +
			"(definitions first, popular functions next, tests last). " +
			"Modes: compact (default, signatures only), full (with source), files (just file paths). " +
			"project is optional once open_project has been called. Aliases: regex (pattern), file_pattern (filePattern), file (pathFilter).",
		InputSchema: strictSchema[searchCodeArgs](),
	}, func(ctx context.Context, req *mcp.CallToolRequest, args searchCodeArgs) (*mcp.CallToolResult, any, error) {
		return s.handleSearchCode(ctx, args), nil, nil
	})
}

// ─── Handler ───────────────────────────────────────────────────────────────

func (s *Server) handleSearchCode(ctx context.Context, args searchCodeArgs) *mcp.CallToolResult {
	span, ctx := clog.StartSpanFromContext(ctx, "search_code")
	defer span.Finish()

	// Normalize aliases: LLMs naturally send regex / file_pattern / file.
	pattern := firstNonEmpty(args.Pattern, args.Regex)
	filePattern := firstNonEmpty(args.FilePattern, args.FilePatternSnake)
	pathFilter := firstNonEmpty(args.PathFilter, args.PathFilterSnake, args.File)

	clog.Info(ctx, "search_code", "project", args.Project, "pattern", pattern, "filePattern", filePattern, "mode", args.Mode)

	if pattern == "" {
		clog.Error(ctx, "error", "error", "pattern is required")
		return errorResult(fmt.Errorf("pattern is required — pass pattern (or regex)"))
	}

	info, err := s.resolveProject(args.Project)
	if err != nil {
		clog.Error(ctx, "error", "error", err.Error())
		return errorResult(err)
	}
	rootDir := info.Root
	if rootDir == "" {
		return errorResult(fmt.Errorf("project root not found for %q", info.Name))
	}

	limit := args.Limit
	if limit <= 0 {
		limit = 20
	}

	// Run native file scan
	matches, filesSearched, elapsed, err := scanFiles(rootDir, pattern, filePattern, args.Context)
	if err != nil {
		clog.Error(ctx, "error", "error", err.Error())
		return errorResult(fmt.Errorf("file scan failed (index may be stale — run index_repository to refresh): %w", err))
	}

	totalMatches := len(matches)

	// Enrich with graph data: deduplicate into containing functions
	enriched := s.enrichMatches(info.Name, matches, rootDir, args.Mode)

	// Apply path filter
	if pathFilter != "" {
		var filtered []searchCodeItem
		for _, item := range enriched {
			if strings.Contains(item.File, pathFilter) {
				filtered = append(filtered, item)
			}
		}
		enriched = filtered
	}

	totalResults := len(enriched)
	clog.Info(ctx, "search_code_result", "totalMatches", totalMatches, "totalResults", totalResults, "filesSearched", filesSearched, "elapsed", fmt.Sprintf("%.3fs", elapsed))

	return jsonResult(searchCodeResult{
		Results:      enriched,
		TotalMatches: totalMatches,
		TotalResults: totalResults,
		Stats: searchCodeStats{
			FilesSearched: filesSearched,
			ElapsedSecs:   fmt.Sprintf("%.3fs", elapsed),
		},
	})
}

// ─── Native file scanning (replaces rg subprocess) ─────────────────────────

// defaultSkipDirs lists directories to skip during file walks.
// These are typically cache, build, or VCS directories that never contain
// source code the user wants to search. Includes release/output dirs where
// compiled binaries land (Archimedes feedback: _release binaries were being
// scanned as text, causing panics and noise).
var defaultSkipDirs = map[string]bool{
	".git":         true,
	".dscli":       true,
	".svn":         true,
	".hg":          true,
	"node_modules": true,
	"vendor":       true,
	"__pycache__":  true,
	".venv":        true,
	"venv":         true,
	"env":          true,
	".tox":         true,
	".cache":       true,
	"dist":         true,
	"_dist":        true,
	"release":      true,
	"_release":     true,
	"out":          true,
	"build":        true,
	"_build":       true,
	"coverage":     true,
	"_coverage":    true,
	"_artifacts":   true,
	".next":        true,
	".turbo":       true,
	"target":       true, // Rust
	"bin":          true, // Go build output
	"obj":          true, // C# / .NET
}

// isBinaryMagic reports whether the first 4 bytes look like a compiled
// binary's magic number: ELF, PE (MZ), Mach-O (both byte orders, 32/64-bit,
// and fat binaries), or Java class files (CAFEBABE — usually with a .class
// extension, but keep the check for extension-less copies).
func isBinaryMagic(header []byte) bool {
	if len(header) < 4 {
		return false
	}
	// ELF: 7f 45 4c 46
	if header[0] == 0x7f && header[1] == 'E' && header[2] == 'L' && header[3] == 'F' {
		return true
	}
	// PE: MZ
	if header[0] == 'M' && header[1] == 'Z' {
		return true
	}
	// Mach-O: FE ED FA CE / FE ED FA CF (big-endian),
	//         CE FA ED FE / CF FA ED FE (little-endian)
	// Fat Mach-O: CA FE BA BE / BE BA FE CA
	switch {
	case header[0] == 0xFE && header[1] == 0xED && header[2] == 0xFA && (header[3] == 0xCE || header[3] == 0xCF):
		return true
	case header[0] == 0xCE && header[1] == 0xFA && header[2] == 0xED && (header[3] == 0xFE || header[3] == 0xFF):
		return true
	case header[0] == 0xCA && header[1] == 0xFE && header[2] == 0xBA && header[3] == 0xBE:
		return true
	case header[0] == 0xBE && header[1] == 0xBA && header[2] == 0xFE && header[3] == 0xCA:
		return true
	}
	return false
}

// scanFiles walks rootDir and returns all lines matching pattern.
// It replaces the previous ripgrep subprocess with pure Go file I/O.
func scanFiles(rootDir, pattern, filePattern string, context int) ([]lineMatch, int, float64, error) {
	start := time.Now()

	var matches []lineMatch
	filesSeen := make(map[string]bool)
	patternLower := strings.ToLower(pattern)

	walkFn := func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			// Permission denied or similar — skip
			return nil
		}

		// Skip directories
		if d.IsDir() {
			if defaultSkipDirs[d.Name()] {
				return fs.SkipDir
			}
			// Skip hidden directories (starting with .) except the root
			if strings.HasPrefix(d.Name(), ".") && path != rootDir {
				return fs.SkipDir
			}
			return nil
		}

		// Only regular files
		if !d.Type().IsRegular() {
			return nil
		}

		// Skip binary-like extensions
		ext := strings.ToLower(filepath.Ext(path))
		switch ext {
		case ".exe", ".dll", ".so", ".dylib", ".bin", ".o", ".a", ".lib",
			".class", ".jar", ".war", ".ear", ".wasm", ".node", ".pyc", ".pyo",
			".png", ".jpg", ".jpeg", ".gif", ".bmp",
			".woff", ".woff2", ".ttf", ".eot", ".otf",
			".zip", ".tar", ".gz", ".bz2", ".xz", ".zst",
			".pdf", ".doc", ".docx", ".xls", ".xlsx", ".pptx",
			".mp3", ".mp4", ".avi", ".mov", ".wav", ".flac",
			".db", ".sqlite", ".wal", ".shm",
			".ico", ".icns":
			return nil
		}

		// Skip files with no extension that look like compiled binaries.
		if ext == "" {
			f, err := os.Open(path)
			if err == nil {
				header := make([]byte, 8)
				n, _ := f.Read(header)
				f.Close()
				if n >= 4 {
					if isBinaryMagic(header) {
						return nil
					}
				}
			}
		}

		// Apply glob filter
		if filePattern != "" {
			matched, err := filepath.Match(filePattern, d.Name())
			if err != nil || !matched {
				return nil
			}
		}

		filesSeen[path] = true

		// Open and scan file
		f, err := os.Open(path)
		if err != nil {
			return nil // skip unreadable files
		}
		defer f.Close()

		scanner := bufio.NewScanner(f)
		// Increase max line length (default 64KB may truncate)
		scanner.Buffer(make([]byte, 0, 1024*1024), 1024*1024)

		var lineNum int
		// Rolling buffer for context lines before a match
		var preBuffer []string
		preBufferCap := context
		if preBufferCap <= 0 {
			preBufferCap = 0
		}

		for scanner.Scan() {
			lineNum++
			text := scanner.Text()

			// Reject lines with invalid UTF-8 (binary content that slipped
			// past magic/extension checks). ToLower would replace invalid
			// bytes with 3-byte U+FFFD runes, shifting byte offsets and
			// causing slice out-of-range panics further down (Archimedes
			// feedback). This also keeps binary noise out of results.
			if !utf8.ValidString(text) {
				continue
			}

			if preBufferCap > 0 {
				preBuffer = append(preBuffer, text)
				if len(preBuffer) > preBufferCap {
					preBuffer = preBuffer[1:]
				}
			}

			// Check for match (case-insensitive)
			if !strings.Contains(strings.ToLower(text), patternLower) {
				continue
			}

			// Find the match position (case-insensitive search within the line)
			lowerLine := strings.ToLower(text)
			idx := strings.Index(lowerLine, patternLower)
			if idx < 0 {
				continue
			}

			// Defensive bounds check: ToLower can change byte lengths for
			// some Unicode characters, so the lowercased index may exceed
			// the original text's length. Skip such lines rather than panic.
			if idx+len(pattern) > len(text) {
				continue
			}

			col := idx + 1 // 1-based
			matchText := text[idx : idx+len(pattern)]

			lm := lineMatch{
				File:   path,
				Line:   lineNum,
				Column: col,
				Text:   text,
				Match:  matchText,
			}

			// Capture context lines
			if preBufferCap > 0 && len(preBuffer) > 0 {
				// preBuffer contains lines before this match, up to context count
				startIdx := 0
				if len(preBuffer) > preBufferCap {
					startIdx = len(preBuffer) - preBufferCap
				}
				lm.ContextPre = strings.Join(preBuffer[startIdx:], "\n")
			}

			matches = append(matches, lm)
		}

		// Ignore scan errors (truncated lines, binary content, etc.)
		return nil
	}

	err := filepath.WalkDir(rootDir, walkFn)
	if err != nil {
		return nil, 0, 0, fmt.Errorf("walk dir: %w", err)
	}

	elapsed := time.Since(start).Seconds()
	return matches, len(filesSeen), elapsed, nil
}

// ─── Graph enrichment ──────────────────────────────────────────────────────

// enrichMatches groups raw line matches by containing function/method using
// the code graph, deduplicates, and ranks results.
func (s *Server) enrichMatches(project string, matches []lineMatch, rootDir string, mode string) []searchCodeItem {
	// Group matches by file
	fileMatches := make(map[string][]lineMatch)
	for _, m := range matches {
		file := m.File
		fileMatches[file] = append(fileMatches[file], m)
	}

	// Files mode: just unique files
	if mode == "files" {
		seen := make(map[string]bool)
		var results []searchCodeItem
		for file := range fileMatches {
			relFile := relPath(file, rootDir)
			if !seen[relFile] {
				seen[relFile] = true
				results = append(results, searchCodeItem{File: relFile})
			}
		}
		return results
	}

	type funcKey struct {
		file string
		qn   string
	}
	seen := make(map[funcKey]bool)
	var results []searchCodeItem

	for file, lms := range fileMatches {
		relFile := relPath(file, rootDir)

		// Build line set
		lineSet := make(map[int]lineMatch)
		for _, lm := range lms {
			lineSet[lm.Line] = lm
		}

		// Get graph nodes for this file
		nodes, err := s.store.GetNodesByFile(file, project)
		if err != nil || len(nodes) == 0 {
			// No graph data — flat per-line results
			for _, lm := range lms {
				results = append(results, searchCodeItem{
					File:        relFile,
					Line:        lm.Line,
					Column:      lm.Column,
					MatchLine:   lm.Text,
					ContextPre:  lm.ContextPre,
					ContextPost: lm.ContextPost,
				})
			}
			continue
		}

		// Sort nodes by line
		sort.Slice(nodes, func(i, j int) bool {
			return nodes[i].Line < nodes[j].Line
		})

		// For each line, find containing function
		remaining := make(map[int]lineMatch)
		for k, v := range lineSet {
			remaining[k] = v
		}

		for _, n := range nodes {
			if n.Kind != "function" && n.Kind != "method" && n.Kind != "class" &&
				n.Kind != "struct" && n.Kind != "interface" {
				continue
			}

			var matchedLines []int
			for line := range remaining {
				lineU32 := uint32(line)
				if lineU32 >= n.Line && lineU32 <= n.EndLine {
					matchedLines = append(matchedLines, line)
				}
			}
			if len(matchedLines) == 0 {
				continue
			}

			for _, l := range matchedLines {
				delete(remaining, l)
			}

			key := funcKey{file: file, qn: n.QualifiedName}
			if seen[key] {
				continue
			}
			seen[key] = true

			sort.Ints(matchedLines)
			firstLine := matchedLines[0]

			item := searchCodeItem{
				FunctionName:  n.Name,
				QualifiedName: n.QualifiedName,
				Kind:          n.Kind,
				File:          relFile,
				Line:          firstLine,
				MatchLine:     lineSet[firstLine].Text,
				ContextPre:    lineSet[firstLine].ContextPre,
				ContextPost:   lineSet[firstLine].ContextPost,
			}

			if mode == "full" {
				sig := n.Signature
				if sig == "" {
					sig = n.QualifiedName
				}
				item.Source = sig
				if n.DocComment != "" {
					item.Source = n.DocComment + "\n" + sig
				}
			}

			results = append(results, item)
		}

		// Remaining lines not contained in any definition
		for line, lm := range remaining {
			results = append(results, searchCodeItem{
				File:        relFile,
				Line:        line,
				MatchLine:   lm.Text,
				Column:      lm.Column,
				ContextPre:  lm.ContextPre,
				ContextPost: lm.ContextPost,
			})
		}
	}

	sortSearchCodeResults(results)
	return results
}

// ─── Ranking ───────────────────────────────────────────────────────────────

func sortSearchCodeResults(items []searchCodeItem) {
	sort.Slice(items, func(i, j int) bool {
		// Items with function name (graph-enriched) first
		iHasFn := items[i].FunctionName != ""
		jHasFn := items[j].FunctionName != ""
		if iHasFn != jHasFn {
			return iHasFn
		}

		// Among enriched items: kind priority
		if iHasFn {
			ip := kindPriority(items[i].Kind)
			jp := kindPriority(items[j].Kind)
			if ip != jp {
				return ip < jp
			}
		}

		// Test files last
		iIsTest := testFileSuffix(items[i].File)
		jIsTest := testFileSuffix(items[j].File)
		if iIsTest != jIsTest {
			return jIsTest
		}

		// By file and line
		if items[i].File != items[j].File {
			return items[i].File < items[j].File
		}
		return items[i].Line < items[j].Line
	})
}

func testFileSuffix(file string) bool {
	return strings.HasSuffix(file, "_test.go") ||
		strings.HasSuffix(file, "_test.py") ||
		strings.HasSuffix(file, "_test.rs") ||
		strings.HasSuffix(file, "_test.js") ||
		strings.HasSuffix(file, ".test.ts") ||
		strings.HasSuffix(file, "_test.c")
}

func kindPriority(k string) int {
	switch k {
	case "function":
		return 0
	case "method":
		return 1
	case "class":
		return 2
	case "struct":
		return 3
	case "interface":
		return 4
	default:
		return 5
	}
}

// ─── Path helpers ──────────────────────────────────────────────────────────

func relPath(absPath, rootDir string) string {
	if strings.HasPrefix(absPath, rootDir) {
		rel := strings.TrimPrefix(absPath, rootDir)
		return strings.TrimPrefix(rel, "/")
	}
	return absPath
}

package mcp

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// ─── Argument struct ──────────────────────────────────────────────────────

type searchCodeArgs struct {
	Pattern     string `json:"pattern"`               // required: search pattern
	Project     string `json:"project"`               // required: project name
	FilePattern string `json:"filePattern,omitempty"` // glob filter (e.g. *.go)
	PathFilter  string `json:"pathFilter,omitempty"`  // regex filter on file path
	Context     int    `json:"context,omitempty"`      // context lines (like grep -C)
	Mode        string `json:"mode,omitempty"`         // compact (default), full, files
	Limit       int    `json:"limit,omitempty"`        // max enriched results
}

// ─── rg JSON output types ─────────────────────────────────────────────────

type rgMatch struct {
	Type string `json:"type"`
	Data rgData `json:"data"`
}

type rgData struct {
	Path           rgPath       `json:"path"`
	Lines          rgLines      `json:"lines"`
	LineNumber     int          `json:"line_number"`
	AbsoluteOffset int64        `json:"absolute_offset"`
	SubMatches     []rgSubMatch `json:"submatches"`
}

type rgPath struct {
	Text string `json:"text"`
}

type rgLines struct {
	Text string `json:"text"`
}

type rgSubMatch struct {
	Match string `json:"match"`
	Start int    `json:"start"`
	End   int    `json:"end"`
}

type rgBegin struct {
	Type string      `json:"type"`
	Data rgBeginData `json:"data"`
}

type rgBeginData struct {
	Path rgPath `json:"path"`
}

type rgEnd struct {
	Type string    `json:"type"`
	Data rgEndData `json:"data"`
}

type rgEndData struct {
	Path  rgPath  `json:"path"`
	Stats rgStats `json:"stats"`
}

type rgStats struct {
	Elapsed rgElapsed `json:"elapsed"`
}

type rgElapsed struct {
	Secs float64 `json:"secs"`
}

// ─── Result types ─────────────────────────────────────────────────────────

type searchCodeItem struct {
	FunctionName  string `json:"functionName,omitempty"`
	QualifiedName string `json:"qualifiedName,omitempty"`
	Kind          string `json:"kind,omitempty"`
	File          string `json:"file"`
	Line          int    `json:"line"`
	Column        int    `json:"column,omitempty"`
	MatchLine     string `json:"matchLine,omitempty"`
	Source        string `json:"source,omitempty"` // full mode only
}

type searchCodeResult struct {
	Results          []searchCodeItem `json:"results"`
	TotalGrepMatches int              `json:"totalGrepMatches"`
	TotalResults     int              `json:"totalResults"`
	Stats            searchCodeStats  `json:"stats"`
}

type searchCodeStats struct {
	FilesSearched int    `json:"filesSearched"`
	ElapsedSecs   string `json:"elapsedSecs"`
}

// ─── Register ─────────────────────────────────────────────────────────────

func registerSearchCode(srv *mcp.Server, s *Server) {
	mcp.AddTool(srv, &mcp.Tool{
		Name: "search_code",
		Description: "Graph-augmented code search. Finds text patterns via grep, then enriches " +
			"results with the knowledge graph: deduplicates matches into containing functions, " +
			"ranks by structural importance (definitions first, popular functions next, tests last). " +
			"Modes: compact (default, signatures only), full (with source), files (just file paths).",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args searchCodeArgs) (*mcp.CallToolResult, any, error) {
		return s.handleSearchCode(args), nil, nil
	})
}

// ─── Handler ──────────────────────────────────────────────────────────────

func (s *Server) handleSearchCode(args searchCodeArgs) *mcp.CallToolResult {
	if args.Pattern == "" {
		return errorResult(fmt.Errorf("pattern is required"))
	}
	if args.Project == "" {
		return errorResult(fmt.Errorf("project is required"))
	}

	info, err := s.store.ProjectStatus(args.Project)
	if err != nil {
		return errorResult(fmt.Errorf("project status: %w", err))
	}
	rootDir := info.Root
	if rootDir == "" {
		return errorResult(fmt.Errorf("project root not found"))
	}

	limit := args.Limit
	if limit <= 0 {
		limit = 20
	}

	// Run rg
	matches, filesSearched, elapsed, err := runRG(rootDir, args.Pattern, args.FilePattern, args.Context)
	if err != nil {
		return errorResult(fmt.Errorf("rg search failed: %w", err))
	}

	totalGrep := len(matches)

	// Enrich with graph data: deduplicate into containing functions
	enriched := s.enrichMatches(args.Project, matches, rootDir, args.Mode)

	// Apply path filter
	if args.PathFilter != "" {
		var filtered []searchCodeItem
		for _, item := range enriched {
			if strings.Contains(item.File, args.PathFilter) {
				filtered = append(filtered, item)
			}
		}
		enriched = filtered
	}

	totalResults := len(enriched)

	return jsonResult(searchCodeResult{
		Results:          enriched,
		TotalGrepMatches: totalGrep,
		TotalResults:     totalResults,
		Stats: searchCodeStats{
			FilesSearched: filesSearched,
			ElapsedSecs:   fmt.Sprintf("%.3fs", elapsed),
		},
	})
}

// ─── rg execution ─────────────────────────────────────────────────────────

func runRG(rootDir, pattern, filePattern string, context int) ([]rgMatch, int, float64, error) {
	args := []string{"--json", "-n", "--no-heading"}
	if context > 0 {
		args = append(args, fmt.Sprintf("-C%d", context))
	}
	if filePattern != "" {
		args = append(args, "--glob", filePattern)
	}
	args = append(args, pattern, rootDir)

	cmd := exec.Command("rg", args...)
	out, err := cmd.Output()
	if err != nil {
		// rg returns exit code 1 when no matches — not an error for us
		if exitErr, ok := err.(*exec.ExitError); ok {
			if exitErr.ExitCode() == 1 {
				return []rgMatch{}, 0, 0, nil
			}
		}
		return nil, 0, 0, fmt.Errorf("exec rg: %w", err)
	}

	var matches []rgMatch
	filesSeen := make(map[string]bool)
	var lastElapsed float64

	scanner := bufio.NewScanner(strings.NewReader(string(out)))
	for scanner.Scan() {
		line := scanner.Bytes()
		var base struct {
			Type string `json:"type"`
		}
		if err := json.Unmarshal(line, &base); err != nil {
			continue
		}

		switch base.Type {
		case "match":
			var m rgMatch
			if err := json.Unmarshal(line, &m); err == nil {
				matches = append(matches, m)
				filesSeen[m.Data.Path.Text] = true
			}
		case "begin":
			var b rgBegin
			if err := json.Unmarshal(line, &b); err == nil {
				filesSeen[b.Data.Path.Text] = true
			}
		case "end":
			var e rgEnd
			if err := json.Unmarshal(line, &e); err == nil {
				lastElapsed = e.Data.Stats.Elapsed.Secs
			}
		}
	}

	return matches, len(filesSeen), lastElapsed, nil
}

// ─── Graph enrichment ─────────────────────────────────────────────────────

func (s *Server) enrichMatches(project string, matches []rgMatch, rootDir string, mode string) []searchCodeItem {
	type lineMatch struct {
		Line  int
		Text  string
		Match string
		Col   int
	}

	// Group matches by file
	fileMatches := make(map[string][]lineMatch)
	for _, m := range matches {
		file := m.Data.Path.Text
		if !filepath.IsAbs(file) {
			file = filepath.Join(rootDir, file)
		}

		col := 0
		matchText := ""
		if len(m.Data.SubMatches) > 0 {
			col = m.Data.SubMatches[0].Start + 1
			matchText = m.Data.SubMatches[0].Match
		}

		fileMatches[file] = append(fileMatches[file], lineMatch{
			Line:  m.Data.LineNumber,
			Text:  strings.TrimRight(m.Data.Lines.Text, "\n\r"),
			Match: matchText,
			Col:   col,
		})
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
		lineSet := make(map[int]string)
		for _, lm := range lms {
			lineSet[lm.Line] = lm.Text
		}

		// Get graph nodes for this file
		nodes, err := s.store.GetNodesByFile(file, project)
		if err != nil || len(nodes) == 0 {
			// No graph data — flat per-line results
			for _, lm := range lms {
				results = append(results, searchCodeItem{
					File:      relFile,
					Line:      lm.Line,
					Column:    lm.Col,
					MatchLine: lm.Text,
				})
			}
			continue
		}

		// Sort nodes by line
		sort.Slice(nodes, func(i, j int) bool {
			return nodes[i].Line < nodes[j].Line
		})

		// For each line, find containing function
		remaining := make(map[int]string)
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
				MatchLine:     lineSet[firstLine],
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
		for line, text := range remaining {
			results = append(results, searchCodeItem{
				File:      relFile,
				Line:      line,
				MatchLine: text,
			})
		}
	}

	sortSearchCodeResults(results)
	return results
}

// ─── Ranking ──────────────────────────────────────────────────────────────

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
		iIsTest := strings.Contains(items[i].File, "_test.go") || strings.Contains(items[i].File, "_test.py")
		jIsTest := strings.Contains(items[j].File, "_test.go") || strings.Contains(items[j].File, "_test.py")
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

// ─── Path helpers ─────────────────────────────────────────────────────────

func relPath(absPath, rootDir string) string {
	if strings.HasPrefix(absPath, rootDir) {
		rel := strings.TrimPrefix(absPath, rootDir)
		return strings.TrimPrefix(rel, "/")
	}
	return absPath
}

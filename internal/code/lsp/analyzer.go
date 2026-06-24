// Package lsp provides tree-sitter based code analysis operations.
//
// It wraps the gotreesitter library to provide file-level code intelligence:
// parsing, structure navigation, definition extraction, syntax validation,
// and AST queries. This package is the "read-only analysis" layer — it does
// not handle document lifecycle or file I/O (those belong to the caller).
//
// The Analyzer type is stateless (no open documents) — each method parses
// the file fresh. Callers (mcp handlers) are responsible for caching if needed.
package lsp

import (
	"github.com/odvcencio/gotreesitter"
)

// Analyzer provides tree-sitter based code analysis operations.
// It is stateless — each method parses the file fresh.
// Safe for concurrent use (no shared mutable state).
type Analyzer struct{}

// NewAnalyzer creates a new Analyzer.
func NewAnalyzer() *Analyzer {
	return &Analyzer{}
}

// ParseResult holds the parsed tree and metadata for a file.
type ParseResult struct {
	Language string `json:"language"`
	Source   []byte `json:"-"`
	Tree     *gotreesitter.Tree
}

// NodeInfo is a serializable AST node description.
type NodeInfo struct {
	Type       string     `json:"type"`
	StartByte  uint32     `json:"startByte"`
	EndByte    uint32     `json:"endByte"`
	StartPoint [2]uint32 `json:"startPoint"`
	EndPoint   [2]uint32 `json:"endPoint"`
	Text       string     `json:"text,omitempty"`
	IsNamed    bool       `json:"isNamed"`
	IsError    bool       `json:"isError"`
	IsMissing  bool       `json:"isMissing"`
	FieldName  string     `json:"fieldName,omitempty"`
	Children   []NodeInfo `json:"children,omitempty"`
}

// Tag represents a definition tag (function, class, struct, etc.).
type Tag struct {
	Kind      string `json:"kind"`
	Name      string `json:"name"`
	StartByte uint32 `json:"startByte"`
	EndByte   uint32 `json:"endByte"`
	StartLine uint32 `json:"startLine"`
	EndLine   uint32 `json:"endLine"`
	StartCol  uint32 `json:"startCol"`
	EndCol    uint32 `json:"endCol"`
}

// QueryMatch represents a tree-sitter query match result.
type QueryMatch struct {
	Pattern  int                  `json:"pattern"`
	Captures map[string][]NodeInfo `json:"captures"`
}

// ValidationResult holds syntax validation results.
type ValidationResult struct {
	Valid           bool          `json:"valid"`
	SyntaxErrors    []SyntaxError `json:"syntaxErrors,omitempty"`
	LineEnding      string        `json:"lineEnding"`
	TrailingNewline bool          `json:"trailingNewline"`
	SourceSize      int           `json:"sourceSize"`
	Language        string        `json:"language"`
}

// SyntaxError describes an error or missing node in the syntax tree.
type SyntaxError struct {
	Type     string `json:"type"` // "error" or "missing"
	StartRow uint32 `json:"startRow"`
	StartCol uint32 `json:"startCol"`
	EndRow   uint32 `json:"endRow"`
	EndCol   uint32 `json:"endCol"`
}

// AnalysisResult holds complexity and structure analysis for a file.
type AnalysisResult struct {
	Cyclomatic int `json:"cyclomatic"`
	Cognitive  int `json:"cognitive"`
	LoopDepth  int `json:"loopDepth"`
	LoopCount  int `json:"loopCount"`
	ParamCount int `json:"paramCount"`
	Recursive  bool `json:"recursive"`
}

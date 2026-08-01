// Package mcp — file structure & AST tools: get_structure, get_definitions,
// get_text, validate.
package mcp

import (
	"context"
	"fmt"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/nanjj/clog"
	"github.com/nanjj/slingshot/internal/code/lsp"
)

// ─── Argument structs ──────────────────────────────────────────────────────────

type getStructureArgs struct {
	File        string `json:"file"`
	MaxDepth    int    `json:"maxDepth,omitempty"`
	MaxChildren int    `json:"maxChildren,omitempty"`
}

type getDefinitionsArgs struct {
	File    string `json:"file"`
	Pattern string `json:"pattern,omitempty"`
	Kind    string `json:"kind,omitempty"`
}

type editorGetTextArgs struct {
	File      string `json:"file"`
	By        string `json:"by,omitempty"` // range, line
	StartByte uint32 `json:"startByte,omitempty"`
	EndByte   uint32 `json:"endByte,omitempty"`
	Line      uint32 `json:"line,omitempty"`
}

type validateArgs struct {
	File string `json:"file"`
}

// ─── get_structure ─────────────────────────────────────────────────────────────

func registerGetStructure(srv *mcp.Server, s *Server) {
	mcp.AddTool(srv, &mcp.Tool{
		Name:        "get_structure",
		Description: "Get the hierarchical code structure (syntax tree) of a file. Returns nested NodeInfo with type, byte range, and child nodes. Use maxDepth and maxChildren to limit output size.",
		InputSchema: strictSchema[getStructureArgs](),
	}, func(ctx context.Context, req *mcp.CallToolRequest, args getStructureArgs) (*mcp.CallToolResult, any, error) {
		return s.handleGetStructure(ctx, args), nil, nil
	})
}

func (s *Server) handleGetStructure(ctx context.Context, args getStructureArgs) *mcp.CallToolResult {
	span, ctx := clog.StartSpanFromContext(ctx, "get_structure")
	defer span.Finish()
	clog.Info(ctx, "get_structure", "file", args.File, "maxDepth", args.MaxDepth)

	if args.File == "" {
		return errorResult(fmt.Errorf("file is required"))
	}

	maxDepth := args.MaxDepth
	if maxDepth == 0 {
		maxDepth = -1 // zero = not specified, default to recursive
	}
	maxChildren := args.MaxChildren
	if maxChildren == 0 {
		maxChildren = -1
	}

	info, err := s.analyzer.GetStructure(args.File, maxDepth, maxChildren)
	if err != nil {
		clog.Error(ctx, "error", "error", err.Error())
		return errorResult(fmt.Errorf("get structure: %w", err))
	}

	clog.Info(ctx, "get_structure_result", "children", countTopLevel(info))
	return jsonResult(info)
}

// ─── get_definitions ───────────────────────────────────────────────────────────

func registerGetDefinitions(srv *mcp.Server, s *Server) {
	mcp.AddTool(srv, &mcp.Tool{
		Name:        "get_definitions",
		Description: "Get all definition tags from a file. Returns functions, methods, classes, structs, interfaces, etc. Optionally filter by name pattern and kind.",
		InputSchema: strictSchema[getDefinitionsArgs](),
	}, func(ctx context.Context, req *mcp.CallToolRequest, args getDefinitionsArgs) (*mcp.CallToolResult, any, error) {
		return s.handleGetDefinitions(ctx, args), nil, nil
	})
}

func (s *Server) handleGetDefinitions(ctx context.Context, args getDefinitionsArgs) *mcp.CallToolResult {
	span, ctx := clog.StartSpanFromContext(ctx, "get_definitions")
	defer span.Finish()
	clog.Info(ctx, "get_definitions", "file", args.File, "pattern", args.Pattern, "kind", args.Kind)

	if args.File == "" {
		return errorResult(fmt.Errorf("file is required"))
	}

	tags, err := s.analyzer.GetDefinitions(args.File)
	if err != nil {
		clog.Error(ctx, "error", "error", err.Error())
		if strings.Contains(err.Error(), "no tags query") || strings.Contains(err.Error(), "unsupported language") {
			return errorResult(fmt.Errorf("get definitions: %v — this language has no tree-sitter tags query; use get_structure for the syntax tree or search_code for text search", err))
		}
		return errorResult(fmt.Errorf("get definitions: %w", err))
	}
	if tags == nil {
		tags = []lsp.Tag{}
	}

	// Apply filters if specified
	if args.Pattern != "" || args.Kind != "" {
		var filtered []lsp.Tag
		for _, t := range tags {
			if args.Pattern != "" && !strings.Contains(t.Name, args.Pattern) {
				continue
			}
			if args.Kind != "" && t.Kind != args.Kind && t.Kind != "definition."+args.Kind {
				continue
			}
			filtered = append(filtered, t)
		}
		tags = filtered
	}

	return jsonResult(tags)
}

// ─── get_text ──────────────────────────────────────────────────────────────────

func registerGetText(srv *mcp.Server, s *Server) {
	mcp.AddTool(srv, &mcp.Tool{
		Name:        "get_text",
		Description: "Get source text from a file. by='range' (default) in byte range [startByte, endByte); by='line' for a specific line (0-indexed, trailing newline stripped).",
		InputSchema: strictSchema[editorGetTextArgs](),
	}, func(ctx context.Context, req *mcp.CallToolRequest, args editorGetTextArgs) (*mcp.CallToolResult, any, error) {
		return s.handleGetText(ctx, args), nil, nil
	})
}

func (s *Server) handleGetText(ctx context.Context, args editorGetTextArgs) *mcp.CallToolResult {
	span, ctx := clog.StartSpanFromContext(ctx, "get_text")
	defer span.Finish()
	clog.Info(ctx, "get_text", "file", args.File, "by", args.By)

	if args.File == "" {
		return errorResult(fmt.Errorf("file is required"))
	}

	switch args.By {
	case "line":
		text, err := s.analyzer.GetLine(args.File, args.Line)
		if err != nil {
			clog.Error(ctx, "error", "error", err.Error())
			return errorResult(fmt.Errorf("get line: %w", err))
		}
		return jsonResult(map[string]string{"text": text})
	default: // "range"
		if args.StartByte == 0 && args.EndByte == 0 {
			// Read full file
			result, err := s.analyzer.ParseFile(args.File)
			if err != nil {
				clog.Error(ctx, "error", "error", err.Error())
				return errorResult(fmt.Errorf("parse file: %w", err))
			}
			result.Tree.Release()
			return jsonResult(map[string]string{"text": string(result.Source)})
		}
		text, err := s.analyzer.GetText(args.File, args.StartByte, args.EndByte)
		if err != nil {
			clog.Error(ctx, "error", "error", err.Error())
			return errorResult(fmt.Errorf("get text: %w", err))
		}
		return jsonResult(map[string]string{"text": text})
	}
}

// ─── validate ──────────────────────────────────────────────────────────────────

func registerValidate(srv *mcp.Server, s *Server) {
	mcp.AddTool(srv, &mcp.Tool{
		Name:        "validate",
		Description: "Validate a file's syntax. Returns syntax errors, line ending style, and trailing newline status.",
		InputSchema: strictSchema[validateArgs](),
	}, func(ctx context.Context, req *mcp.CallToolRequest, args validateArgs) (*mcp.CallToolResult, any, error) {
		return s.handleValidate(ctx, args), nil, nil
	})
}

func (s *Server) handleValidate(ctx context.Context, args validateArgs) *mcp.CallToolResult {
	span, ctx := clog.StartSpanFromContext(ctx, "validate")
	defer span.Finish()
	clog.Info(ctx, "validate", "file", args.File)

	if args.File == "" {
		return errorResult(fmt.Errorf("file is required"))
	}

	result, err := s.analyzer.Validate(args.File)
	if err != nil {
		clog.Error(ctx, "error", "error", err.Error())
		return errorResult(fmt.Errorf("validate: %w", err))
	}

	clog.Info(ctx, "validate_result", "errors", len(result.SyntaxErrors))
	return jsonResult(result)
}

// ─── Helper ────────────────────────────────────────────────────────────────────

// countTopLevel returns the number of top-level children in a tree or 0.
func countTopLevel(v any) int {
	m, ok := v.(map[string]any)
	if !ok {
		return 0
	}
	children, ok := m["children"].([]any)
	if !ok {
		return 0
	}
	return len(children)
}

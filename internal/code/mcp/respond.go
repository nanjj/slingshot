// Package mcp provides MCP tool handlers for code intelligence.
//
// It connects the base (storage/graph) and lsp (analysis) packages
// to the Model Context Protocol, exposing 27 tools for code search,
// navigation, editing, and project management.
package mcp

import (
	"encoding/json"
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// jsonResult marshals v to JSON and wraps it in a successful MCP result.
func jsonResult(v any) *mcp.CallToolResult {
	data, err := json.Marshal(v)
	if err != nil {
		return errorResult(fmt.Errorf("marshal result: %w", err))
	}
	return &mcp.CallToolResult{
		Content: []mcp.Content{
			&mcp.TextContent{
				Text: string(data),
			},
		},
	}
}

// errorResult wraps an error in an MCP error result.
func errorResult(err error) *mcp.CallToolResult {
	return &mcp.CallToolResult{
		IsError: true,
		Content: []mcp.Content{
			&mcp.TextContent{
				Text: err.Error(),
			},
		},
	}
}

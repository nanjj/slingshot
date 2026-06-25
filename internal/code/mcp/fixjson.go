// Package mcp provides MCP tool handlers for code intelligence.
package mcp

import (
	"context"
	"encoding/json"
	"log/slog"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// FixDoubleEncodedJSON is MCP receiving middleware that tolerates LLM
// double-encoded JSON strings in tool call arguments.
//
// Some LLMs occasionally send a JSON string where the schema expects a JSON
// object, e.g. sending:
//
//	{"selector": "{\"path\":[...]}", ...}
//
// instead of:
//
//	{"selector": {"path":[...]}, ...}
//
// This middleware walks the arguments map. For each string value that can be
// JSON-unmarshaled into a map (and not a scalar), it replaces the string with
// the parsed object. This fixes the most common LLM encoding mistake without
// risking false positives on normal string parameters.
func FixDoubleEncodedJSON(next mcp.MethodHandler) mcp.MethodHandler {
	return func(ctx context.Context, method string, req mcp.Request) (mcp.Result, error) {
		if method == "tools/call" {
			if callReq, ok := req.(*mcp.ServerRequest[*mcp.CallToolParamsRaw]); ok {
				if len(callReq.Params.Arguments) > 0 {
					fixed, changed, err := fixDoubleEncodedArgs(callReq.Params.Arguments)
					if err != nil {
						// If we can't parse the arguments at all, let the normal
						// validation pipeline produce the proper error.
						slog.Warn("fix_double_encoded_json: failed to parse arguments", "error", err)
					} else if changed {
						callReq.Params.Arguments = fixed
						slog.Debug("fix_double_encoded_json: repaired double-encoded JSON arguments",
							"tool", callReq.Params.Name)
					}
				}
			}
		}
		return next(ctx, method, req)
	}
}

// fixDoubleEncodedArgs unmarshals the raw arguments, walks all values, and
// for any string value that can be JSON-unmarshaled into a map[string]any,
// replaces the string with the parsed value. Returns the (possibly modified)
// JSON, a boolean indicating whether any change was made, and any error
// encountered during unmarshaling/marshaling.
func fixDoubleEncodedArgs(raw json.RawMessage) (json.RawMessage, bool, error) {
	var args map[string]any
	if err := json.Unmarshal(raw, &args); err != nil {
		return raw, false, err
	}

	changed := false
	for k, v := range args {
		fixed, ok := tryUnmarshalJSONStr(v)
		if ok {
			args[k] = fixed
			changed = true
		}
	}

	if !changed {
		return raw, false, nil
	}

	fixed, err := json.Marshal(args)
	if err != nil {
		return raw, false, err
	}
	return fixed, true, nil
}

// tryUnmarshalJSONStr attempts to JSON-unmarshal a string value. If the value
// is a string containing valid JSON that unmarshals into a map[string]any
// (i.e. a JSON object, not a scalar), it returns the parsed value and true.
// Otherwise it returns the original value and false.
//
// The rationale for only accepting map objects: scalar JSON strings, numbers,
// booleans, and arrays don't make sense as replacements for object-type
// schema fields. Only JSON objects are ambiguous with string fields that
// happen to contain JSON-looking text.
func tryUnmarshalJSONStr(v any) (any, bool) {
	s, ok := v.(string)
	if !ok || s == "" {
		return v, false
	}

	var parsed any
	if err := json.Unmarshal([]byte(s), &parsed); err != nil {
		return v, false
	}

	if _, ok := parsed.(map[string]any); ok {
		return parsed, true
	}
	return v, false
}

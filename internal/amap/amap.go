// Package amap provides a Go client for the Amap (高德地图) MCP server.
//
// The Amap MCP server is a stateless JSON-RPC 2.0 endpoint over
// Streamable HTTP. Every call is an independent POST to:
//
//	https://mcp.amap.com/mcp?key=<KEY>
//
// with body:
//
//	{"jsonrpc":"2.0","id":1,"method":"tools/call",
//	 "params":{"name":"<tool>","arguments":{...}}}
//
// The response carries result.content[].text, which is itself a JSON
// string (occasionally plain text) containing the Amap API result.
//
// The API key is resolved by ResolveKey: env AMAP_KEY first, then the
// "amap.key" config key (set via `slingshot config set amap.key <key>`).
package amap

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/nanjj/slingshot/internal/config"
)

// DefaultURL is the Amap MCP endpoint. A variable so tests can override it.
var DefaultURL = "https://mcp.amap.com/mcp"

// MCP tool names exposed by the Amap MCP server (verified 2026-08-23).
const (
	ToolTextSearch         = "maps_text_search"
	ToolAroundSearch       = "maps_around_search"
	ToolSearchDetail       = "maps_search_detail"
	ToolGeo                = "maps_geo"
	ToolRegeocode          = "maps_regeocode"
	ToolDirectionDriving   = "maps_direction_driving"
	ToolDirectionWalking   = "maps_direction_walking"
	ToolDirectionBicycling = "maps_direction_bicycling"
	ToolDirectionTransit   = "maps_direction_transit_integrated"
	ToolDistance           = "maps_distance"
	ToolIPLocation         = "maps_ip_location"
)

// DefaultTimeout mirrors the curl -m 40 used by the reference script.
const DefaultTimeout = 40 * time.Second

// Client is a stateless Amap MCP client.
type Client struct {
	Key  string
	URL  string
	HTTP *http.Client
}

// NewClient creates a client with the given API key.
func NewClient(key string) *Client {
	return &Client{
		Key:  key,
		URL:  DefaultURL,
		HTTP: &http.Client{Timeout: DefaultTimeout},
	}
}

// request is one JSON-RPC tools/call message.
type request struct {
	JSONRPC string        `json:"jsonrpc"`
	ID      int           `json:"id"`
	Method  string        `json:"method"`
	Params  requestParams `json:"params"`
}

type requestParams struct {
	Name      string         `json:"name"`
	Arguments map[string]any `json:"arguments"`
}

// response is the JSON-RPC response envelope.
type response struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      int             `json:"id"`
	Result  *responseResult `json:"result,omitempty"`
	Error   *responseError  `json:"error,omitempty"`
}

type responseResult struct {
	Content []responseContent `json:"content"`
}

type responseContent struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

type responseError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// Call invokes the named tool and returns the decoded payload: the JSON
// carried in result.content[].text (usually map[string]any), or the plain
// text itself when the payload is not JSON.
func (c *Client) Call(ctx context.Context, tool string, args map[string]any) (any, error) {
	if tool == "" {
		return nil, errors.New("empty tool name")
	}
	body, err := json.Marshal(request{
		JSONRPC: "2.0",
		ID:      1,
		Method:  "tools/call",
		Params:  requestParams{Name: tool, Arguments: args},
	})
	if err != nil {
		return nil, fmt.Errorf("marshaling request: %w", err)
	}

	url := c.URL
	if url == "" {
		url = DefaultURL
	}
	sep := "?"
	if strings.Contains(url, "?") {
		sep = "&"
	}
	url += sep + "key=" + c.Key
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("building request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")

	httpc := c.HTTP
	if httpc == nil {
		httpc = &http.Client{Timeout: DefaultTimeout}
	}
	resp, err := httpc.Do(req)
	if err != nil {
		return nil, fmt.Errorf("HTTP request failed: %w", err)
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, truncate(string(raw), 300))
	}

	r, err := decodeBody(raw)
	if err != nil {
		return nil, err
	}
	if r.Error != nil {
		return nil, fmt.Errorf("MCP error (code %d): %s", r.Error.Code, r.Error.Message)
	}
	if r.Result == nil || len(r.Result.Content) == 0 {
		return nil, nil
	}
	// Amap returns a single text item; pick the first text-type item.
	var text string
	for _, item := range r.Result.Content {
		if item.Type == "text" || item.Type == "" {
			text = item.Text
			break
		}
	}
	if text == "" {
		return nil, nil
	}
	return parsePayload(text)
}

// parsePayload decodes the tool result payload. The payload is normally a
// JSON string; non-JSON content is returned verbatim as a string.
func parsePayload(text string) (any, error) {
	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		return "", nil
	}
	var v any
	dec := json.NewDecoder(strings.NewReader(trimmed))
	dec.UseNumber() // preserve numeric precision (ids, coordinates)
	if err := dec.Decode(&v); err != nil {
		return text, nil // plain text fallback
	}
	return v, nil
}

// decodeBody parses the HTTP body. The server responds with application/json
// in the verified setup, but streamable HTTP may also frame the same JSON
// as SSE (event:/data: lines), so both are tolerated.
func decodeBody(raw []byte) (*response, error) {
	trimmed := bytes.TrimSpace(raw)
	var r response
	if err := json.Unmarshal(trimmed, &r); err == nil {
		return &r, nil
	}
	data := extractSSEData(string(trimmed))
	if data == "" {
		return nil, fmt.Errorf("decoding response: not JSON and no SSE data frames")
	}
	if err := json.Unmarshal([]byte(data), &r); err != nil {
		return nil, fmt.Errorf("decoding response: %w", err)
	}
	return &r, nil
}

// extractSSEData joins the payload of data: frames in an SSE stream.
func extractSSEData(s string) string {
	var parts []string
	for _, line := range strings.Split(s, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "data:") {
			parts = append(parts, strings.TrimSpace(strings.TrimPrefix(line, "data:")))
		}
	}
	if len(parts) == 0 {
		return ""
	}
	return strings.Join(parts, "\n")
}

// ResolveKey returns the Amap API key: env AMAP_KEY first, then the
// "amap.key" config key. Returns an actionable error when both are missing.
func ResolveKey(cfg *config.Config) (string, error) {
	if k := strings.TrimSpace(os.Getenv("AMAP_KEY")); k != "" {
		return k, nil
	}
	raw, err := config.Get(cfg, "amap.key")
	if err == nil {
		if s, ok := raw.(string); ok && strings.TrimSpace(s) != "" {
			return strings.TrimSpace(s), nil
		}
	}
	return "", errors.New("AMAP_KEY env var not set and amap.key not found in config; configure with: slingshot config set amap.key <key>")
}

// LocationFromGeo extracts "经度,纬度" from a maps_geo result payload.
// The payload may use "geocodes" (Web API shape) or "results".
func LocationFromGeo(v any) (string, bool) {
	m, ok := v.(map[string]any)
	if !ok {
		return "", false
	}
	for _, key := range []string{"results", "geocodes"} {
		arr, ok := m[key].([]any)
		if !ok || len(arr) == 0 {
			continue
		}
		first, ok := arr[0].(map[string]any)
		if !ok {
			continue
		}
		if loc, ok := first["location"].(string); ok && strings.TrimSpace(loc) != "" {
			return strings.TrimSpace(loc), true
		}
	}
	return "", false
}

// truncate limits s to maxLen runes for error messages.
func truncate(s string, maxLen int) string {
	runes := []rune(s)
	if len(runes) <= maxLen {
		return s
	}
	return string(runes[:maxLen]) + "..."
}

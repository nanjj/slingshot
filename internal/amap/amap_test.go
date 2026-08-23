package amap

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/nanjj/slingshot/internal/config"
)

// testServer starts an httptest server whose handler records the request
// (headers, URL) and request body, and responds with the given body/status.
// The second return value returns the recorded request and body (safe after
// the call under test, since the body bytes are captured before close).
func testServer(t *testing.T, status int, body string) (*httptest.Server, func() (*http.Request, []byte)) {
	t.Helper()
	var got *http.Request
	var gotBody []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = r
		gotBody, _ = io.ReadAll(io.LimitReader(r.Body, 1<<20))
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		io.WriteString(w, body)
	}))
	t.Cleanup(srv.Close)
	return srv, func() (*http.Request, []byte) { return got, gotBody }
}

func TestCallSuccess(t *testing.T) {
	srv, _ := testServer(t, http.StatusOK,
		`{"jsonrpc":"2.0","id":1,"result":{"content":[{"type":"text","text":"{\"status\":\"1\",\"count\":2,\"pois\":[]}"}]}}`)
	c := NewClient("test-key")
	c.URL = srv.URL

	v, err := c.Call(context.Background(), ToolTextSearch, map[string]any{"keywords": "冰煮羊", "citylimit": true})
	if err != nil {
		t.Fatalf("Call: %v", err)
	}
	m, ok := v.(map[string]any)
	if !ok {
		t.Fatalf("expected map, got %T", v)
	}
	if m["status"] != "1" || m["count"] != json.Number("2") {
		t.Fatalf("unexpected payload: %#v", m)
	}
}

func TestCallSendsToolNameAndArgs(t *testing.T) {
	srv, getReq := testServer(t, http.StatusOK,
		`{"jsonrpc":"2.0","id":1,"result":{"content":[{"type":"text","text":"{}"}]}}`)
	c := NewClient("test-key")
	c.URL = srv.URL

	if _, err := c.Call(context.Background(), ToolGeo, map[string]any{"address": "呼和浩特东站"}); err != nil {
		t.Fatalf("Call: %v", err)
	}
	req, reqBody := getReq()
	if got := req.URL.Query().Get("key"); got != "test-key" {
		t.Fatalf("URL missing key: %s (got %q)", req.URL, got)
	}
	if req.Header.Get("Content-Type") != "application/json" {
		t.Errorf("Content-Type = %q", req.Header.Get("Content-Type"))
	}
	if req.Header.Get("Accept") != "application/json, text/event-stream" {
		t.Errorf("Accept = %q", req.Header.Get("Accept"))
	}
	var body struct {
		JSONRPC string `json:"jsonrpc"`
		ID      int    `json:"id"`
		Method  string `json:"method"`
		Params  struct {
			Name      string         `json:"name"`
			Arguments map[string]any `json:"arguments"`
		} `json:"params"`
	}
	if err := json.NewDecoder(bytes.NewReader(reqBody)).Decode(&body); err != nil {
		t.Fatalf("decode request body: %v", err)
	}
	if body.JSONRPC != "2.0" || body.ID != 1 || body.Method != "tools/call" {
		t.Fatalf("unexpected envelope: %+v", body)
	}
	if body.Params.Name != ToolGeo {
		t.Errorf("tool name = %q", body.Params.Name)
	}
	if body.Params.Arguments["address"] != "呼和浩特东站" {
		t.Errorf("arguments = %#v", body.Params.Arguments)
	}
}

func TestCallError(t *testing.T) {
	srv, _ := testServer(t, http.StatusOK,
		`{"jsonrpc":"2.0","id":1,"error":{"code":-32000,"message":"invalid key"}}`)
	c := NewClient("test-key")
	c.URL = srv.URL

	_, err := c.Call(context.Background(), ToolGeo, nil)
	if err == nil || !strings.Contains(err.Error(), "invalid key") || !strings.Contains(err.Error(), "-32000") {
		t.Fatalf("expected MCP error, got: %v", err)
	}
}

func TestCallSSE(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		io.WriteString(w, "event: message\ndata: {\"jsonrpc\":\"2.0\",\"id\":1,\"result\":{\"content\":[{\"type\":\"text\",\"text\":\"{\\\"ok\\\":true}\"}]}}\n\n")
	}))
	defer srv.Close()
	c := NewClient("test-key")
	c.URL = srv.URL

	v, err := c.Call(context.Background(), ToolGeo, nil)
	if err != nil {
		t.Fatalf("Call: %v", err)
	}
	if m := v.(map[string]any); m["ok"] != true {
		t.Fatalf("unexpected payload: %#v", m)
	}
}

func TestCallPlainTextPayload(t *testing.T) {
	srv, _ := testServer(t, http.StatusOK,
		`{"jsonrpc":"2.0","id":1,"result":{"content":[{"type":"text","text":"just a message"}]}}`)
	c := NewClient("test-key")
	c.URL = srv.URL

	v, err := c.Call(context.Background(), ToolGeo, nil)
	if err != nil {
		t.Fatalf("Call: %v", err)
	}
	if v != "just a message" {
		t.Fatalf("expected plain text, got %#v", v)
	}
}

func TestCallEmptyContent(t *testing.T) {
	srv, _ := testServer(t, http.StatusOK, `{"jsonrpc":"2.0","id":1,"result":{}}`)
	c := NewClient("test-key")
	c.URL = srv.URL

	v, err := c.Call(context.Background(), ToolGeo, nil)
	if err != nil {
		t.Fatalf("Call: %v", err)
	}
	if v != nil {
		t.Fatalf("expected nil, got %#v", v)
	}
}

func TestCallHTTPError(t *testing.T) {
	srv, _ := testServer(t, http.StatusInternalServerError, "oops")
	c := NewClient("test-key")
	c.URL = srv.URL

	_, err := c.Call(context.Background(), ToolGeo, nil)
	if err == nil || !strings.Contains(err.Error(), "500") {
		t.Fatalf("expected HTTP error, got: %v", err)
	}
}

func TestCallBadBody(t *testing.T) {
	srv, _ := testServer(t, http.StatusOK, "not json at all")
	c := NewClient("test-key")
	c.URL = srv.URL

	_, err := c.Call(context.Background(), ToolGeo, nil)
	if err == nil {
		t.Fatal("expected decoding error")
	}
}

func TestCallEmptyTool(t *testing.T) {
	c := NewClient("test-key")
	if _, err := c.Call(context.Background(), "", nil); err == nil {
		t.Fatal("expected error for empty tool")
	}
}

func TestCallURLWithExistingQuery(t *testing.T) {
	// URL already carrying a query string must keep it (key appended with &).
	srv, getReq := testServer(t, http.StatusOK,
		`{"jsonrpc":"2.0","id":1,"result":{"content":[{"type":"text","text":"{}"}]}}`)
	c := NewClient("test-key")
	c.URL = srv.URL + "?api-version=1"

	if _, err := c.Call(context.Background(), ToolGeo, nil); err != nil {
		t.Fatalf("Call: %v", err)
	}
	req, _ := getReq()
	if got := req.URL.Query().Get("key"); got != "test-key" {
		t.Errorf("key = %q", got)
	}
	if got := req.URL.Query().Get("api-version"); got != "1" {
		t.Errorf("existing query lost: api-version = %q", got)
	}
}

func TestLocationFromGeo(t *testing.T) {
	// geocodes shape (Web API v3)
	geocodes := map[string]any{
		"geocodes": []any{map[string]any{"location": "111.772234,40.853779"}},
	}
	if loc, ok := LocationFromGeo(geocodes); !ok || loc != "111.772234,40.853779" {
		t.Fatalf("geocodes: got %q, %v", loc, ok)
	}
	// results shape (MCP maps_geo)
	results := map[string]any{
		"results": []any{map[string]any{"location": "111.772234,40.853779"}},
	}
	if loc, ok := LocationFromGeo(results); !ok || loc != "111.772234,40.853779" {
		t.Fatalf("results: got %q, %v", loc, ok)
	}
	// empty / wrong shapes
	if _, ok := LocationFromGeo(map[string]any{}); ok {
		t.Fatal("expected no location for empty payload")
	}
	if _, ok := LocationFromGeo("not a map"); ok {
		t.Fatal("expected no location for non-map payload")
	}
}

func TestResolveKey(t *testing.T) {
	t.Setenv("AMAP_KEY", "")
	cfg := &config.Config{Extra: map[string]any{
		"amap": map[string]any{"key": "config-key"},
	}}
	if k, err := ResolveKey(cfg); err != nil || k != "config-key" {
		t.Fatalf("config key: got %q, %v", k, err)
	}

	t.Setenv("AMAP_KEY", "env-key")
	if k, err := ResolveKey(cfg); err != nil || k != "env-key" {
		t.Fatalf("env priority: got %q, %v", k, err)
	}

	// env empty, config missing → error with config hint
	t.Setenv("AMAP_KEY", "")
	empty := &config.Config{}
	_, err := ResolveKey(empty)
	if err == nil || !strings.Contains(err.Error(), "config set amap.key") {
		t.Fatalf("expected actionable error, got: %v", err)
	}
}

func TestResolveKeyEnvEmptyString(t *testing.T) {
	// env var set but blank should fall through to config
	t.Setenv("AMAP_KEY", "   ")
	cfg := &config.Config{Extra: map[string]any{
		"amap": map[string]any{"key": "config-key"},
	}}
	if k, err := ResolveKey(cfg); err != nil || k != "config-key" {
		t.Fatalf("blank env should fall back to config: got %q, %v", k, err)
	}
}

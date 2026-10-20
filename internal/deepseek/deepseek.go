// Package deepseek provides a Go client for the DeepSeek Search and Fetch
// tools API.
//
// Both endpoints are plain JSON over HTTPS with Bearer authentication:
//
//	POST https://api.deepseek.com/v1/tools/search
//	POST https://api.deepseek.com/v1/tools/fetch
//
// The API key is resolved by ResolveKey: env DEEPSEEK_API_KEY first, then
// the "deepseek.key" config key (set via `slingshot config set deepseek.key <key>`).
package deepseek

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

// DefaultBaseURL is the DeepSeek API base. A variable so tests can override it.
var DefaultBaseURL = "https://api.deepseek.com"

// DefaultTimeout is the per-request timeout.
const DefaultTimeout = 40 * time.Second

// Client is a stateless DeepSeek tools client.
type Client struct {
	Key  string
	Base string
	HTTP *http.Client
}

// NewClient creates a client with the given API key.
func NewClient(key string) *Client {
	return &Client{
		Key:  key,
		Base: DefaultBaseURL,
		HTTP: &http.Client{Timeout: DefaultTimeout},
	}
}

// SearchRequest is the body of POST /v1/tools/search.
type SearchRequest struct {
	Query        string `json:"query"`
	Count        int    `json:"count,omitempty"`
	OriQuestion  string `json:"ori_question,omitempty"`
	Localization string `json:"localization,omitempty"`
	NeedContent  bool   `json:"need_content,omitempty"`
}

// SearchResult is one entry in the search response.
type SearchResult struct {
	RerankScore float64 `json:"rerank_score"`
	Title       string  `json:"title"`
	URL         string  `json:"url"`
	Snippet     string  `json:"snippet"`
	Summary     string  `json:"summary"`
	Content     string  `json:"content"`
	SiteName    *string `json:"site_name"`
	SiteIcon    *string `json:"site_icon"`
	Date        *string `json:"date"`
	Authority   int     `json:"authority"`
}

// SearchResponse is the response of POST /v1/tools/search.
type SearchResponse struct {
	Results []SearchResult `json:"results"`
}

// FetchRequest is the body of POST /v1/tools/fetch.
type FetchRequest struct {
	URL string `json:"url"`
}

// FetchResponse is the response of POST /v1/tools/fetch.
type FetchResponse struct {
	Title   string `json:"title"`
	Content string `json:"content"`
}

// apiError is the OpenAI-compatible error envelope.
type apiError struct {
	Error struct {
		Message string `json:"message"`
		Type    string `json:"type"`
		Param   any    `json:"param"`
		Code    string `json:"code"`
	} `json:"error"`
}

// post sends a JSON request and decodes the JSON response into out.
func (c *Client) post(ctx context.Context, path string, body any, out any) error {
	data, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("marshaling request: %w", err)
	}

	base := c.Base
	if base == "" {
		base = DefaultBaseURL
	}
	url := strings.TrimRight(base, "/") + path

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("building request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.Key)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.HTTP.Do(req)
	if err != nil {
		return fmt.Errorf("calling %s: %w", path, err)
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("reading response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		var ae apiError
		if json.Unmarshal(raw, &ae) == nil && ae.Error.Message != "" {
			return fmt.Errorf("%s: HTTP %d: %s", path, resp.StatusCode, ae.Error.Message)
		}
		return fmt.Errorf("%s: HTTP %d: %s", path, resp.StatusCode, strings.TrimSpace(string(raw)))
	}

	if err := json.Unmarshal(raw, out); err != nil {
		return fmt.Errorf("decoding response: %w", err)
	}
	return nil
}

// Search calls POST /v1/tools/search.
func (c *Client) Search(ctx context.Context, req SearchRequest) (*SearchResponse, error) {
	if strings.TrimSpace(req.Query) == "" {
		return nil, errors.New("search: empty query")
	}
	resp := &SearchResponse{}
	if err := c.post(ctx, "/v1/tools/search", req, resp); err != nil {
		return nil, err
	}
	return resp, nil
}

// Fetch calls POST /v1/tools/fetch.
func (c *Client) Fetch(ctx context.Context, url string) (*FetchResponse, error) {
	if strings.TrimSpace(url) == "" {
		return nil, errors.New("fetch: empty url")
	}
	resp := &FetchResponse{}
	if err := c.post(ctx, "/v1/tools/fetch", FetchRequest{URL: url}, resp); err != nil {
		return nil, err
	}
	return resp, nil
}

// ResolveKey returns the DeepSeek API key: env DEEPSEEK_API_KEY first, then
// the "deepseek.key" config key. Returns an actionable error when both are missing.
func ResolveKey(cfg *config.Config) (string, error) {
	if k := strings.TrimSpace(os.Getenv("DEEPSEEK_API_KEY")); k != "" {
		return k, nil
	}
	raw, err := config.Get(cfg, "deepseek.key")
	if err == nil {
		if s, ok := raw.(string); ok && strings.TrimSpace(s) != "" {
			return strings.TrimSpace(s), nil
		}
	}
	return "", errors.New("DEEPSEEK_API_KEY env var not set and deepseek.key not found in config; configure with: slingshot config set deepseek.key <key>")
}

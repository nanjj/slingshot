package deepseek

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/chromedp/chromedp"

	"github.com/nanjj/slingshot/internal/config"
)

// searchLocalTimeout bounds a single Google search via remote CDP. A
// cold connection plus page load plus extraction fits well inside this.
const searchLocalTimeout = 90 * time.Second

// searchLocalWait is how long to let the results page settle after
// navigation before reading the DOM.
const searchLocalWait = 3 * time.Second

// SearchLocal runs a Google search through a remote lightpanda CDP
// endpoint and maps the results onto SearchResponse.
//
// The endpoint, token and proxy come from the config keys
// lightpanda.remote.{url,token,proxy}. The proxy is required: without a
// US or otherwise non-EU exit, Google serves a consent page instead of
// results.
func SearchLocal(ctx context.Context, cfg *config.Config, req SearchRequest) (*SearchResponse, error) {
	query := strings.TrimSpace(req.Query)
	if query == "" {
		return nil, errors.New("search local: empty query")
	}

	endpoint, err := resolveCDPEndpoint(cfg)
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(ctx, searchLocalTimeout)
	defer cancel()

	allocCtx, allocCancel := chromedp.NewRemoteAllocator(ctx, endpoint, chromedp.NoModifyURL)
	defer allocCancel()

	tabCtx, tabCancel := chromedp.NewContext(allocCtx)
	defer tabCancel()

	searchURL := "https://www.google.com/search?q=" + url.QueryEscape(query) + "&hl=en&gl=us"

	var raw []map[string]string
	err = chromedp.Run(tabCtx,
		chromedp.Navigate(searchURL),
		chromedp.Sleep(searchLocalWait),
		chromedp.Evaluate(`
			Array.from(document.querySelectorAll('h3')).map(h => {
				var a = h.closest('a');
				var title = h.textContent || '';

				// Google's result-block class names change without notice, so
				// walk up until an ancestor carries enough text to be the
				// result body. Cap the walk so a stray h3 cannot pull in the
				// whole page.
				var block = h.parentElement;
				for (var i = 0; i < 6 && block; i++) {
					if ((block.textContent || '').length > 120) break;
					block = block.parentElement;
				}

				var snip = '';
				if (block) {
					// Known snippet containers first, then the block text with
					// the title removed. textContent, not innerText: hidden
					// knowledge-panel fragments still carry their text.
					var s = block.querySelector('div.VwiC3b, span.aCOpRe, div[data-sncf]');
					if (s) snip = s.textContent || '';
					if (!snip) {
						var text = block.textContent || '';
						if (title && text.indexOf(title) === 0) {
							text = text.slice(title.length);
						}
						snip = text.slice(0, 200);
					}
				}

				return {
					title: title,
					url: a ? a.href : '',
					snippet: snip.replace(/\s+/g, ' ').trim()
				};
			})
		`, &raw),
	)
	if err != nil {
		return nil, fmt.Errorf("search local: %w", err)
	}

	if len(raw) == 0 {
		return nil, errors.New("search local: no results found; the search page may be a consent or captcha page")
	}

	limit := req.Count
	if limit <= 0 || limit > len(raw) {
		limit = len(raw)
	}

	resp := &SearchResponse{Results: make([]SearchResult, 0, limit)}
	for _, item := range raw[:limit] {
		resp.Results = append(resp.Results, SearchResult{
			Title:   item["title"],
			URL:     item["url"],
			Snippet: item["snippet"],
		})
	}

	return resp, nil
}

// resolveCDPEndpoint builds the remote CDP websocket URL from config.
func resolveCDPEndpoint(cfg *config.Config) (string, error) {
	rawURL, err := config.Get(cfg, "lightpanda.remote.url")
	if err != nil {
		return "", errors.New("search local: lightpanda.remote.url not set in config")
	}
	endpoint, ok := rawURL.(string)
	if !ok || strings.TrimSpace(endpoint) == "" {
		return "", errors.New("search local: lightpanda.remote.url is empty")
	}

	sep := "?"
	if strings.Contains(endpoint, "?") {
		sep = "&"
	}

	if raw, err := config.Get(cfg, "lightpanda.remote.token"); err == nil {
		if token, ok := raw.(string); ok && token != "" {
			endpoint += sep + "token=" + token
			sep = "&"
		}
	}

	// A proxy is not optional: without one the search page is a consent
	// wall, and the caller would get an empty result set.
	rawProxy, err := config.Get(cfg, "lightpanda.remote.proxy")
	if err != nil {
		return "", errors.New("search local: lightpanda.remote.proxy not set in config (a non-EU exit is required to skip Google's consent page)")
	}
	proxy, ok := rawProxy.(string)
	if !ok || strings.TrimSpace(proxy) == "" {
		return "", errors.New("search local: lightpanda.remote.proxy is empty")
	}
	endpoint += sep + "proxy=" + proxy

	return endpoint, nil
}

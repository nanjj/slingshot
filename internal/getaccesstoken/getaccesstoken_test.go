package getaccesstoken

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/nanjj/slingshot/internal/config"
)

func TestCachedToken(t *testing.T) {
	t.Run("no_token_in_config", func(t *testing.T) {
		cfg := &config.Config{}
		token, ok := cachedToken(cfg)
		if ok {
			t.Errorf("expected ok=false, got true (token=%q)", token)
		}
	})

	t.Run("valid_token", func(t *testing.T) {
		cfg := &config.Config{Extra: map[string]any{
			"wechat": map[string]any{
				"access_token":     "test_token_123",
				"token_expires_at": fmt.Sprintf("%d", time.Now().Unix()+3600),
			},
		}}
		token, ok := cachedToken(cfg)
		if !ok {
			t.Fatal("expected ok=true, got false")
		}
		if token != "test_token_123" {
			t.Errorf("expected test_token_123, got %q", token)
		}
	})

	t.Run("expired_token", func(t *testing.T) {
		cfg := &config.Config{Extra: map[string]any{
			"wechat": map[string]any{
				"access_token":     "test_token_456",
				"token_expires_at": fmt.Sprintf("%d", time.Now().Unix()-100),
			},
		}}
		token, ok := cachedToken(cfg)
		if ok {
			t.Errorf("expected ok=false (expired), got true (token=%q)", token)
		}
	})

	t.Run("token_within_buffer_zone", func(t *testing.T) {
		// Token expires in 60 seconds - within the 300s buffer, should be refreshed
		cfg := &config.Config{Extra: map[string]any{
			"wechat": map[string]any{
				"access_token":     "test_token_buffer",
				"token_expires_at": fmt.Sprintf("%d", time.Now().Unix()+60),
			},
		}}
		token, ok := cachedToken(cfg)
		if ok {
			t.Errorf("expected ok=false (within buffer), got true (token=%q)", token)
		}
	})
}

func TestRequestToken(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodGet {
				t.Errorf("expected GET, got %s", r.Method)
			}
			if r.URL.Query().Get("grant_type") != "client_credential" {
				t.Errorf("expected grant_type=client_credential")
			}
			if r.URL.Query().Get("appid") != "test_appid" {
				t.Errorf("expected appid=test_appid")
			}
			if r.URL.Query().Get("secret") != "test_secret" {
				t.Errorf("expected secret=test_secret")
			}
			_ = json.NewEncoder(w).Encode(TokenResponse{
				AccessToken: "mock_token_abc",
				ExpiresIn:   7200,
			})
		}))
		defer srv.Close()

		// Temporarily override the TokenURL
		originalURL := TokenURL
		TokenURL = srv.URL
		defer func() { TokenURL = originalURL }()

		token, expiresIn, err := requestToken("test_appid", "test_secret")
		if err != nil {
			t.Fatalf("requestToken() error = %v", err)
		}
		if token != "mock_token_abc" {
			t.Errorf("expected mock_token_abc, got %q", token)
		}
		if expiresIn != 7200 {
			t.Errorf("expected expiresIn=7200, got %d", expiresIn)
		}
	})

	t.Run("api_error", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_ = json.NewEncoder(w).Encode(TokenResponse{
				ErrCode: 40013,
				ErrMsg:  "invalid appid",
			})
		}))
		defer srv.Close()

		originalURL := TokenURL
		TokenURL = srv.URL
		defer func() { TokenURL = originalURL }()

		_, _, err := requestToken("bad_appid", "bad_secret")
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if err.Error() != `WeChat API error (code 40013): invalid appid` {
			t.Errorf("unexpected error: %v", err)
		}
	})

	t.Run("empty_token", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_ = json.NewEncoder(w).Encode(TokenResponse{
				AccessToken: "",
				ExpiresIn:   0,
			})
		}))
		defer srv.Close()

		originalURL := TokenURL
		TokenURL = srv.URL
		defer func() { TokenURL = originalURL }()

		_, _, err := requestToken("appid", "secret")
		if err == nil {
			t.Fatal("expected error for empty token, got nil")
		}
	})
}

func TestGetTokenWithConfig(t *testing.T) {
	t.Run("uses_cached_token", func(t *testing.T) {
		cfg := &config.Config{Extra: map[string]any{
			"wechat": map[string]any{
				"access_token":     "cached_token",
				"token_expires_at": fmt.Sprintf("%d", time.Now().Unix()+3600),
				"appid":            "test_appid",
				"secret":           "test_secret",
			},
		}}

		token, err := GetToken(cfg)
		if err != nil {
			t.Fatalf("GetToken() error = %v", err)
		}
		if token != "cached_token" {
			t.Errorf("expected cached_token, got %q", token)
		}
	})

	t.Run("requests_new_when_expired", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_ = json.NewEncoder(w).Encode(TokenResponse{
				AccessToken: "fresh_token",
				ExpiresIn:   7200,
			})
		}))
		defer srv.Close()

		originalURL := TokenURL
		TokenURL = srv.URL
		defer func() { TokenURL = originalURL }()

		// Save to a temp file instead of the real config
		savePath := t.TempDir() + "/test_config.yml"
		originalSavePath := ConfigSavePath
		ConfigSavePath = savePath
		defer func() { ConfigSavePath = originalSavePath }()

		cfg := &config.Config{Extra: map[string]any{
			"wechat": map[string]any{
				"access_token":     "stale_token",
				"token_expires_at": fmt.Sprintf("%d", time.Now().Unix()-100),
				"appid":            "test_appid",
				"secret":           "test_secret",
			},
		}}

		token, err := GetToken(cfg)
		if err != nil {
			t.Fatalf("GetToken() error = %v", err)
		}
		if token != "fresh_token" {
			t.Errorf("expected fresh_token, got %q", token)
		}

		// Config should have been updated
		raw, err := config.Get(cfg, "wechat.access_token")
		if err != nil {
			t.Fatal("access_token should exist in config after refresh")
		}
		if raw.(string) != "fresh_token" {
			t.Errorf("config should have fresh_token, got %q", raw.(string))
		}

		// Verify the temp config file was saved and contains the token
		savedData, err := os.ReadFile(savePath)
		if err != nil {
			t.Fatalf("expected temp config file to exist: %v", err)
		}
		if !strings.Contains(string(savedData), "fresh_token") {
			t.Errorf("temp config should contain fresh_token, got: %s", string(savedData))
		}
	})
}

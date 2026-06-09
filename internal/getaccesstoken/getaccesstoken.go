// Package getaccesstoken manages WeChat API access tokens with config caching.
//
// Strategy:
//  1. Check config for cached token with expiry timestamp
//  2. If valid (not expired), return cached token
//  3. If expired/missing, request new token from WeChat API
//  4. Cache new token and expiry in config, save config
//  5. Return token
package getaccesstoken

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/nanjj/slingshot/internal/config"
)

// These are made variables (not constants) so that tests can override them.
var (
	// TokenURL is the WeChat API endpoint for access tokens.
	TokenURL = "https://api.weixin.qq.com/cgi-bin/token"
	// DefaultExpiryBuffer is the safety margin (seconds) before actual expiry
	// to proactively refresh the token.
	DefaultExpiryBuffer = 300 // 5 minutes
)

// TokenResponse represents the WeChat access token API response.
type TokenResponse struct {
	AccessToken string `json:"access_token"`
	ExpiresIn   int    `json:"expires_in"`
	ErrCode     int    `json:"errcode,omitempty"`
	ErrMsg      string `json:"errmsg,omitempty"`
}

// config keys for token caching
const (
	cfgKeyToken     = "wechat.access_token"
	cfgKeyExpiresAt = "wechat.token_expires_at"
	cfgKeyAppID     = "wechat.appid"
	cfgKeySecret    = "wechat.secret"
)

// GetToken retrieves a valid WeChat access token.
//
// It first checks the config cache. If the cached token is still valid
// (not expired, with a 5-minute buffer), it returns the cached token.
// Otherwise, it requests a new token from the WeChat API, caches it,
// and returns it.
func GetToken(cfg *config.Config) (string, error) {
	// 1. Try cached token first
	if token, ok := cachedToken(cfg); ok {
		return token, nil
	}

	// 2. Get appid and secret from config
	appid, err := config.Get(cfg, cfgKeyAppID)
	if err != nil {
		return "", fmt.Errorf("wechat.appid not found in config: %w", err)
	}
	secret, err := config.Get(cfg, cfgKeySecret)
	if err != nil {
		return "", fmt.Errorf("wechat.secret not found in config: %w", err)
	}

	// 3. Request new token from WeChat
	token, expiresIn, err := requestToken(appid.(string), secret.(string))
	if err != nil {
		return "", fmt.Errorf("requesting access token: %w", err)
	}

	// 4. Cache in config
	expiresAt := time.Now().Unix() + int64(expiresIn)
	if err := config.Set(cfg, cfgKeyToken, token); err != nil {
		return "", fmt.Errorf("caching access token: %w", err)
	}
	if err := config.Set(cfg, cfgKeyExpiresAt, fmt.Sprintf("%d", expiresAt)); err != nil {
		return "", fmt.Errorf("caching token expiry: %w", err)
	}

	// Save config
	path := config.Path()
	if err := config.Save(cfg, path); err != nil {
		return "", fmt.Errorf("saving config: %w", err)
	}

	return token, nil
}

// cachedToken checks if a valid cached token exists in config.
// Returns the token and true if valid, or "" and false if expired/missing.
func cachedToken(cfg *config.Config) (string, bool) {
	raw, err := config.Get(cfg, cfgKeyToken)
	if err != nil {
		return "", false
	}
	token, ok := raw.(string)
	if !ok || token == "" {
		return "", false
	}

	rawExp, err := config.Get(cfg, cfgKeyExpiresAt)
	if err != nil {
		return "", false
	}
	expiresAt, ok := rawExp.(string)
	if !ok || expiresAt == "" {
		return "", false
	}

	var expInt int64
	if _, err := fmt.Sscanf(expiresAt, "%d", &expInt); err != nil {
		return "", false
	}

	// Check with safety buffer
	now := time.Now().Unix()
	if now+int64(DefaultExpiryBuffer) < expInt {
		return token, true
	}
	return "", false
}

// requestToken calls the WeChat API to get a new access token.
func requestToken(appid, secret string) (string, int, error) {
	url := fmt.Sprintf("%s?grant_type=client_credential&appid=%s&secret=%s",
		TokenURL, appid, secret)

	resp, err := http.Get(url) //nolint:gosec // URL is constructed from trusted config values
	if err != nil {
		return "", 0, fmt.Errorf("HTTP request failed: %w", err)
	}
	defer resp.Body.Close()

	var tr TokenResponse
	if err := json.NewDecoder(resp.Body).Decode(&tr); err != nil {
		return "", 0, fmt.Errorf("decoding response: %w", err)
	}

	if tr.ErrCode != 0 {
		return "", 0, fmt.Errorf("WeChat API error (code %d): %s", tr.ErrCode, tr.ErrMsg)
	}
	if tr.AccessToken == "" {
		return "", 0, fmt.Errorf("empty access token in response")
	}

	return tr.AccessToken, tr.ExpiresIn, nil
}

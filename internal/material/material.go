// Package material provides WeChat permanent material management API operations.
//
// WeChat Material Management (素材管理) API allows managing permanent materials:
//   - List: POST /cgi-bin/material/batchget_material
package material

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

// API endpoints — made variables so tests can override them.
var (
	BatchGetURL = "https://api.weixin.qq.com/cgi-bin/material/batchget_material"
)

// MaterialType represents the type of material in WeChat.
type MaterialType string

const (
	TypeImage MaterialType = "image"
	TypeVideo MaterialType = "video"
	TypeVoice MaterialType = "voice"
	TypeNews  MaterialType = "news"
)

// ListItem represents a single material item in the list response.
type ListItem struct {
	MediaID    string `json:"media_id"`
	Name       string `json:"name"`
	UpdateTime int64  `json:"update_time"`
	URL        string `json:"url,omitempty"`
}

// ListResponse is the response from the material/batchget_material API.
type ListResponse struct {
	TotalCount int        `json:"total_count"`
	ItemCount  int        `json:"item_count"`
	Items      []ListItem `json:"item"`
	ErrCode    int        `json:"errcode,omitempty"`
	ErrMsg     string     `json:"errmsg,omitempty"`
}

// ListRequest is the request body for listing materials.
type ListRequest struct {
	Type   MaterialType `json:"type"`
	Offset int          `json:"offset"`
	Count  int          `json:"count"`
}

// List retrieves a paginated list of permanent materials of the given type.
func List(token string, materialType MaterialType, offset, count int) (*ListResponse, error) {
	req := ListRequest{
		Type:   materialType,
		Offset: offset,
		Count:  count,
	}
	data, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("marshaling request: %w", err)
	}

	url := fmt.Sprintf("%s?access_token=%s", BatchGetURL, token)
	resp, err := http.Post(url, "application/json; charset=utf-8", bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("HTTP request failed: %w", err)
	}
	defer resp.Body.Close()

	return decodeResponse[ListResponse](resp.Body)
}

// wechatError is the common error fields in WeChat API responses.
type wechatError struct {
	ErrCode int    `json:"errcode"`
	ErrMsg  string `json:"errmsg"`
}

// decodeResponse decodes a WeChat API response and checks for errors.
func decodeResponse[T any](r io.Reader) (*T, error) {
	raw, err := io.ReadAll(r)
	if err != nil {
		return nil, fmt.Errorf("reading response: %w", err)
	}

	// Check for WeChat API error first
	var we wechatError
	if err := json.Unmarshal(raw, &we); err == nil && we.ErrCode != 0 {
		return nil, fmt.Errorf("WeChat API error (code %d): %s", we.ErrCode, we.ErrMsg)
	}

	var result T
	if err := json.Unmarshal(raw, &result); err != nil {
		return nil, fmt.Errorf("decoding response: %w", err)
	}
	return &result, nil
}

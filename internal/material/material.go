// Package material provides WeChat permanent material management API operations.
//
// WeChat Material Management (素材管理) API allows managing permanent materials:
//   - List:  POST /cgi-bin/material/batchget_material
//   - Add:   POST /cgi-bin/material/add_material
//   - Get:   POST /cgi-bin/material/get_material (Show)
//   - Del:   POST /cgi-bin/material/del_material (Remove)
package material

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
)

// API endpoints — made variables so tests can override them.
var (
	BatchGetURL = "https://api.weixin.qq.com/cgi-bin/material/batchget_material"
	AddURL      = "https://api.weixin.qq.com/cgi-bin/material/add_material"
	GetURL      = "https://api.weixin.qq.com/cgi-bin/material/get_material"
	DelURL      = "https://api.weixin.qq.com/cgi-bin/material/del_material"
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

// --- Add ---

// AddResponse is the response from the material/add_material API.
type AddResponse struct {
	MediaID string `json:"media_id"`
	URL     string `json:"url,omitempty"`
	ErrCode int    `json:"errcode,omitempty"`
	ErrMsg  string `json:"errmsg,omitempty"`
}

// Add uploads a file as a permanent material of the given type.
// For video type, title and introduction are required.
func Add(token, filePath string, materialType MaterialType, title, introduction string) (*AddResponse, error) {
	f, err := os.Open(filePath)
	if err != nil {
		return nil, fmt.Errorf("opening file %q: %w", filePath, err)
	}
	defer f.Close()

	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)

	// type field
	if err := w.WriteField("type", string(materialType)); err != nil {
		return nil, fmt.Errorf("writing type field: %w", err)
	}

	// media file
	fw, err := w.CreateFormFile("media", filepath.Base(filePath))
	if err != nil {
		return nil, fmt.Errorf("creating form file: %w", err)
	}
	if _, err := io.Copy(fw, f); err != nil {
		return nil, fmt.Errorf("copying file content: %w", err)
	}

	// Video-specific fields
	if materialType == TypeVideo {
		if title == "" {
			return nil, fmt.Errorf("title is required for video material")
		}
		if err := w.WriteField("title", title); err != nil {
			return nil, fmt.Errorf("writing title field: %w", err)
		}
		if err := w.WriteField("introduction", introduction); err != nil {
			return nil, fmt.Errorf("writing introduction field: %w", err)
		}
	}

	if err := w.Close(); err != nil {
		return nil, fmt.Errorf("closing multipart writer: %w", err)
	}

	url := fmt.Sprintf("%s?access_token=%s", AddURL, token)
	resp, err := http.Post(url, w.FormDataContentType(), &buf)
	if err != nil {
		return nil, fmt.Errorf("HTTP request failed: %w", err)
	}
	defer resp.Body.Close()

	return decodeResponse[AddResponse](resp.Body)
}

// --- Remove (Delete) ---

// RemoveResponse is the response from the material/del_material API.
type RemoveResponse struct {
	ErrCode int    `json:"errcode"`
	ErrMsg  string `json:"errmsg"`
}

// Remove deletes a permanent material by media_id.
func Remove(token, mediaID string) error {
	body := map[string]string{"media_id": mediaID}
	data, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("marshaling request: %w", err)
	}

	url := fmt.Sprintf("%s?access_token=%s", DelURL, token)
	resp, err := http.Post(url, "application/json; charset=utf-8", bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("HTTP request failed: %w", err)
	}
	defer resp.Body.Close()

	_, err = decodeResponse[RemoveResponse](resp.Body)
	return err
}

// --- Show (Get) ---

// ShowResponse is the response from the material/get_material API.
// For news and video types, the response is JSON. For image and voice types,
// the response is the raw file content — those fields will be empty and
// RawBody will contain the binary data.
type ShowResponse struct {
	// RawBody contains the raw response body (useful for binary content).
	RawBody []byte

	// JSON-parsed fields (populated for news and video types).

	// NewsItem is populated for news materials.
	NewsItem []NewsArticle `json:"news_item,omitempty"`

	// Video fields are populated for video materials.
	VideoTitle       string `json:"title,omitempty"`
	VideoDescription string `json:"description,omitempty"`
	DownURL          string `json:"down_url,omitempty"`

	// ErrCode and ErrMsg are checked for API errors.
	ErrCode int    `json:"errcode,omitempty"`
	ErrMsg  string `json:"errmsg,omitempty"`
}

// NewsArticle represents a single article in a news material.
type NewsArticle struct {
	Title              string `json:"title"`
	ThumbMediaID       string `json:"thumb_media_id"`
	ShowCoverPic       int    `json:"show_cover_pic"`
	Author             string `json:"author"`
	Digest             string `json:"digest"`
	Content            string `json:"content"`
	URL                string `json:"url"`
	ContentSourceURL   string `json:"content_source_url"`
	NeedOpenComment    int    `json:"need_open_comment"`
	OnlyFansCanComment int    `json:"only_fans_can_comment"`
}

// Show retrieves a permanent material by media_id.
// For news and video types, the response is decoded into ShowResponse fields.
// For image and voice types, the raw binary is returned in RawBody.
func Show(token, mediaID string) (*ShowResponse, error) {
	body := map[string]string{"media_id": mediaID}
	data, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("marshaling request: %w", err)
	}

	url := fmt.Sprintf("%s?access_token=%s", GetURL, token)
	resp, err := http.Post(url, "application/json; charset=utf-8", bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("HTTP request failed: %w", err)
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading response: %w", err)
	}

	// Try to decode as JSON — works for news, video, and error responses.
	var sr ShowResponse
	if err := json.Unmarshal(raw, &sr); err == nil {
		// Check for API error
		if sr.ErrCode != 0 {
			return nil, fmt.Errorf("WeChat API error (code %d): %s", sr.ErrCode, sr.ErrMsg)
		}
		// If we got a JSON with meaningful fields, return it
		if len(sr.NewsItem) > 0 || sr.VideoTitle != "" || sr.DownURL != "" {
			sr.RawBody = raw
			return &sr, nil
		}
		// JSON was just {"errcode":0,"errmsg":"ok"} with no content fields.
		// This means the response is binary (image/voice) — fall through.
	} else {
		// Not JSON at all — binary response (image/voice)
		// Check for WeChat API error via the HTTP status
		if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(raw))
		}
	}

	// Return raw binary (image/voice content)
	return &ShowResponse{RawBody: raw}, nil
}

// --- helpers ---

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

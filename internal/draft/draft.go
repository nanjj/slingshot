// Package draft provides WeChat Draft API operations.
//
// WeChat Draft Box (草稿箱) API allows managing draft articles:
//   - Add:    POST /cgi-bin/draft/add
//   - List:   POST /cgi-bin/draft/batchget
//   - Show:   POST /cgi-bin/draft/get
//   - Update: POST /cgi-bin/draft/update
//   - Remove: POST /cgi-bin/draft/delete
package draft

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

// API endpoints — made variables so tests can override them.
var (
	AddURL    = "https://api.weixin.qq.com/cgi-bin/draft/add"
	ListURL   = "https://api.weixin.qq.com/cgi-bin/draft/batchget"
	GetURL    = "https://api.weixin.qq.com/cgi-bin/draft/get"
	UpdateURL = "https://api.weixin.qq.com/cgi-bin/draft/update"
	DeleteURL = "https://api.weixin.qq.com/cgi-bin/draft/delete"
)


// Article represents a single article in a WeChat draft.
type Article struct {
	Title              string `json:"title"`
	ThumbMediaID       string `json:"thumb_media_id,omitempty"`
	Author             string `json:"author,omitempty"`
	Digest             string `json:"digest,omitempty"`
	ShowCoverPic       int    `json:"show_cover_pic,omitempty"`
	Content            string `json:"content"`
	ContentSourceURL   string `json:"content_source_url,omitempty"`
	NeedOpenComment    int    `json:"need_open_comment,omitempty"`
	OnlyFansCanComment int    `json:"only_fans_can_comment,omitempty"`
}

// AddResponse is the response from the draft/add API.
type AddResponse struct {
	MediaID string `json:"media_id"`
	ErrCode int    `json:"errcode,omitempty"`
	ErrMsg  string `json:"errmsg,omitempty"`
}

// Add creates a new draft with the given articles.
func Add(token string, articles []Article) (*AddResponse, error) {
	body := map[string]any{"articles": articles}
	data, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("marshaling request: %w", err)
	}

	url := fmt.Sprintf("%s?access_token=%s", AddURL, token)
	resp, err := http.Post(url, "application/json; charset=utf-8", bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("HTTP request failed: %w", err)
	}
	defer resp.Body.Close()

	return decodeResponse[AddResponse](resp.Body)
}

// --- List ---

// ListRequest is the request body for listing drafts.
type ListRequest struct {
	Offset    int `json:"offset"`
	Count     int `json:"count"`
	NoContent int `json:"no_content"`
}

// ArticleInfo provides details of an article in a draft (from list/get).
type ArticleInfo struct {
	Title              string `json:"title"`
	Author             string `json:"author"`
	Digest             string `json:"digest"`
	Content            string `json:"content,omitempty"`
	ContentSourceURL   string `json:"content_source_url"`
	ThumbMediaID       string `json:"thumb_media_id"`
	ShowCoverPic       int    `json:"show_cover_pic"`
	URL                string `json:"url"`
	NeedOpenComment    int    `json:"need_open_comment"`
	OnlyFansCanComment int    `json:"only_fans_can_comment"`
	CreateTime         int64  `json:"create_time"`
	UpdateTime         int64  `json:"update_time"`
}

// DraftItem represents a single draft in the list.
type DraftItem struct {
	MediaID    string       `json:"media_id"`
	Content    DraftContent `json:"content"`
	UpdateTime int64        `json:"update_time"`
}

// DraftContent wraps the articles in a draft.
type DraftContent struct {
	Articles []ArticleInfo `json:"news_item"`
}

// ListResponse is the response from the draft/batchget API.
type ListResponse struct {
	TotalCount int         `json:"total_count"`
	ItemCount  int         `json:"item_count"`
	Items      []DraftItem `json:"item"`
	ErrCode    int         `json:"errcode,omitempty"`
	ErrMsg     string      `json:"errmsg,omitempty"`
}

// List retrieves a paginated list of drafts.
func List(token string, offset, count int) (*ListResponse, error) {
	req := ListRequest{
		Offset:    offset,
		Count:     count,
		NoContent: 1, // skip content body to reduce payload
	}
	data, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("marshaling request: %w", err)
	}

	url := fmt.Sprintf("%s?access_token=%s", ListURL, token)
	resp, err := http.Post(url, "application/json; charset=utf-8", bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("HTTP request failed: %w", err)
	}
	defer resp.Body.Close()

	return decodeResponse[ListResponse](resp.Body)
}

// --- Show (Get) ---

// ShowResponse is the response from the draft/get API.
type ShowResponse struct {
	DraftContent
	ErrCode int    `json:"errcode,omitempty"`
	ErrMsg  string `json:"errmsg,omitempty"`
}

// Show retrieves a single draft's full details by media_id.
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

	return decodeResponse[ShowResponse](resp.Body)
}

// --- Update ---

// Update modifies an article in a draft by media_id and article index.
// The articles parameter is a single Article (not a slice) — this matches
// the WeChat API shape.
func Update(token, mediaID string, index int, article Article) error {
	body := map[string]any{
		"media_id": mediaID,
		"index":    index,
		"articles": article,
	}
	data, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("marshaling request: %w", err)
	}

	url := fmt.Sprintf("%s?access_token=%s", UpdateURL, token)
	resp, err := http.Post(url, "application/json; charset=utf-8", bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("HTTP request failed: %w", err)
	}
	defer resp.Body.Close()

	_, err = decodeResponse[struct {
		ErrCode int    `json:"errcode"`
		ErrMsg  string `json:"errmsg"`
	}](resp.Body)
	return err
}

// --- Remove (Delete) ---

// Remove deletes a draft by media_id.
func Remove(token, mediaID string) error {
	body := map[string]string{"media_id": mediaID}
	data, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("marshaling request: %w", err)
	}

	url := fmt.Sprintf("%s?access_token=%s", DeleteURL, token)
	resp, err := http.Post(url, "application/json; charset=utf-8", bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("HTTP request failed: %w", err)
	}
	defer resp.Body.Close()

	_, err = decodeResponse[struct {
		ErrCode int    `json:"errcode"`
		ErrMsg  string `json:"errmsg"`
	}](resp.Body)
	return err
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

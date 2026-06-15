// Package uploadimage — see uploadimage.go for overview.

package uploadimage

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

// MaterialUploadURL is the WeChat API endpoint for uploading permanent material
// (素材管理). It returns a media_id suitable for use as thumb_media_id in drafts.
//
//	POST https://api.weixin.qq.com/cgi-bin/material/add_material?access_token=TOKEN&type=image
//
// Made a variable (not constant) so that tests can override it.
var MaterialUploadURL = "https://api.weixin.qq.com/cgi-bin/material/add_material"

// ThumbUploadResponse represents the WeChat permanent material upload response.
type ThumbUploadResponse struct {
	MediaID string `json:"media_id"`
	URL     string `json:"url,omitempty"`
	ErrCode int    `json:"errcode"`
	ErrMsg  string `json:"errmsg"`
}

// UploadThumb uploads an image file as permanent material and returns its
// media_id and URL. The media_id can be used as thumb_media_id in draft/add
// or draft/update. The URL can be used in article content — saving both
// avoids a separate content-image upload for the same file.
//
// The upload uses cgi-bin/material/add_material?type=image which stores the
// image permanently in WeChat's material management system.
func UploadThumb(token, filePath string) (mediaID, url string, err error) {
	// Validate file exists and is readable
	f, err := os.Open(filePath)
	if err != nil {
		return "", "", fmt.Errorf("opening image file %q: %w", filePath, err)
	}
	defer f.Close()

	// Build multipart form
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)

	fw, err := w.CreateFormFile("media", filepath.Base(filePath))
	if err != nil {
		return "", "", fmt.Errorf("creating form file: %w", err)
	}

	if _, err := io.Copy(fw, f); err != nil {
		return "", "", fmt.Errorf("copying file content: %w", err)
	}
	if err := w.Close(); err != nil {
		return "", "", fmt.Errorf("closing multipart writer: %w", err)
	}

	// Build and send request
	urlStr := fmt.Sprintf("%s?access_token=%s&type=image", MaterialUploadURL, token)
	resp, err := http.Post(urlStr, w.FormDataContentType(), &buf) //nolint:gosec // token is from config
	if err != nil {
		return "", "", fmt.Errorf("HTTP request failed: %w", err)
	}
	defer resp.Body.Close()

	// Decode response
	var tr ThumbUploadResponse
	if err := json.NewDecoder(resp.Body).Decode(&tr); err != nil {
		return "", "", fmt.Errorf("decoding response: %w", err)
	}

	if tr.ErrCode != 0 {
		return "", "", fmt.Errorf("WeChat upload error (code %d): %s", tr.ErrCode, tr.ErrMsg)
	}
	if tr.MediaID == "" {
		return "", "", fmt.Errorf("empty media_id in upload response")
	}

	return tr.MediaID, tr.URL, nil
}


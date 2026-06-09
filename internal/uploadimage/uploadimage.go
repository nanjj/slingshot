// Package uploadimage uploads images to WeChat permanent material management.
//
// It uses the WeChat API endpoint:
//
//	POST https://api.weixin.qq.com/cgi-bin/media/uploadimg?access_token=TOKEN
//
// The uploaded image URL can be used in WeChat article content.
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

// UploadURL is the WeChat API endpoint for uploading images (发表内容中的图片).
// Made a variable (not constant) so that tests can override it.
var UploadURL = "https://api.weixin.qq.com/cgi-bin/media/uploadimg"

// UploadResponse represents the WeChat image upload API response.
type UploadResponse struct {
	URL     string `json:"url"`
	ErrCode int    `json:"errcode"`
	ErrMsg  string `json:"errmsg"`
}

// Upload uploads an image file to WeChat and returns the upload response.
//
// The token is a valid WeChat access token.
// The filePath is the path to the image file on disk.
func Upload(token, filePath string) (*UploadResponse, error) {
	// Validate file exists and is readable
	f, err := os.Open(filePath)
	if err != nil {
		return nil, fmt.Errorf("opening image file %q: %w", filePath, err)
	}
	defer f.Close()

	// Build multipart form
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)

	fw, err := w.CreateFormFile("media", filepath.Base(filePath))
	if err != nil {
		return nil, fmt.Errorf("creating form file: %w", err)
	}

	if _, err := io.Copy(fw, f); err != nil {
		return nil, fmt.Errorf("copying file content: %w", err)
	}
	if err := w.Close(); err != nil {
		return nil, fmt.Errorf("closing multipart writer: %w", err)
	}

	// Build and send request
	url := fmt.Sprintf("%s?access_token=%s", UploadURL, token)
	resp, err := http.Post(url, w.FormDataContentType(), &buf) //nolint:gosec // token is from config
	if err != nil {
		return nil, fmt.Errorf("HTTP request failed: %w", err)
	}
	defer resp.Body.Close()

	// Decode response
	var ur UploadResponse
	if err := json.NewDecoder(resp.Body).Decode(&ur); err != nil {
		return nil, fmt.Errorf("decoding response: %w", err)
	}

	if ur.ErrCode != 0 {
		return nil, fmt.Errorf("WeChat upload error (code %d): %s", ur.ErrCode, ur.ErrMsg)
	}
	if ur.URL == "" {
		return nil, fmt.Errorf("empty URL in upload response")
	}

	return &ur, nil
}

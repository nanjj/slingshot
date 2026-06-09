package uploadimage

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestUpload(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		// Create a temporary image file
		tmpDir := t.TempDir()
		imgPath := filepath.Join(tmpDir, "test.png")
		if err := os.WriteFile(imgPath, []byte("fake-png-content"), 0644); err != nil {
			t.Fatal(err)
		}

		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodPost {
				t.Errorf("expected POST, got %s", r.Method)
			}
			if r.URL.Query().Get("access_token") != "test_token" {
				t.Errorf("expected access_token=test_token")
			}
			// Verify multipart form
			ct := r.Header.Get("Content-Type")
			if !strings.HasPrefix(ct, "multipart/form-data") {
				t.Errorf("expected multipart/form-data, got %q", ct)
			}
			if err := r.ParseMultipartForm(10 << 20); err != nil {
				t.Fatal(err)
			}
			file, _, err := r.FormFile("media")
			if err != nil {
				t.Fatalf("expected media field: %v", err)
			}
			file.Close()

			_ = json.NewEncoder(w).Encode(UploadResponse{
				URL:     "http://mmbiz.qpic.cn/test123",
				ErrCode: 0,
				ErrMsg:  "ok",
			})
		}))
		defer srv.Close()

		originalURL := UploadURL
		UploadURL = srv.URL
		defer func() { UploadURL = originalURL }()

		resp, err := Upload("test_token", imgPath)
		if err != nil {
			t.Fatalf("Upload() error = %v", err)
		}
		if resp.URL != "http://mmbiz.qpic.cn/test123" {
			t.Errorf("expected test URL, got %q", resp.URL)
		}
		if resp.ErrCode != 0 {
			t.Errorf("expected errcode=0, got %d", resp.ErrCode)
		}
	})

	t.Run("api_error", func(t *testing.T) {
		tmpDir := t.TempDir()
		imgPath := filepath.Join(tmpDir, "test.jpg")
		if err := os.WriteFile(imgPath, []byte("fake-jpg"), 0644); err != nil {
			t.Fatal(err)
		}

		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_ = json.NewEncoder(w).Encode(UploadResponse{
				ErrCode: 40005,
				ErrMsg:  "invalid file type",
			})
		}))
		defer srv.Close()

		originalURL := UploadURL
		UploadURL = srv.URL
		defer func() { UploadURL = originalURL }()

		_, err := Upload("token", imgPath)
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if !strings.Contains(err.Error(), "invalid file type") {
			t.Errorf("unexpected error: %v", err)
		}
	})

	t.Run("file_not_found", func(t *testing.T) {
		_, err := Upload("token", "/nonexistent/image.png")
		if err == nil {
			t.Fatal("expected error for missing file, got nil")
		}
	})

	t.Run("empty_url_response", func(t *testing.T) {
		tmpDir := t.TempDir()
		imgPath := filepath.Join(tmpDir, "test.png")
		if err := os.WriteFile(imgPath, []byte("content"), 0644); err != nil {
			t.Fatal(err)
		}

		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_ = json.NewEncoder(w).Encode(UploadResponse{
				URL:     "",
				ErrCode: 0,
				ErrMsg:  "ok",
			})
		}))
		defer srv.Close()

		originalURL := UploadURL
		UploadURL = srv.URL
		defer func() { UploadURL = originalURL }()

		_, err := Upload("token", imgPath)
		if err == nil {
			t.Fatal("expected error for empty URL, got nil")
		}
	})
}

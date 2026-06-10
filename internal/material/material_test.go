package material

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

func TestList(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if r.URL.Query().Get("access_token") != "test-token" {
			t.Errorf("expected access_token=test-token")
		}
		defer r.Body.Close()
		var req ListRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatal(err)
		}
		if req.Type != TypeImage {
			t.Errorf("expected type 'image', got %q", req.Type)
		}
		if req.Offset != 0 {
			t.Errorf("expected offset 0, got %d", req.Offset)
		}
		if req.Count != 20 {
			t.Errorf("expected count 20, got %d", req.Count)
		}
		w.Write([]byte(`{
			"total_count": 3,
			"item_count": 2,
			"item": [
				{"media_id": "media1", "name": "image1.jpg", "update_time": 1717000000, "url": "https://mmbiz.qpic.cn/xxx1"},
				{"media_id": "media2", "name": "image2.png", "update_time": 1717000001, "url": "https://mmbiz.qpic.cn/xxx2"}
			]
		}`))
	}))
	defer ts.Close()

	BatchGetURL = ts.URL + "/material/batchget"

	resp, err := List("test-token", TypeImage, 0, 20)
	if err != nil {
		t.Fatal(err)
	}
	if resp.TotalCount != 3 {
		t.Errorf("expected total_count 3, got %d", resp.TotalCount)
	}
	if resp.ItemCount != 2 {
		t.Errorf("expected item_count 2, got %d", resp.ItemCount)
	}
	if len(resp.Items) != 2 {
		t.Fatalf("expected 2 items, got %d", len(resp.Items))
	}
	if resp.Items[0].MediaID != "media1" {
		t.Errorf("expected media_id 'media1', got %q", resp.Items[0].MediaID)
	}
	if resp.Items[0].Name != "image1.jpg" {
		t.Errorf("expected name 'image1.jpg', got %q", resp.Items[0].Name)
	}
	if resp.Items[0].URL != "https://mmbiz.qpic.cn/xxx1" {
		t.Errorf("expected URL, got %q", resp.Items[0].URL)
	}
	if resp.Items[1].MediaID != "media2" {
		t.Errorf("expected media_id 'media2', got %q", resp.Items[1].MediaID)
	}
}

func TestListAPIError(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"errcode": 40001, "errmsg": "invalid credential"}`))
	}))
	defer ts.Close()

	BatchGetURL = ts.URL + "/material/batchget"

	_, err := List("bad-token", TypeImage, 0, 20)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if err.Error() != "WeChat API error (code 40001): invalid credential" {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestListVideoType(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer r.Body.Close()
		var req ListRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatal(err)
		}
		if req.Type != TypeVideo {
			t.Errorf("expected type 'video', got %q", req.Type)
		}
		w.Write([]byte(`{"total_count": 0, "item_count": 0, "item": []}`))
	}))
	defer ts.Close()

	BatchGetURL = ts.URL + "/material/batchget"

	resp, err := List("test-token", TypeVideo, 0, 20)
	if err != nil {
		t.Fatal(err)
	}
	if resp.TotalCount != 0 {
		t.Errorf("expected total_count 0, got %d", resp.TotalCount)
	}
}

func TestListCustomOffset(t *testing.T) {
	var gotReq ListRequest
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer r.Body.Close()
		json.NewDecoder(r.Body).Decode(&gotReq)
		w.Write([]byte(`{"total_count": 20, "item_count": 5, "item": []}`))
	}))
	defer ts.Close()

	BatchGetURL = ts.URL + "/material/batchget"

	_, err := List("test-token", TypeImage, 10, 5)
	if err != nil {
		t.Fatal(err)
	}
	if gotReq.Offset != 10 {
		t.Errorf("expected offset 10, got %d", gotReq.Offset)
	}
	if gotReq.Count != 5 {
		t.Errorf("expected count 5, got %d", gotReq.Count)
	}
}

// --- Remove tests ---

func TestRemove(t *testing.T) {
	var gotBody map[string]string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if r.URL.Query().Get("access_token") != "test-token" {
			t.Errorf("expected access_token=test-token")
		}
		defer r.Body.Close()
		if err := json.NewDecoder(r.Body).Decode(&gotBody); err != nil {
			t.Fatal(err)
		}
		w.Write([]byte(`{"errcode": 0, "errmsg": "ok"}`))
	}))
	defer ts.Close()

	DelURL = ts.URL + "/material/del"

	err := Remove("test-token", "media_to_delete")
	if err != nil {
		t.Fatal(err)
	}

	if gotBody["media_id"] != "media_to_delete" {
		t.Errorf("expected media_id 'media_to_delete', got %q", gotBody["media_id"])
	}
}

func TestRemoveAPIError(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"errcode": 40007, "errmsg": "invalid media_id"}`))
	}))
	defer ts.Close()

	DelURL = ts.URL + "/material/del"

	err := Remove("test-token", "nonexistent")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if err.Error() != "WeChat API error (code 40007): invalid media_id" {
		t.Errorf("unexpected error: %v", err)
	}
}

// --- Show tests ---

func TestShowNews(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if r.URL.Query().Get("access_token") != "test-token" {
			t.Errorf("expected access_token=test-token")
		}
		defer r.Body.Close()
		var body map[string]string
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		if body["media_id"] != "news_media" {
			t.Errorf("expected media_id 'news_media', got %q", body["media_id"])
		}
		w.Write([]byte(`{
			"news_item": [
				{"title": "Article 1", "author": "Author 1", "digest": "Digest 1",
				 "thumb_media_id": "thumb1", "show_cover_pic": 1,
				 "content": "<p>Content</p>", "url": "https://mp.weixin.qq.com/s/abc",
				 "content_source_url": "", "need_open_comment": 0, "only_fans_can_comment": 0}
			]
		}`))
	}))
	defer ts.Close()

	GetURL = ts.URL + "/material/get"

	resp, err := Show("test-token", "news_media")
	if err != nil {
		t.Fatal(err)
	}
	if len(resp.NewsItem) != 1 {
		t.Fatalf("expected 1 news item, got %d", len(resp.NewsItem))
	}
	if resp.NewsItem[0].Title != "Article 1" {
		t.Errorf("expected title 'Article 1', got %q", resp.NewsItem[0].Title)
	}
	if resp.NewsItem[0].URL != "https://mp.weixin.qq.com/s/abc" {
		t.Errorf("unexpected URL: %q", resp.NewsItem[0].URL)
	}
}

func TestShowVideo(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"title": "My Video", "description": "A test video", "down_url": "https://example.com/video.mp4"}`))
	}))
	defer ts.Close()

	GetURL = ts.URL + "/material/get"

	resp, err := Show("test-token", "video_media")
	if err != nil {
		t.Fatal(err)
	}
	if resp.VideoTitle != "My Video" {
		t.Errorf("expected title 'My Video', got %q", resp.VideoTitle)
	}
	if resp.DownURL != "https://example.com/video.mp4" {
		t.Errorf("expected down_url, got %q", resp.DownURL)
	}
}

func TestShowImageBinary(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/jpeg")
		w.Write([]byte{0xFF, 0xD8, 0xFF, 0xE0}) // JPEG header
	}))
	defer ts.Close()

	GetURL = ts.URL + "/material/get"

	resp, err := Show("test-token", "image_media")
	if err != nil {
		t.Fatal(err)
	}
	if len(resp.RawBody) == 0 {
		t.Fatal("expected raw body for image material")
	}
	if resp.RawBody[0] != 0xFF || resp.RawBody[1] != 0xD8 {
		t.Errorf("expected JPEG header, got %x", resp.RawBody[:2])
	}
}

func TestShowAPIError(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"errcode": 40007, "errmsg": "media_id not exist"}`))
	}))
	defer ts.Close()

	GetURL = ts.URL + "/material/get"

	_, err := Show("test-token", "nonexistent")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if err.Error() != "WeChat API error (code 40007): media_id not exist" {
		t.Errorf("unexpected error: %v", err)
	}
}

// --- Add tests (using a temp file for upload) ---

func TestAddImage(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		// Verify it's multipart
		ctype := r.Header.Get("Content-Type")
		if !strings.Contains(ctype, "multipart/form-data") {
			t.Errorf("expected multipart/form-data, got %q", ctype)
		}
		if err := r.ParseMultipartForm(32 << 20); err != nil {
			t.Fatal(err)
		}
		if r.FormValue("type") != "image" {
			t.Errorf("expected type 'image', got %q", r.FormValue("type"))
		}
		file, _, err := r.FormFile("media")
		if err != nil {
			t.Fatal(err)
		}
		file.Close()
		w.Write([]byte(`{"media_id": "uploaded_media_id", "url": "https://mmbiz.qpic.cn/xxx"}`))
	}))
	defer ts.Close()

	AddURL = ts.URL + "/material/add"

	// Create a temp file
	tmpDir := t.TempDir()
	tmpFile := tmpDir + "/test.png"
	if err := os.WriteFile(tmpFile, []byte("fake-image-data"), 0644); err != nil {
		t.Fatal(err)
	}

	resp, err := Add("test-token", tmpFile, TypeImage, "", "")
	if err != nil {
		t.Fatal(err)
	}
	if resp.MediaID != "uploaded_media_id" {
		t.Errorf("expected media_id 'uploaded_media_id', got %q", resp.MediaID)
	}
}

func TestAddVideo(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseMultipartForm(32 << 20); err != nil {
			t.Fatal(err)
		}
		if r.FormValue("type") != "video" {
			t.Errorf("expected type 'video', got %q", r.FormValue("type"))
		}
		if r.FormValue("title") != "My Video" {
			t.Errorf("expected title 'My Video', got %q", r.FormValue("title"))
		}
		if r.FormValue("introduction") != "Intro text" {
			t.Errorf("expected introduction 'Intro text', got %q", r.FormValue("introduction"))
		}
		w.Write([]byte(`{"media_id": "video_media_id"}`))
	}))
	defer ts.Close()

	AddURL = ts.URL + "/material/add"

	tmpDir := t.TempDir()
	tmpFile := tmpDir + "/test.mp4"
	if err := os.WriteFile(tmpFile, []byte("fake-video-data"), 0644); err != nil {
		t.Fatal(err)
	}

	resp, err := Add("test-token", tmpFile, TypeVideo, "My Video", "Intro text")
	if err != nil {
		t.Fatal(err)
	}
	if resp.MediaID != "video_media_id" {
		t.Errorf("expected media_id 'video_media_id', got %q", resp.MediaID)
	}
}

func TestAddVideoMissingTitle(t *testing.T) {
	tmpDir := t.TempDir()
	tmpFile := tmpDir + "/test.mp4"
	if err := os.WriteFile(tmpFile, []byte("data"), 0644); err != nil {
		t.Fatal(err)
	}

	_, err := Add("token", tmpFile, TypeVideo, "", "")
	if err == nil {
		t.Fatal("expected error for missing video title, got nil")
	}
}

func TestAddAPIError(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"errcode": 40009, "errmsg": "file size exceeds limit"}`))
	}))
	defer ts.Close()

	AddURL = ts.URL + "/material/add"

	tmpDir := t.TempDir()
	tmpFile := tmpDir + "/test.jpg"
	if err := os.WriteFile(tmpFile, []byte("data"), 0644); err != nil {
		t.Fatal(err)
	}

	_, err := Add("token", tmpFile, TypeImage, "", "")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if err.Error() != "WeChat API error (code 40009): file size exceeds limit" {
		t.Errorf("unexpected error: %v", err)
	}
}

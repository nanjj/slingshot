package draft

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestAdd(t *testing.T) {
	var gotBody map[string]any
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
		w.Write([]byte(`{"media_id": "test_media_id"}`))
	}))
	defer ts.Close()

	AddURL = ts.URL + "/draft/add"

	articles := []Article{
		{Title: "Test Title", Content: "<p>Test content</p>"},
	}
	resp, err := Add("test-token", articles)
	if err != nil {
		t.Fatal(err)
	}
	if resp.MediaID != "test_media_id" {
		t.Errorf("expected media_id 'test_media_id', got %q", resp.MediaID)
	}

	// Verify request body
	arts, ok := gotBody["articles"].([]any)
	if !ok {
		t.Fatal("expected articles array in request body")
	}
	if len(arts) != 1 {
		t.Fatalf("expected 1 article, got %d", len(arts))
	}
	art := arts[0].(map[string]any)
	if art["title"] != "Test Title" {
		t.Errorf("expected title 'Test Title', got %v", art["title"])
	}
	if art["content"] != "<p>Test content</p>" {
		t.Errorf("expected content '<p>Test content</p>', got %v", art["content"])
	}
}

func TestAddAPIError(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"errcode": 40001, "errmsg": "invalid credential"}`))
	}))
	defer ts.Close()

	AddURL = ts.URL + "/draft/add"

	_, err := Add("bad-token", []Article{{Title: "Test"}})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if err.Error() != "WeChat API error (code 40001): invalid credential" {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestList(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		w.Write([]byte(`{
			"total_count": 2,
			"item_count": 2,
			"item": [
				{
					"media_id": "media1",
					"content": {
						"news_item": [
							{"title": "Article 1", "author": "Author 1"}
						]
					},
					"update_time": 1717000000
				},
				{
					"media_id": "media2",
					"content": {
						"news_item": [
							{"title": "Article 2", "author": "Author 2"}
						]
					},
					"update_time": 1717000001
				}
			]
		}`))
	}))
	defer ts.Close()

	ListURL = ts.URL + "/draft/batchget"

	resp, err := List("test-token", 0, 20)
	if err != nil {
		t.Fatal(err)
	}
	if resp.TotalCount != 2 {
		t.Errorf("expected total_count 2, got %d", resp.TotalCount)
	}
	if len(resp.Items) != 2 {
		t.Fatalf("expected 2 items, got %d", len(resp.Items))
	}
	if resp.Items[0].MediaID != "media1" {
		t.Errorf("expected media_id 'media1', got %q", resp.Items[0].MediaID)
	}
	if resp.Items[0].Content.Articles[0].Title != "Article 1" {
		t.Errorf("expected title 'Article 1', got %q", resp.Items[0].Content.Articles[0].Title)
	}
}

func TestShow(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]string
		defer r.Body.Close()
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		if body["media_id"] != "test_media" {
			t.Errorf("expected media_id 'test_media', got %q", body["media_id"])
		}
		w.Write([]byte(`{
			"news_item": [
				{
					"title": "Detailed Article",
					"author": "Test Author",
					"digest": "A summary",
					"content": "<p>Full content</p>",
					"url": "https://mp.weixin.qq.com/s/test"
				}
			]
		}`))
	}))
	defer ts.Close()

	GetURL = ts.URL + "/draft/get"

	resp, err := Show("test-token", "test_media")
	if err != nil {
		t.Fatal(err)
	}
	if len(resp.Articles) != 1 {
		t.Fatalf("expected 1 article, got %d", len(resp.Articles))
	}
	if resp.Articles[0].Title != "Detailed Article" {
		t.Errorf("expected title 'Detailed Article', got %q", resp.Articles[0].Title)
	}
	if resp.Articles[0].URL != "https://mp.weixin.qq.com/s/test" {
		t.Errorf("unexpected URL: %q", resp.Articles[0].URL)
	}
}

func TestUpdate(t *testing.T) {
	var gotBody map[string]any
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer r.Body.Close()
		if err := json.NewDecoder(r.Body).Decode(&gotBody); err != nil {
			t.Fatal(err)
		}
		w.Write([]byte(`{"errcode": 0, "errmsg": "ok"}`))
	}))
	defer ts.Close()

	UpdateURL = ts.URL + "/draft/update"

	article := Article{Title: "Updated Title", Content: "<p>Updated</p>"}
	err := Update("test-token", "media_id_1", 0, article)
	if err != nil {
		t.Fatal(err)
	}

	// Verify request body
	if gotBody["media_id"] != "media_id_1" {
		t.Errorf("expected media_id 'media_id_1', got %v", gotBody["media_id"])
	}
	if gotBody["index"] != float64(0) {
		t.Errorf("expected index 0, got %v", gotBody["index"])
	}
	art, ok := gotBody["articles"].(map[string]any)
	if !ok {
		t.Fatal("expected articles object in request body")
	}
	if art["title"] != "Updated Title" {
		t.Errorf("expected title 'Updated Title', got %v", art["title"])
	}
}

func TestUpdateAPIError(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"errcode": 40007, "errmsg": "invalid media_id"}`))
	}))
	defer ts.Close()

	UpdateURL = ts.URL + "/draft/update"

	err := Update("test-token", "invalid_media", 0, Article{Title: "Test"})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if err.Error() != "WeChat API error (code 40007): invalid media_id" {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestRemove(t *testing.T) {
	var gotBody map[string]string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		defer r.Body.Close()
		if err := json.NewDecoder(r.Body).Decode(&gotBody); err != nil {
			t.Fatal(err)
		}
		w.Write([]byte(`{"errcode": 0, "errmsg": "ok"}`))
	}))
	defer ts.Close()

	DeleteURL = ts.URL + "/draft/delete"

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
		w.Write([]byte(`{"errcode": 40007, "errmsg": "media_id not exist"}`))
	}))
	defer ts.Close()

	DeleteURL = ts.URL + "/draft/delete"

	err := Remove("test-token", "nonexistent")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestHTTPError(t *testing.T) {
	// Test with a server that immediately closes connection
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// close without writing anything
	}))
	defer ts.Close()

	AddURL = ts.URL + "/draft/add"

	_, err := Add("test-token", []Article{{Title: "Test"}})
	// This should either fail with an error (empty response)
	if err == nil {
		t.Fatal("expected error from empty response, got nil")
	}
}

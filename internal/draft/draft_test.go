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

func TestPublish(t *testing.T) {
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
		w.Write([]byte(`{"publish_id": "100000001"}`))
	}))
	defer ts.Close()

	PublishURL = ts.URL + "/freepublish/submit"

	resp, err := Publish("test-token", "test_media_id")
	if err != nil {
		t.Fatal(err)
	}
	if resp.PublishID != "100000001" {
		t.Errorf("expected publish_id '100000001', got %q", resp.PublishID)
	}
	if gotBody["media_id"] != "test_media_id" {
		t.Errorf("expected media_id 'test_media_id', got %q", gotBody["media_id"])
	}
}

func TestPublishAPIError(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"errcode": 40007, "errmsg": "invalid media_id"}`))
	}))
	defer ts.Close()

	PublishURL = ts.URL + "/freepublish/submit"

	_, err := Publish("test-token", "bad_media_id")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if err.Error() != "WeChat API error (code 40007): invalid media_id" {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestSendAll(t *testing.T) {
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
		w.Write([]byte(`{"errcode":0,"errmsg":"send job submission success","msg_id":34182,"msg_data_id":206227730}`))
	}))
	defer ts.Close()

	SendAllURL = ts.URL + "/message/mass/sendall"

	resp, err := SendAll("test-token", "test_media_id", true, nil, 0, "")
	if err != nil {
		t.Fatal(err)
	}
	if resp.MsgID != 34182 {
		t.Errorf("expected msg_id 34182, got %d", resp.MsgID)
	}
	if resp.MsgDataID != 206227730 {
		t.Errorf("expected msg_data_id 206227730, got %d", resp.MsgDataID)
	}

	// Verify request body
	filter, ok := gotBody["filter"].(map[string]any)
	if !ok {
		t.Fatal("expected filter in request body")
	}
	if filter["is_to_all"] != true {
		t.Errorf("expected is_to_all=true, got %v", filter["is_to_all"])
	}
	if gotBody["msgtype"] != "mpnews" {
		t.Errorf("expected msgtype=mpnews, got %v", gotBody["msgtype"])
	}
	mpnews, ok := gotBody["mpnews"].(map[string]any)
	if !ok {
		t.Fatal("expected mpnews in request body")
	}
	if mpnews["media_id"] != "test_media_id" {
		t.Errorf("expected media_id 'test_media_id', got %v", mpnews["media_id"])
	}
}

func TestSendAllWithTag(t *testing.T) {
	tagID := 2
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		defer r.Body.Close()
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		filter := body["filter"].(map[string]any)
		if filter["is_to_all"] != false {
			t.Errorf("expected is_to_all=false")
		}
		if filter["tag_id"] != float64(2) {
			t.Errorf("expected tag_id=2, got %v", filter["tag_id"])
		}
		w.Write([]byte(`{"errcode":0,"errmsg":"send job submission success","msg_id":34183}`))
	}))
	defer ts.Close()

	SendAllURL = ts.URL + "/message/mass/sendall"

	resp, err := SendAll("test-token", "test_media_id", false, &tagID, 1, "my-msg-id")
	if err != nil {
		t.Fatal(err)
	}
	if resp.MsgID != 34183 {
		t.Errorf("expected msg_id 34183, got %d", resp.MsgID)
	}
}

func TestSendAllAPIError(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"errcode": 45009, "errmsg": "reach max api daily quota limit"}`))
	}))
	defer ts.Close()

	SendAllURL = ts.URL + "/message/mass/sendall"

	_, err := SendAll("test-token", "bad_media_id", true, nil, 0, "")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if err.Error() != "WeChat API error (code 45009): reach max api daily quota limit" {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestPreview(t *testing.T) {
	var gotBody map[string]any
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		defer r.Body.Close()
		if err := json.NewDecoder(r.Body).Decode(&gotBody); err != nil {
			t.Fatal(err)
		}
		w.Write([]byte(`{"errcode":0,"errmsg":"preview success"}`))
	}))
	defer ts.Close()

	PreviewURL = ts.URL + "/message/mass/preview"

	resp, err := Preview("test-token", "test_media_id", "openid123", "")
	if err != nil {
		t.Fatal(err)
	}
	if resp.ErrCode != 0 {
		t.Errorf("expected errcode 0, got %d", resp.ErrCode)
	}

	if gotBody["touser"] != "openid123" {
		t.Errorf("expected touser 'openid123', got %v", gotBody["touser"])
	}
	if gotBody["msgtype"] != "mpnews" {
		t.Errorf("expected msgtype mpnews, got %v", gotBody["msgtype"])
	}
	mpnews, ok := gotBody["mpnews"].(map[string]any)
	if !ok {
		t.Fatal("expected mpnews in request body")
	}
	if mpnews["media_id"] != "test_media_id" {
		t.Errorf("expected media_id 'test_media_id', got %v", mpnews["media_id"])
	}
}

func TestPreviewWithWxName(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		defer r.Body.Close()
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		if body["towxname"] != "test_wxname" {
			t.Errorf("expected towxname 'test_wxname', got %v", body["towxname"])
		}
		w.Write([]byte(`{"errcode":0,"errmsg":"preview success"}`))
	}))
	defer ts.Close()

	PreviewURL = ts.URL + "/message/mass/preview"

	_, err := Preview("test-token", "test_media_id", "", "test_wxname")
	if err != nil {
		t.Fatal(err)
	}
}

func TestPreviewAPIError(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"errcode": 40001, "errmsg": "invalid credential"}`))
	}))
	defer ts.Close()

	PreviewURL = ts.URL + "/message/mass/preview"

	_, err := Preview("bad-token", "test_media_id", "openid123", "")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if err.Error() != "WeChat API error (code 40001): invalid credential" {
		t.Errorf("unexpected error: %v", err)
	}
}

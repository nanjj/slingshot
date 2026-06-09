package material

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
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

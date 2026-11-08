package deepseek

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// newTestClient points a Client at srv. Base is set explicitly so the test
// never reaches DefaultBaseURL.
func newTestClient(srv *httptest.Server) *Client {
	c := NewClient("test-key")
	c.Base = srv.URL

	return c
}

// TestPostSendsAuthAndContentType pins the two headers the API needs. Without
// them every request fails with an authentication error that says nothing
// about the missing header.
func TestPostSendsAuthAndContentType(t *testing.T) {
	var gotAuth, gotType, gotBody string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		gotType = r.Header.Get("Content-Type")

		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("reading request body: %v", err)
		}
		gotBody = string(body)

		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer srv.Close()

	c := newTestClient(srv)
	var out struct{ OK bool }
	if err := c.post(context.Background(), "/v1/x", map[string]string{"a": "b"}, &out); err != nil {
		t.Fatalf("post() = %v, want nil", err)
	}

	if gotAuth != "Bearer test-key" {
		t.Errorf("Authorization = %q, want %q", gotAuth, "Bearer test-key")
	}
	if gotType != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", gotType)
	}

	var sent map[string]string
	if err := json.Unmarshal([]byte(gotBody), &sent); err != nil {
		t.Fatalf("request body is not JSON: %v", err)
	}
	if sent["a"] != "b" {
		t.Errorf("request body = %v, want a=b", sent)
	}
	if !out.OK {
		t.Error("response was not decoded into out")
	}
}

// TestPostErrorPaths covers the three ways a request can fail. Each message
// has to name what went wrong: the API's own error text when it provides one,
// the raw body otherwise, and the decode error when the body is not JSON.
func TestPostErrorPaths(t *testing.T) {
	tests := []struct {
		name       string
		status     int
		body       string
		wantSubstr []string
	}{
		{
			name:       "api error message is surfaced",
			status:     http.StatusBadRequest,
			body:       `{"error":{"message":"invalid query"}}`,
			wantSubstr: []string{"HTTP 400", "invalid query"},
		},
		{
			name:       "raw body when there is no api error",
			status:     http.StatusInternalServerError,
			body:       `upstream exploded`,
			wantSubstr: []string{"HTTP 500", "upstream exploded"},
		},
		{
			name:       "undecodable success body",
			status:     http.StatusOK,
			body:       `not json`,
			wantSubstr: []string{"decoding response"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tt.status)
				_, _ = w.Write([]byte(tt.body))
			}))
			defer srv.Close()

			c := newTestClient(srv)
			var out struct{ OK bool }
			err := c.post(context.Background(), "/v1/x", map[string]string{}, &out)
			if err == nil {
				t.Fatalf("post() = nil, want an error for status %d", tt.status)
			}

			for _, want := range tt.wantSubstr {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("error %q does not contain %q", err, want)
				}
			}
		})
	}
}

// TestPostTransportError covers the request never reaching the server: the
// error must name the path so the caller knows which call failed.
func TestPostTransportError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	url := srv.URL
	srv.Close() // now every request fails to connect

	c := NewClient("test-key")
	c.Base = url

	var out struct{ OK bool }
	err := c.post(context.Background(), "/v1/search", map[string]string{}, &out)
	if err == nil {
		t.Fatal("post() = nil, want a transport error")
	}
	if !strings.Contains(err.Error(), "/v1/search") {
		t.Errorf("error %q does not name the path", err)
	}
}

// TestPostUndecodableRequestBody covers the marshal step. A channel cannot be
// encoded as JSON, so the error has to come from post rather than the server.
func TestPostUndecodableRequestBody(t *testing.T) {
	c := NewClient("k")
	c.Base = "http://127.0.0.1:0" // never reached

	var out struct{ OK bool }
	err := c.post(context.Background(), "/v1/x", make(chan int), &out)
	if err == nil {
		t.Fatal("post() = nil, want a marshal error")
	}
	if !strings.Contains(err.Error(), "marshaling request") {
		t.Errorf("error %q does not say the request could not be marshaled", err)
	}
}

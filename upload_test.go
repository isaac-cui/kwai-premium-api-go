package premium

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestChunkedUpload(t *testing.T) {
	var chunks []string
	completed := false
	storage := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/upload/fragment":
			if r.URL.Query().Get("fragment_id") != fmt.Sprint(len(chunks)) {
				t.Error("wrong fragment index")
			}
			body, _ := io.ReadAll(r.Body)
			chunks = append(chunks, string(body))
		case "/api/upload/complete":
			if r.URL.Query().Get("fragment_count") != "3" {
				t.Error("wrong count")
			}
			completed = true
		}
		fmt.Fprint(w, "plain storage response")
	}))
	defer storage.Close()
	c, _ := New(Config{APIKey: "key"})
	u, _ := c.NewUploader(UploadTokenData{Token: "single-file", UploadEndpoint: storage.URL})
	if _, err := u.Upload(context.Background(), strings.NewReader("abcdefgh"), 3); err != nil {
		t.Fatal(err)
	}
	if strings.Join(chunks, "") != "abcdefgh" || len(chunks) != 3 || !completed {
		t.Fatal("corrupted file")
	}
	if _, err := u.Upload(context.Background(), strings.NewReader(""), 3); err == nil {
		t.Fatal("empty upload accepted")
	}
	if _, err := u.Complete(context.Background(), 0); err == nil {
		t.Fatal("zero fragments accepted")
	}
}

type failingReader struct{}

func (failingReader) Read([]byte) (int, error) { return 0, errors.New("disk error") }
func TestUploadFailuresNeverCompleteOrSubmit(t *testing.T) {
	for _, status := range []int{307, 500} {
		calls := 0
		storage := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls++; w.WriteHeader(status) }))
		c, _ := New(Config{APIKey: "key"})
		u, _ := c.NewUploader(UploadTokenData{Token: "token", UploadEndpoint: storage.URL})
		if _, err := u.Upload(context.Background(), strings.NewReader("abcdefgh"), 3); err == nil {
			t.Fatal("ignored storage failure")
		}
		if calls != 1 {
			t.Fatal("completed or retried failed upload")
		}
		storage.Close()
	}
	c, _ := New(Config{APIKey: "key"})
	u, _ := c.NewUploader(UploadTokenData{Token: "token", UploadEndpoint: "storage.example.test"})
	if _, err := u.Upload(context.Background(), failingReader{}, 3); err == nil {
		t.Fatal("read failure ignored")
	}
}

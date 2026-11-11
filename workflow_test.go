package premium

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestImageAndVideoUploadWorkflow(t *testing.T) {
	for _, kind := range []string{"image", "video"} {
		t.Run(kind, func(t *testing.T) {
			var uploaded strings.Builder
			storageCalls := 0
			apiCalls := 0
			storage := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				storageCalls++
				if r.Method != "POST" || r.Header.Get("Authorization") != "" || r.URL.Query().Get("upload_token") != "token +&/?" {
					t.Error("storage auth boundary or token encoding broken")
				}
				if r.Header.Get("Content-Type") != "application/octet-stream" {
					t.Error("storage upload must be binary")
				}
				switch r.URL.Path {
				case "/api/upload/fragment":
					if r.URL.Query().Get("fragment_id") != "0" {
						t.Error("first fragment must be zero")
					}
					body, _ := io.ReadAll(r.Body)
					uploaded.Write(body)
				case "/api/upload/complete":
					if r.URL.Query().Get("fragment_count") != "1" {
						t.Error("wrong fragment count")
					}
				default:
					t.Error("wrong storage path")
				}
				// HTTP 200 with an empty body is success according to this document.
			}))
			defer storage.Close()
			client := testClient(t, func(w http.ResponseWriter, r *http.Request) {
				apiCalls++
				if r.Header.Get("Authorization") != "Bearer test-api-key" {
					t.Error("missing API bearer")
				}
				switch r.URL.Path {
				case "/open/v1/resource/" + kind + "/token":
					if storageCalls != 0 {
						t.Error("token requested after upload")
					}
					fmt.Fprintf(w, `{"result":1,"data":{"token":"token +&/?","uploadEndpoint":%q}}`, storage.URL)
				case "/open/v1/resource/" + kind + "/submit":
					if storageCalls != 2 {
						t.Error("submit happened before storage completed")
					}
					var body map[string]string
					_ = json.NewDecoder(r.Body).Decode(&body)
					if body["token"] != "token +&/?" {
						t.Error("upload token not preserved")
					}
					if kind == "image" {
						fmt.Fprint(w, `{"result":1,"data":{"imageKey":"cover-key"}}`)
					} else {
						if body["videoType"] != "MP4" {
							t.Error("submit format changed")
						}
						fmt.Fprint(w, `{"result":1,"data":{"videoId":"video-01"}}`)
					}
				default:
					t.Error("wrong API path")
				}
			})
			if kind == "image" {
				response, err := client.Images.Upload(context.Background(), strings.NewReader("raw bytes"), "JPG")
				if err != nil || response.Data.ImageKey != "cover-key" {
					t.Fatalf("image failed: %v", err)
				}
			} else {
				response, err := client.Videos.Upload(context.Background(), strings.NewReader("raw bytes"), "MP4")
				if err != nil || response.Data.VideoID != "video-01" {
					t.Fatalf("video failed: %v", err)
				}
			}
			if uploaded.String() != "raw bytes" || apiCalls != 2 || storageCalls != 2 {
				t.Fatal("workflow did not upload once")
			}
		})
	}
}
func TestTranscodePolling(t *testing.T) {
	calls := 0
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.URL.Path != PathVideoTranscode {
			t.Error("wrong polling endpoint")
		}
		if calls == 1 {
			fmt.Fprint(w, `{"result":1,"data":{"status":1}}`)
		} else {
			fmt.Fprint(w, `{"result":1,"data":{"status":2},"traceId":"ready"}`)
		}
	})
	response, err := c.Videos.WaitForTranscode(context.Background(), "video-id", PollOptions{Interval: time.Millisecond, Timeout: time.Second})
	if err != nil || response.TraceID != "ready" || calls != 2 {
		t.Fatalf("poll failed: %v calls=%d", err, calls)
	}
}
func TestPollingStopsOnFailureUnknownStatusRateLimitAndCancellation(t *testing.T) {
	for _, test := range []struct{ name, body string }{
		{"failure", `{"result":1,"data":{"status":3},"traceId":"failed"}`},
		{"unknown", `{"result":1,"data":{"status":99}}`},
		{"rate", `{"result":0,"errorCode":"RATE_LIMIT_EXCEEDED"}`},
		{"timeout", `{"result":1,"data":{"status":1}}`},
	} {
		t.Run(test.name, func(t *testing.T) {
			calls := 0
			c := testClient(t, func(w http.ResponseWriter, r *http.Request) { calls++; fmt.Fprint(w, test.body) })
			_, err := c.Videos.WaitForTranscode(context.Background(), "video-id", PollOptions{Interval: time.Hour, Timeout: 50 * time.Millisecond})
			if err == nil || calls != 1 {
				t.Fatalf("did not stop: %v, calls=%d", err, calls)
			}
			if test.name == "failure" {
				var failure *TranscodeError
				if !errors.As(err, &failure) || failure.TraceID != "failed" {
					t.Fatal("lost transcode failure")
				}
			}
			if test.name == "timeout" && !errors.Is(err, context.DeadlineExceeded) {
				t.Fatal("timeout not preserved")
			}
		})
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	c, _ := New(Config{APIKey: "key"})
	if _, err := c.Videos.WaitForTranscode(ctx, "video-id", PollOptions{}); !errors.Is(err, context.Canceled) {
		t.Fatal("cancelled polling did not stop")
	}
}

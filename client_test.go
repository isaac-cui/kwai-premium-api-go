package premium

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

func testClient(t *testing.T, handler http.HandlerFunc) *Client {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	c, err := New(Config{APIKey: "test-api-key", BaseURL: server.URL})
	if err != nil {
		t.Fatal(err)
	}
	return c
}
func validDrama() CreateDramaRequest {
	return CreateDramaRequest{IdempotencyKey: "business-stable-id", Title: "短剧", CoverImageKey: "cover",
		HeaderImages: []HeaderImage{{ImageKey: "header"}},
		Episodes: []Episode{
			{EpisodeIndex: 1, VideoID: "paid", Title: "第二集", ModuleFeeType: EpisodePaid},
			{EpisodeIndex: 0, VideoID: "free", Title: "第一集", ModuleFeeType: EpisodeFree},
		}}
}
func TestDramaWireContractAndLargeID(t *testing.T) {
	calls := 0
	client := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.Method != "POST" || r.URL.Path != PathDramaCreate || r.Header.Get("Authorization") != "Bearer test-api-key" || r.Header.Get("Content-Type") != "application/json" {
			t.Error("wrong authenticated JSON protocol")
		}
		var body CreateDramaRequest
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
		}
		if body.IdempotencyKey != "business-stable-id" || body.Episodes[0].EpisodeIndex != 1 || body.Episodes[1].EpisodeIndex != 0 {
			t.Error("request reordered or idempotency key changed")
		}
		fmt.Fprint(w, `{"result":1,"data":{"dramaId":1000000000000000304},"traceId":"drama-trace"}`)
	})
	request := validDrama()
	response, err := client.Dramas.Create(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if response.Data.DramaID != 1000000000000000304 || response.TraceID != "drama-trace" || len(response.Raw) == 0 || response.HTTPStatus != 200 {
		t.Fatal("lost typed data or metadata")
	}
	if request.Episodes[0].EpisodeIndex != 1 || calls != 1 {
		t.Fatal("mutated request")
	}
}
func TestBusinessAndHTTPFailures(t *testing.T) {
	for _, test := range []struct {
		name   string
		status int
		body   string
		api    bool
	}{
		{"rate limit", 200, `{"result":0,"errorCode":"RATE_LIMIT_EXCEEDED","errorMsg":"Too many requests, level=MINUTE","traceId":"rate-trace"}`, true},
		{"unauthorized", 200, `{"result":0,"errorCode":"UNAUTHORIZED","traceId":"auth-trace"}`, true},
		{"server", 503, "upstream error", true},
		{"bad JSON", 200, "{", false},
		{"missing result", 200, `{"data":{}}`, false},
		{"unsupported result", 200, `{"result":2,"data":{}}`, false},
		{"null data", 200, `{"result":1,"data":null}`, false},
		{"array data", 200, `{"result":1,"data":[]}`, false},
		{"trailing data", 200, `{"result":1,"data":{}} {}`, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			calls := 0
			c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
				calls++
				w.Header().Set("Retry-After", "12")
				w.WriteHeader(test.status)
				fmt.Fprint(w, test.body)
			})
			_, err := c.Dramas.Create(context.Background(), validDrama())
			if err == nil {
				t.Fatal("failure was ignored")
			}
			var api *APIError
			if errors.As(err, &api) != test.api {
				t.Fatalf("wrong error type %T", err)
			}
			if test.name == "rate limit" && (!api.IsRateLimited() || api.RetryAfter != 12*time.Second || api.TraceID != "rate-trace") {
				t.Fatal("lost rate-limit diagnostics")
			}
			if calls != 1 {
				t.Fatal("write request retried")
			}
		})
	}
}

type failingTransport struct{}

func (failingTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	return nil, &url.Error{Op: "Post", URL: r.URL.String(), Err: errors.New("offline")}
}
func TestTransportErrorsDoNotPrintCredentials(t *testing.T) {
	c, err := New(Config{APIKey: "top-secret", UploadHTTPClient: &http.Client{Transport: failingTransport{}}})
	if err != nil {
		t.Fatal(err)
	}
	u, _ := c.NewUploader(UploadTokenData{Token: "sensitive-upload-token", UploadEndpoint: "uploads.example.test"})
	_, err = u.Fragment(context.Background(), 0, []byte("bytes"))
	var transport *TransportError
	if !errors.As(err, &transport) {
		t.Fatal("expected transport error")
	}
	if strings.Contains(err.Error(), "sensitive") || strings.Contains(err.Error(), "upload_token") {
		t.Fatal("credential leaked in printable error")
	}
}
func TestCancellationRedirectAndResponseLimit(t *testing.T) {
	targetCalls := 0
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { targetCalls++ }))
	defer target.Close()
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, target.URL, 307) })
	if _, err := c.Images.UploadToken(context.Background(), ImageTokenRequest{ImageType: "jpg"}); err == nil {
		t.Fatal("redirect accepted")
	}
	if targetCalls != 0 {
		t.Fatal("credential redirect followed")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := c.Images.UploadToken(ctx, ImageTokenRequest{ImageType: "jpg"}); !errors.Is(err, context.Canceled) {
		t.Fatalf("lost context cancellation: %v", err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, strings.Repeat("x", 100)) }))
	defer server.Close()
	c, _ = New(Config{APIKey: "key", BaseURL: server.URL, MaxResponseBytes: 10})
	if _, err := c.Call(context.Background(), PathImageToken, map[string]string{"imageType": "jpg"}); err == nil {
		t.Fatal("oversize response accepted")
	}
}
func TestGenericCallPreservesRawNumbers(t *testing.T) {
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		if string(raw) != "{}" {
			t.Errorf("expected object: %s", raw)
		}
		fmt.Fprint(w, `{"result":1,"data":{"id":1000000000000000304}}`)
	})
	response, err := c.Call(context.Background(), "/open/v1/future", nil)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(response.Data), "1000000000000000304") {
		t.Fatal("raw data rounded")
	}
	for _, path := range []string{"https://other.test", "/open/v1/../x", "/open/v1/x?q=1", "/open/v1/%2f"} {
		if _, err := c.Call(context.Background(), path, nil); err == nil {
			t.Error("invalid path accepted")
		}
	}
	if _, err := c.Call(context.Background(), PathImageToken, []string{}); err == nil {
		t.Fatal("nonobject request accepted")
	}
}
func TestConfiguration(t *testing.T) {
	for _, config := range []Config{{}, {APIKey: "x\ny"}, {APIKey: "x", BaseURL: "https://user:pass@example.test"}, {APIKey: "x", BaseURL: "https://example.test/path"}, {APIKey: "x", BaseURL: "https://example.test?x=1"}, {APIKey: "x", MaxResponseBytes: -1}} {
		if _, err := New(config); err == nil {
			t.Errorf("accepted bad config: %+v", config)
		}
	}
	c, err := New(Config{APIKey: "key"})
	if err != nil {
		t.Fatal(err)
	}
	u, err := c.NewUploader(UploadTokenData{Token: "token", UploadEndpoint: "storage.example.test"})
	if err != nil || u.endpoint != "https://storage.example.test" {
		t.Fatal("missing HTTPS normalization")
	}
	original := &http.Client{Timeout: 7 * time.Second}
	c, _ = New(Config{APIKey: "key", HTTPClient: original})
	if c.http.Timeout != original.Timeout || original.CheckRedirect != nil {
		t.Fatal("caller HTTP client mutated")
	}
}

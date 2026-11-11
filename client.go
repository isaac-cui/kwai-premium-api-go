// Package premium provides a Go SDK for the Kwai Premium short-drama Open API.
package premium

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const DefaultBaseURL = "https://premium.kwai.com"
const defaultResponseLimit int64 = 4 << 20

type Config struct {
	APIKey  string
	BaseURL string
	// HTTPClient is copied; redirects are disabled on the copy.
	HTTPClient *http.Client
	// UploadHTTPClient is separate so API-specific transports need not reach storage.
	UploadHTTPClient *http.Client
	MaxResponseBytes int64
}

type Client struct {
	baseURL       string
	apiKey        string
	http          *http.Client
	uploadHTTP    *http.Client
	responseLimit int64
	Images        *ImagesService
	Videos        *VideosService
	Dramas        *DramasService
}

func New(config Config) (*Client, error) {
	if strings.TrimSpace(config.APIKey) == "" || strings.ContainsAny(config.APIKey, "\r\n") {
		return nil, errors.New("premium: a nonempty API key without line breaks is required")
	}
	if config.BaseURL == "" {
		config.BaseURL = DefaultBaseURL
	}
	if err := validateOrigin(config.BaseURL); err != nil {
		return nil, err
	}
	if config.MaxResponseBytes == 0 {
		config.MaxResponseBytes = defaultResponseLimit
	}
	if config.MaxResponseBytes < 1 || config.MaxResponseBytes >= 1<<63-1 {
		return nil, errors.New("premium: invalid response size limit")
	}
	c := &Client{baseURL: strings.TrimRight(config.BaseURL, "/"), apiKey: config.APIKey, http: copyHTTPClient(config.HTTPClient),
		uploadHTTP: copyHTTPClient(config.UploadHTTPClient), responseLimit: config.MaxResponseBytes}
	c.Images = &ImagesService{client: c}
	c.Videos = &VideosService{client: c}
	c.Dramas = &DramasService{client: c}
	return c, nil
}

func validateOrigin(raw string) error {
	u, err := url.Parse(raw)
	if err != nil || u.Hostname() == "" || (u.Scheme != "https" && u.Scheme != "http") || u.User != nil || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" || (u.Path != "" && u.Path != "/") {
		return errors.New("premium: expected an HTTP(S) origin without a path, credentials, query or fragment")
	}
	return nil
}

func copyHTTPClient(original *http.Client) *http.Client {
	c := http.Client{Timeout: 30 * time.Second}
	if original != nil {
		c = *original
	}
	c.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	return &c
}

// Response retains both typed business data and diagnostic metadata.
type Response[T any] struct {
	Result     int
	Data       T
	TraceID    string
	HTTPStatus int
	Header     http.Header
	Raw        json.RawMessage
}

// APIError represents a server rejection, including result=0 with HTTP 200.
// RetryAfter is the response header's delay; calls are never automatically replayed.
type APIError struct {
	HTTPStatus int
	Result     int
	Code       string
	Message    string
	TraceID    string
	RetryAfter time.Duration
	Header     http.Header
	Raw        json.RawMessage
}

func (e *APIError) Error() string {
	return fmt.Sprintf("premium: HTTP %d, %s: %s (traceId=%s)", e.HTTPStatus, e.Code, e.Message, e.TraceID)
}
func (e *APIError) IsRateLimited() bool { return e.Code == ErrCodeRateLimitExceeded }

// TransportError deliberately omits the URL, which may contain an upload token.
type TransportError struct {
	Operation string
	Err       error
}

func (e *TransportError) Error() string { return "premium: " + e.Operation + " transport failed" }
func (e *TransportError) Unwrap() error { return e.Err }

type ProtocolError struct {
	Message string
	TraceID string
}

func (e *ProtocolError) Error() string { return "premium: invalid response: " + e.Message }

func decodeJSON(raw []byte, target any) error {
	d := json.NewDecoder(bytes.NewReader(raw))
	d.UseNumber()
	if err := d.Decode(target); err != nil {
		return err
	}
	var extra any
	if err := d.Decode(&extra); err != io.EOF {
		return errors.New("trailing JSON content")
	}
	return nil
}

func execute(ctx context.Context, client *http.Client, request *http.Request, operation string, limit int64) ([]byte, int, http.Header, error) {
	response, err := client.Do(request)
	if err != nil {
		if ctx.Err() != nil {
			return nil, 0, nil, ctx.Err()
		}
		return nil, 0, nil, &TransportError{Operation: operation, Err: err}
	}
	defer response.Body.Close()
	header := response.Header.Clone()
	raw, err := io.ReadAll(io.LimitReader(response.Body, limit+1))
	if err != nil {
		return nil, response.StatusCode, header, &TransportError{Operation: operation + " response read", Err: err}
	}
	if int64(len(raw)) > limit {
		return nil, response.StatusCode, header, &ProtocolError{Message: "body exceeds configured size limit"}
	}
	return raw, response.StatusCode, header, nil
}

func retryAfter(header http.Header) time.Duration {
	n, err := strconv.ParseInt(strings.TrimSpace(header.Get("Retry-After")), 10, 64)
	if err != nil || n < 0 || n > int64((1<<63-1)/time.Second) {
		return 0
	}
	return time.Duration(n) * time.Second
}

func call[T any](ctx context.Context, c *Client, path string, request any) (*Response[T], error) {
	if !strings.HasPrefix(path, "/open/v1/") || strings.Contains(path, "..") || strings.ContainsAny(path, "?#\\%\r\n") {
		return nil, errors.New("premium: expected an absolute /open/v1/ endpoint path")
	}
	if request == nil {
		request = map[string]any{}
	}
	body, err := json.Marshal(request)
	if err != nil {
		return nil, fmt.Errorf("premium: encode request: %w", err)
	}
	if len(body) == 0 || body[0] != '{' {
		return nil, errors.New("premium: request body must be a JSON object")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+path, bytes.NewReader(body))
	if err != nil {
		return nil, errors.New("premium: invalid request")
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	raw, status, header, err := execute(ctx, c.http, req, "API", c.responseLimit)
	if err != nil {
		return nil, err
	}
	var envelope struct {
		Result    *int            `json:"result"`
		Data      json.RawMessage `json:"data"`
		ErrorCode string          `json:"errorCode"`
		ErrorMsg  string          `json:"errorMsg"`
		TraceID   string          `json:"traceId"`
	}
	decodeErr := decodeJSON(raw, &envelope)
	result := -1
	if envelope.Result != nil {
		result = *envelope.Result
	}
	if status != http.StatusOK {
		return nil, &APIError{HTTPStatus: status, Result: result, Code: envelope.ErrorCode, Message: envelope.ErrorMsg, TraceID: envelope.TraceID, RetryAfter: retryAfter(header), Header: header, Raw: raw}
	}
	if decodeErr != nil {
		return nil, &ProtocolError{Message: "expected a single JSON object"}
	}
	if result == 0 {
		return nil, &APIError{HTTPStatus: status, Result: result, Code: envelope.ErrorCode, Message: envelope.ErrorMsg, TraceID: envelope.TraceID, RetryAfter: retryAfter(header), Header: header, Raw: raw}
	}
	if result != 1 {
		return nil, &ProtocolError{Message: "missing or unsupported result", TraceID: envelope.TraceID}
	}
	if len(envelope.Data) == 0 || bytes.Equal(bytes.TrimSpace(envelope.Data), []byte("null")) || bytes.TrimSpace(envelope.Data)[0] != '{' {
		return nil, &ProtocolError{Message: "missing data object", TraceID: envelope.TraceID}
	}
	var data T
	if err := decodeJSON(envelope.Data, &data); err != nil {
		return nil, &ProtocolError{Message: "data does not match response type", TraceID: envelope.TraceID}
	}
	return &Response[T]{Result: result, Data: data, TraceID: envelope.TraceID, HTTPStatus: status, Header: header, Raw: raw}, nil
}

// Call is an escape hatch for future endpoints. Raw JSON data preserves numeric IDs.
func (c *Client) Call(ctx context.Context, path string, request any) (*Response[json.RawMessage], error) {
	return call[json.RawMessage](ctx, c, path, request)
}

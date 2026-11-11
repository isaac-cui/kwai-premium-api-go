package premium

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

const DefaultChunkSize = 4 * 1024 * 1024

type Uploader struct {
	endpoint      string
	token         string
	http          *http.Client
	responseLimit int64
}

// UploadResponse uses the storage service's HTTP contract, not the Premium JSON envelope.
type UploadResponse struct {
	HTTPStatus int
	Header     http.Header
	Body       []byte
}

// NewUploader constructs a storage client from an image/video UploadTokenData.
// Scheme-less endpoints use HTTPS; a token can only be used for one complete file.
func (c *Client) NewUploader(data UploadTokenData) (*Uploader, error) {
	endpoint := data.UploadEndpoint
	if !strings.Contains(endpoint, "://") {
		endpoint = "https://" + endpoint
	}
	if err := validateOrigin(endpoint); err != nil {
		return nil, err
	}
	if err := required("upload token", data.Token, 0); err != nil {
		return nil, err
	}
	return &Uploader{endpoint: strings.TrimRight(endpoint, "/"), token: data.Token, http: c.uploadHTTP, responseLimit: c.responseLimit}, nil
}

func (u *Uploader) request(ctx context.Context, path string, query url.Values, content []byte) (*UploadResponse, error) {
	query.Set("upload_token", u.token)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u.endpoint+path+"?"+query.Encode(), bytes.NewReader(content))
	if err != nil {
		return nil, errors.New("premium: invalid upload request")
	}
	req.Header.Set("Content-Type", "application/octet-stream")
	// Never add the Premium API key to a storage request.
	raw, status, header, err := execute(ctx, u.http, req, "storage", u.responseLimit)
	if err != nil {
		return nil, err
	}
	if status != http.StatusOK {
		return nil, &APIError{HTTPStatus: status, Result: -1, Code: "STORAGE_HTTP_ERROR", Message: "storage upload rejected", RetryAfter: retryAfter(header), Header: header, Raw: raw}
	}
	// The document defines HTTP 200 as success; storage may return empty or non-JSON data.
	return &UploadResponse{HTTPStatus: status, Header: header, Body: raw}, nil
}

// Fragment supports a single large fragment as documented. Prefer Upload with
// DefaultChunkSize for bounded memory and the recommended 4 MiB fragments.
func (u *Uploader) Fragment(ctx context.Context, index int, content []byte) (*UploadResponse, error) {
	if index < 0 || len(content) == 0 {
		return nil, errors.New("premium: nonnegative index and nonempty fragment are required")
	}
	return u.request(ctx, "/api/upload/fragment", url.Values{"fragment_id": {strconv.Itoa(index)}}, content)
}
func (u *Uploader) Complete(ctx context.Context, count int) (*UploadResponse, error) {
	if count < 1 {
		return nil, errors.New("premium: fragment count must be positive")
	}
	return u.request(ctx, "/api/upload/complete", url.Values{"fragment_count": {strconv.Itoa(count)}}, nil)
}

// Upload reads one chunk ahead to reject empty/unreadable input before transmitting.
// No partial file is completed after a reader, network, or cancellation failure.
func (u *Uploader) Upload(ctx context.Context, reader io.Reader, chunkSize int) (*UploadResponse, error) {
	if reader == nil || chunkSize < 1 || chunkSize > DefaultChunkSize {
		return nil, errors.New("premium: reader and chunk size within 1 byte to 4 MiB are required")
	}
	buffer := make([]byte, chunkSize)
	count := 0
	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		n, err := io.ReadFull(reader, buffer)
		if err != nil && err != io.EOF && err != io.ErrUnexpectedEOF {
			return nil, err
		}
		if n > 0 {
			if _, uploadErr := u.Fragment(ctx, count, buffer[:n]); uploadErr != nil {
				return nil, uploadErr
			}
			count++
		}
		if err == io.EOF || err == io.ErrUnexpectedEOF {
			break
		}
	}
	if count == 0 {
		return nil, errors.New("premium: cannot upload an empty file")
	}
	return u.Complete(ctx, count)
}

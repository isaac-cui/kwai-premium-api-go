package premium

import (
	"context"
	"errors"
	"fmt"
	"io"
	"time"
)

type ImagesService struct{ client *Client }
type VideosService struct{ client *Client }
type DramasService struct{ client *Client }

func (s *ImagesService) UploadToken(ctx context.Context, r ImageTokenRequest) (*Response[UploadTokenData], error) {
	if err := validateType("imageType", r.ImageType, "jpg", "jpeg", "png", "webp"); err != nil {
		return nil, err
	}
	return call[UploadTokenData](ctx, s.client, PathImageToken, r)
}
func (s *ImagesService) Submit(ctx context.Context, r ImageSubmitRequest) (*Response[ImageSubmitData], error) {
	if err := required("token", r.Token, 0); err != nil {
		return nil, err
	}
	return call[ImageSubmitData](ctx, s.client, PathImageSubmit, r)
}
func (s *VideosService) UploadToken(ctx context.Context, r VideoTokenRequest) (*Response[UploadTokenData], error) {
	if err := validateType("videoType", r.VideoType, "mp4", "mov"); err != nil {
		return nil, err
	}
	return call[UploadTokenData](ctx, s.client, PathVideoToken, r)
}
func (s *VideosService) Submit(ctx context.Context, r VideoSubmitRequest) (*Response[VideoSubmitData], error) {
	if err := required("token", r.Token, 0); err != nil {
		return nil, err
	}
	if err := validateType("videoType", r.VideoType, "mp4", "mov"); err != nil {
		return nil, err
	}
	return call[VideoSubmitData](ctx, s.client, PathVideoSubmit, r)
}
func (s *VideosService) Transcode(ctx context.Context, r VideoTranscodeRequest) (*Response[VideoTranscodeData], error) {
	if err := required("videoId", r.VideoID, 0); err != nil {
		return nil, err
	}
	return call[VideoTranscodeData](ctx, s.client, PathVideoTranscode, r)
}
func (s *DramasService) Create(ctx context.Context, r CreateDramaRequest) (*Response[CreateDramaData], error) {
	if err := r.Validate(); err != nil {
		return nil, err
	}
	return call[CreateDramaData](ctx, s.client, PathDramaCreate, r)
}

type PollOptions struct {
	// Interval defaults to the documented 10 seconds.
	Interval time.Duration
	// Timeout defaults to 30 minutes; the parent context may impose an earlier limit.
	Timeout time.Duration
}
type TranscodeError struct {
	VideoID string
	TraceID string
}

func (e *TranscodeError) Error() string {
	return fmt.Sprintf("premium: video transcoding failed (videoId=%s, traceId=%s)", e.VideoID, e.TraceID)
}

// WaitForTranscode only repeats the read-only query. Business errors, including
// rate limiting, return to the caller with RetryAfter instead of being swallowed.
func (s *VideosService) WaitForTranscode(ctx context.Context, videoID string, options PollOptions) (*Response[VideoTranscodeData], error) {
	if err := required("videoId", videoID, 0); err != nil {
		return nil, err
	}
	if options.Interval < 0 || options.Timeout < 0 {
		return nil, errors.New("premium: polling interval and timeout cannot be negative")
	}
	if options.Interval == 0 {
		options.Interval = 10 * time.Second
	}
	if options.Timeout == 0 {
		options.Timeout = 30 * time.Minute
	}
	ctx, cancel := context.WithTimeout(ctx, options.Timeout)
	defer cancel()
	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		response, err := s.Transcode(ctx, VideoTranscodeRequest{VideoID: videoID})
		if err != nil {
			return nil, err
		}
		switch response.Data.Status {
		case TranscodeSucceeded:
			return response, nil
		case TranscodeFailed:
			return response, &TranscodeError{VideoID: videoID, TraceID: response.TraceID}
		case TranscodeProcessing:
		default:
			return response, &ProtocolError{Message: "unsupported transcode status", TraceID: response.TraceID}
		}
		timer := time.NewTimer(options.Interval)
		select {
		case <-ctx.Done():
			timer.Stop()
			return nil, ctx.Err()
		case <-timer.C:
		}
	}
}

// Upload uploads an image using a new single-file token, completes storage upload,
// and submits the image to obtain imageKey. It does not close the caller's reader.
func (s *ImagesService) Upload(ctx context.Context, reader io.Reader, imageType string) (*Response[ImageSubmitData], error) {
	if reader == nil {
		return nil, errors.New("premium: reader is required")
	}
	token, err := s.UploadToken(ctx, ImageTokenRequest{ImageType: imageType})
	if err != nil {
		return nil, err
	}
	uploader, err := s.client.NewUploader(token.Data)
	if err != nil {
		return nil, err
	}
	if _, err = uploader.Upload(ctx, reader, DefaultChunkSize); err != nil {
		return nil, err
	}
	return s.Submit(ctx, ImageSubmitRequest{Token: token.Data.Token})
}

// Upload uploads and submits a video once. Use WaitForTranscode on the returned
// videoId before creating a drama. A fresh upload token is requested per file.
func (s *VideosService) Upload(ctx context.Context, reader io.Reader, videoType string) (*Response[VideoSubmitData], error) {
	if reader == nil {
		return nil, errors.New("premium: reader is required")
	}
	token, err := s.UploadToken(ctx, VideoTokenRequest{VideoType: videoType})
	if err != nil {
		return nil, err
	}
	uploader, err := s.client.NewUploader(token.Data)
	if err != nil {
		return nil, err
	}
	if _, err = uploader.Upload(ctx, reader, DefaultChunkSize); err != nil {
		return nil, err
	}
	return s.Submit(ctx, VideoSubmitRequest{Token: token.Data.Token, VideoType: videoType})
}

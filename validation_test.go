package premium

import (
	"context"
	"strings"
	"testing"
)

func TestDramaValidationRules(t *testing.T) {
	tests := []struct {
		name   string
		change func(*CreateDramaRequest)
	}{
		{"missing key", func(r *CreateDramaRequest) { r.IdempotencyKey = "" }},
		{"long key", func(r *CreateDramaRequest) { r.IdempotencyKey = strings.Repeat("字", 129) }},
		{"long title", func(r *CreateDramaRequest) { r.Title = strings.Repeat("字", 101) }},
		{"description", func(r *CreateDramaRequest) { r.Description = strings.Repeat("x", 1501) }},
		{"nil header array", func(r *CreateDramaRequest) { r.HeaderImages = nil }},
		{"empty header key", func(r *CreateDramaRequest) { r.HeaderImages[0].ImageKey = "" }},
		{"no episodes", func(r *CreateDramaRequest) { r.Episodes = nil }},
		{"negative index", func(r *CreateDramaRequest) { r.Episodes[0].EpisodeIndex = -1 }},
		{"duplicate index", func(r *CreateDramaRequest) { r.Episodes[0].EpisodeIndex = 0 }},
		{"missing zero index", func(r *CreateDramaRequest) { r.Episodes[1].EpisodeIndex = 2 }},
		{"empty video", func(r *CreateDramaRequest) { r.Episodes[0].VideoID = "" }},
		{"invalid fee", func(r *CreateDramaRequest) { r.Episodes[0].ModuleFeeType = 0 }},
		{"no paid", func(r *CreateDramaRequest) { r.Episodes[0].ModuleFeeType = EpisodeFree }},
		{"no free", func(r *CreateDramaRequest) { r.Episodes[1].ModuleFeeType = EpisodePaid }},
		{"paid before free", func(r *CreateDramaRequest) {
			r.Episodes[0].ModuleFeeType = EpisodeFree
			r.Episodes[1].ModuleFeeType = EpisodePaid
		}},
		{"episode title", func(r *CreateDramaRequest) { r.Episodes[0].Title = "" }},
		{"episode description", func(r *CreateDramaRequest) { r.Episodes[0].Description = strings.Repeat("x", 301) }},
		{"too many episodes", func(r *CreateDramaRequest) { r.Episodes = make([]Episode, 131) }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			r := validDrama()
			test.change(&r)
			if err := r.Validate(); err == nil {
				t.Fatal("invalid drama accepted")
			}
		})
	}
	r := validDrama()
	r.Title = strings.Repeat("字", 100)
	r.HeaderImages = []HeaderImage{}
	if err := r.Validate(); err != nil {
		t.Fatal(err)
	}
	if r.Episodes[0].EpisodeIndex != 1 {
		t.Fatal("validation sorted caller slice")
	}
}
func TestInvalidFieldsDoNotReachTransport(t *testing.T) {
	c, _ := New(Config{APIKey: "key", HTTPClient: nil})
	ctx := context.Background()
	if _, err := c.Images.UploadToken(ctx, ImageTokenRequest{ImageType: "gif"}); err == nil {
		t.Fatal("unsupported image accepted")
	}
	if _, err := c.Videos.UploadToken(ctx, VideoTokenRequest{VideoType: "avi"}); err == nil {
		t.Fatal("unsupported video accepted")
	}
	if _, err := c.Images.Submit(ctx, ImageSubmitRequest{}); err == nil {
		t.Fatal("empty token accepted")
	}
	if _, err := c.Videos.Submit(ctx, VideoSubmitRequest{Token: "x", VideoType: "avi"}); err == nil {
		t.Fatal("unsupported submit format accepted")
	}
	if _, err := c.Videos.Transcode(ctx, VideoTranscodeRequest{}); err == nil {
		t.Fatal("empty videoId accepted")
	}
}

package premium

const (
	PathImageToken      = "/open/v1/resource/image/token"
	PathImageSubmit     = "/open/v1/resource/image/submit"
	PathVideoToken      = "/open/v1/resource/video/token"
	PathVideoSubmit     = "/open/v1/resource/video/submit"
	PathVideoTranscode  = "/open/v1/resource/video/transcode"
	PathDramaCreate     = "/open/v1/drama/create"
	TranscodeProcessing = 1
	TranscodeSucceeded  = 2
	TranscodeFailed     = 3
	EpisodeFree         = 1
	EpisodePaid         = 2
)

type ImageTokenRequest struct {
	ImageType string `json:"imageType"`
}
type VideoTokenRequest struct {
	VideoType string `json:"videoType"`
}
type UploadTokenData struct {
	Token          string `json:"token"`
	UploadEndpoint string `json:"uploadEndpoint"`
}
type ImageSubmitRequest struct {
	Token string `json:"token"`
}
type ImageSubmitData struct {
	ImageKey string `json:"imageKey"`
}
type VideoSubmitRequest struct {
	Token     string `json:"token"`
	VideoType string `json:"videoType"`
}
type VideoSubmitData struct {
	VideoID string `json:"videoId"`
}
type VideoTranscodeRequest struct {
	VideoID string `json:"videoId"`
}
type VideoTranscodeData struct {
	Status int `json:"status"`
}
type HeaderImage struct {
	ImageKey string `json:"imageKey"`
}
type Episode struct {
	// EpisodeIndex starts at zero and is always serialized, including zero.
	EpisodeIndex  int    `json:"episodeIndex"`
	VideoID       string `json:"videoId"`
	Title         string `json:"title"`
	Description   string `json:"description,omitempty"`
	ModuleFeeType int    `json:"moduleFeeType"`
}
type CreateDramaRequest struct {
	IdempotencyKey string `json:"idempotencyKey"`
	Title          string `json:"title"`
	Description    string `json:"description,omitempty"`
	CoverImageKey  string `json:"coverImageKey"`
	// HeaderImages must be provided; use []HeaderImage{} to explicitly send [].
	HeaderImages []HeaderImage `json:"headerImages"`
	Episodes     []Episode     `json:"episodes"`
}
type CreateDramaData struct {
	DramaID int64 `json:"dramaId"`
}

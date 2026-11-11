package premium

// Error codes are preserved verbatim from the Premium API documentation.
const (
	ErrCodeUnauthorized             = "UNAUTHORIZED"
	ErrCodeRateLimitExceeded        = "RATE_LIMIT_EXCEEDED"
	ErrCodeParamValidationFailure   = "PARAM_VALIDATION_FAILURE"
	ErrCodeServiceBusy              = "SERVICE_BUSY"
	ErrCodeUploadTokenFailed        = "UPLOAD_TOKEN_FAILED"
	ErrCodeTokenVerifyFailed        = "TOKEN_VERIFY_FAILED"
	ErrCodeSubmitTransferCodeFailed = "SUBMIT_TRANSFER_CODE_FAILED"
	ErrCodeTranscodeInfoNotFound    = "TRANSCODE_INFO_NOT_FOUND"
	ErrCodeDataNotFound             = "DATA_NOT_FOUND"
	ErrCodeNotHaveAuth              = "NOT_HAVE_AUTH"
	ErrCodeIdempotencyProcessing    = "OPEN_API_IDEMPOTENCY_PROCESSING"
	ErrCodeDailyQuotaExceeded       = "OPEN_API_DAILY_QUOTA_EXCEEDED"
	ErrCodeVideoNotTranscoded       = "OPEN_API_VIDEO_NOT_TRANSCODED"
	ErrCodeNoFreeEpisode            = "COURSE_MODULE_NO_FREE"
	ErrCodeNoPaidEpisode            = "COURSE_MODULE_NO_PAID"
	ErrCodeFreeNotConsecutive       = "COURSE_MODULE_FREE_NOT_CONSECUTIVE"
)

package apikey

import "errors"

var (
	ErrAPIKeyNotFound      = errors.New("api key not found")
	ErrAPIKeyRevoked       = errors.New("api key has been revoked")
	ErrAPIKeyExpired       = errors.New("api key has expired")
	ErrAPIKeyInvalid       = errors.New("api key is invalid")
	ErrInvalidScope        = errors.New("invalid scope")
	ErrScopeNotAllowed     = errors.New("scope not allowed for this operation")
	ErrRateLimitExceeded   = errors.New("rate limit exceeded")
	ErrInvalidKeyFormat    = errors.New("invalid API key format")
	ErrDuplicateKeyName    = errors.New("API key with this name already exists")
)

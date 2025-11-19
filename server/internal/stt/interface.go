package stt

import (
	"context"
	"errors"
	"io"
)

// TranscriptionResult represents the result of speech-to-text transcription
type TranscriptionResult struct {
	Text       string  `json:"text"`
	Confidence float64 `json:"confidence"` // 0.0 to 1.0
	Language   string  `json:"language"`
	Duration   float64 `json:"duration_seconds"`
}

// TranscriptionOptions provides configuration for transcription
type TranscriptionOptions struct {
	Language       string  // Target language (e.g., "ko-KR", "en-US")
	EnableProfanityFilter bool    // Filter profanity in results
	MaxDuration    int     // Maximum audio duration in seconds
	Format         string  // Audio format: "wav", "mp3", "m4a", "webm"
}

// Provider defines the interface for speech-to-text services
// This allows for multiple implementations (Google Cloud, AWS, Azure, Whisper, etc.)
type Provider interface {
	// Transcribe converts audio to text
	Transcribe(ctx context.Context, audio io.Reader, opts TranscriptionOptions) (*TranscriptionResult, error)

	// SupportsLanguage checks if the provider supports the given language
	SupportsLanguage(language string) bool

	// GetSupportedFormats returns the list of supported audio formats
	GetSupportedFormats() []string
}

// Service wraps the STT provider with additional business logic
type Service struct {
	provider Provider
	enabled  bool // Feature flag for STT
}

// NewService creates a new STT service
func NewService(provider Provider, enabled bool) *Service {
	return &Service{
		provider: provider,
		enabled:  enabled,
	}
}

// Transcribe transcribes audio to text if STT is enabled
func (s *Service) Transcribe(ctx context.Context, audio io.Reader, opts TranscriptionOptions) (*TranscriptionResult, error) {
	if !s.enabled {
		return nil, errors.New("speech-to-text is not enabled")
	}

	if s.provider == nil {
		return nil, errors.New("no STT provider configured")
	}

	return s.provider.Transcribe(ctx, audio, opts)
}

// IsEnabled returns whether STT is enabled
func (s *Service) IsEnabled() bool {
	return s.enabled
}

// SupportsLanguage checks if the configured provider supports the language
func (s *Service) SupportsLanguage(language string) bool {
	if !s.enabled || s.provider == nil {
		return false
	}
	return s.provider.SupportsLanguage(language)
}
